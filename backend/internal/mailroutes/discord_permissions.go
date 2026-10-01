package mailroutes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const discordRequestTimeout = 15 * time.Second

// permViewChannel is Discord's VIEW_CHANNEL permission bit (1 << 10).
const permViewChannel = "1024"

// discordChannelTypeForum is Discord's channel `type` value for a Forum
// channel (GUILD_FORUM).
const discordChannelTypeForum = 15

// HTTPPermissionsClient implements PermissionsClient against Discord's REST
// API: creates each route's forum channel under a fixed parent category,
// denies @everyone (the guild id doubles as @everyone's role id) and allows
// exactly the picked roles to view it.
type HTTPPermissionsClient struct {
	botToken   string
	guildID    string
	categoryID string
	client     *http.Client
}

func NewHTTPPermissionsClient(botToken, guildID, categoryID string) (*HTTPPermissionsClient, error) {
	botToken = strings.TrimSpace(botToken)
	guildID = strings.TrimSpace(guildID)
	categoryID = strings.TrimSpace(categoryID)
	if botToken == "" || guildID == "" || categoryID == "" {
		return nil, errors.New("discord bot token, guild id and forum category id are required")
	}
	return &HTTPPermissionsClient{botToken: botToken, guildID: guildID, categoryID: categoryID, client: &http.Client{Timeout: discordRequestTimeout}}, nil
}

func (c *HTTPPermissionsClient) CreateForumChannel(ctx context.Context, name string) (string, error) {
	payload, err := json.Marshal(struct {
		Name     string `json:"name"`
		Type     int    `json:"type"`
		ParentID string `json:"parent_id"`
	}{Name: name, Type: discordChannelTypeForum, ParentID: c.categoryID})
	if err != nil {
		return "", err
	}
	response, err := c.request(ctx, http.MethodPost, "/guilds/"+url.PathEscape(c.guildID)+"/channels", payload)
	if err != nil {
		return "", err
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(response, &result); err != nil || strings.TrimSpace(result.ID) == "" {
		return "", errors.New("discord channel response did not contain an id")
	}
	return result.ID, nil
}

func (c *HTTPPermissionsClient) ListGuildRoles(ctx context.Context) ([]DiscordRole, error) {
	response, err := c.request(ctx, http.MethodGet, "/guilds/"+url.PathEscape(c.guildID)+"/roles", nil)
	if err != nil {
		return nil, err
	}
	var rawRoles []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Managed bool   `json:"managed"`
	}
	if err := json.Unmarshal(response, &rawRoles); err != nil {
		return nil, fmt.Errorf("parse discord roles response: %w", err)
	}
	roles := make([]DiscordRole, 0, len(rawRoles))
	for _, raw := range rawRoles {
		// Skip "managed" roles (auto-created for a bot/integration — not a
		// real, pickable staff group) and @everyone itself (its id is
		// always the guild id; ApplyChannelVisibility already handles it
		// implicitly by denying it).
		if raw.Managed || raw.ID == c.guildID {
			continue
		}
		roles = append(roles, DiscordRole{ID: raw.ID, Name: raw.Name})
	}
	return roles, nil
}

func (c *HTTPPermissionsClient) ApplyChannelVisibility(ctx context.Context, channelID string, roleIDs []string) error {
	if len(roleIDs) == 0 {
		return nil
	}
	if err := c.putOverwrite(ctx, channelID, c.guildID, "0", permViewChannel); err != nil {
		return fmt.Errorf("deny @everyone: %w", err)
	}
	for _, roleID := range roleIDs {
		if err := c.putOverwrite(ctx, channelID, roleID, permViewChannel, "0"); err != nil {
			return fmt.Errorf("allow role %s: %w", roleID, err)
		}
	}
	return nil
}

func (c *HTTPPermissionsClient) LockChannel(ctx context.Context, channelID string, previouslyVisibleRoleIDs []string) error {
	if err := c.putOverwrite(ctx, channelID, c.guildID, "0", permViewChannel); err != nil {
		return fmt.Errorf("deny @everyone: %w", err)
	}
	// A role-level ALLOW overwrite still beats @everyone's DENY in Discord's
	// permission resolution, so denying @everyone alone isn't enough —
	// every role this route ever granted access to needs its overwrite
	// removed too.
	var firstErr error
	for _, roleID := range previouslyVisibleRoleIDs {
		if err := c.deleteOverwrite(ctx, channelID, roleID); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("remove role %s overwrite: %w", roleID, err)
		}
	}
	return firstErr
}

func (c *HTTPPermissionsClient) deleteOverwrite(ctx context.Context, channelID, overwriteID string) error {
	_, err := c.request(ctx, http.MethodDelete, "/channels/"+url.PathEscape(channelID)+"/permissions/"+url.PathEscape(overwriteID), nil)
	return err
}

// ArchiveAllThreads lists every active thread whose parent is channelID
// (Discord's active-threads listing is guild-wide, not per-channel, hence
// the filter) and archives+locks each one.
func (c *HTTPPermissionsClient) ArchiveAllThreads(ctx context.Context, channelID string) error {
	response, err := c.request(ctx, http.MethodGet, "/guilds/"+url.PathEscape(c.guildID)+"/threads/active", nil)
	if err != nil {
		return fmt.Errorf("list active threads: %w", err)
	}
	var result struct {
		Threads []struct {
			ID       string `json:"id"`
			ParentID string `json:"parent_id"`
		} `json:"threads"`
	}
	if err := json.Unmarshal(response, &result); err != nil {
		return fmt.Errorf("parse active threads response: %w", err)
	}
	var firstErr error
	for _, thread := range result.Threads {
		if thread.ParentID != channelID {
			continue
		}
		if err := c.archiveThread(ctx, thread.ID); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("archive thread %s: %w", thread.ID, err)
		}
	}
	return firstErr
}

func (c *HTTPPermissionsClient) archiveThread(ctx context.Context, threadID string) error {
	payload, err := json.Marshal(struct {
		Archived bool `json:"archived"`
		Locked   bool `json:"locked"`
	}{Archived: true, Locked: true})
	if err != nil {
		return err
	}
	_, err = c.request(ctx, http.MethodPatch, "/channels/"+url.PathEscape(threadID), payload)
	return err
}

func (c *HTTPPermissionsClient) putOverwrite(ctx context.Context, channelID, overwriteID, allow, deny string) error {
	payload, err := json.Marshal(struct {
		Type  int    `json:"type"`
		Allow string `json:"allow"`
		Deny  string `json:"deny"`
	}{Type: 0, Allow: allow, Deny: deny})
	if err != nil {
		return err
	}
	_, err = c.request(ctx, http.MethodPut, "/channels/"+url.PathEscape(channelID)+"/permissions/"+url.PathEscape(overwriteID), payload)
	return err
}

func (c *HTTPPermissionsClient) request(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, method, "https://discord.com/api/v10"+path, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("discord request could not be created")
	}
	request.Header.Set("Authorization", "Bot "+c.botToken)
	request.Header.Set("User-Agent", "STA-mail/1.0")
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("discord request failed: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("discord returned status %d: %s", response.StatusCode, string(responseBody))
	}
	return responseBody, nil
}
