package support

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const discordRequestTimeout = 15 * time.Second

type DiscordConfig struct {
	Token             string
	GuildID           string
	CategoryID        string
	ArchiveCategoryID string
	SupportRoleID     string
}

type DiscordSender struct {
	config DiscordConfig
	client *http.Client
}

func NewDiscordSender(config DiscordConfig) (*DiscordSender, error) {
	config.Token = strings.TrimSpace(config.Token)
	config.GuildID = strings.TrimSpace(config.GuildID)
	config.CategoryID = strings.TrimSpace(config.CategoryID)
	config.ArchiveCategoryID = strings.TrimSpace(config.ArchiveCategoryID)
	config.SupportRoleID = strings.TrimSpace(config.SupportRoleID)
	if config.Token == "" || config.GuildID == "" || config.CategoryID == "" || config.SupportRoleID == "" {
		return nil, errors.New("Discord support token, guild, category, and support role are required")
	}
	return &DiscordSender{config: config, client: &http.Client{Timeout: discordRequestTimeout}}, nil
}

func (s *DiscordSender) Send(ctx context.Context, task DiscordOutboxTask) (string, error) {
	if s == nil || s.config.Token == "" {
		return "", errors.New("Discord support sender is not configured")
	}
	switch task.Operation {
	case "create_channel":
		return s.createChannel(ctx, task)
	case "create_message":
		return s.createMessage(ctx, task)
	case "edit_message":
		return s.editMessage(ctx, task)
	case "delete_message":
		return s.deleteMessage(ctx, task)
	case "archive_channel":
		return s.archiveChannel(ctx, task)
	case "reopen_channel":
		return s.reopenChannel(ctx, task)
	default:
		return "", errors.New("unknown Discord support outbox operation")
	}
}

func (s *DiscordSender) createChannel(ctx context.Context, task DiscordOutboxTask) (string, error) {
	type permissionOverwrite struct {
		ID    string `json:"id"`
		Type  int    `json:"type"`
		Allow string `json:"allow,omitempty"`
		Deny  string `json:"deny,omitempty"`
	}
	payload := struct {
		Name                 string                `json:"name"`
		Type                 int                   `json:"type"`
		ParentID             string                `json:"parent_id"`
		PermissionOverwrites []permissionOverwrite `json:"permission_overwrites"`
	}{
		Name:     discordChannelName(task.TicketNumber),
		Type:     0,
		ParentID: s.config.CategoryID,
		PermissionOverwrites: []permissionOverwrite{
			{ID: s.config.GuildID, Type: 0, Deny: "1024"},
			{ID: s.config.SupportRoleID, Type: 0, Allow: "3072"},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	response, err := s.request(ctx, http.MethodPost, "/api/v10/guilds/"+url.PathEscape(s.config.GuildID)+"/channels", body)
	if err != nil {
		return "", err
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(response, &result); err != nil || strings.TrimSpace(result.ID) == "" {
		return "", errors.New("Discord channel response did not contain an id")
	}
	return result.ID, nil
}

func (s *DiscordSender) createMessage(ctx context.Context, task DiscordOutboxTask) (string, error) {
	if strings.TrimSpace(task.ChannelID) == "" {
		return "", errors.New("Discord support channel is not ready")
	}
	payload, err := json.Marshal(struct {
		Content string `json:"content"`
	}{Content: discordMessageContent(task)})
	if err != nil {
		return "", err
	}
	response, err := s.request(ctx, http.MethodPost, "/api/v10/channels/"+url.PathEscape(task.ChannelID)+"/messages", payload)
	if err != nil {
		return "", err
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(response, &result); err != nil || strings.TrimSpace(result.ID) == "" {
		return "", errors.New("Discord message response did not contain an id")
	}
	return result.ID, nil
}

func (s *DiscordSender) editMessage(ctx context.Context, task DiscordOutboxTask) (string, error) {
	if task.ChannelID == "" || task.ExternalMessageID == "" {
		return "", errors.New("Discord edit data is incomplete")
	}
	payload, err := json.Marshal(struct {
		Content string `json:"content"`
	}{Content: discordMessageContent(task)})
	if err != nil {
		return "", err
	}
	_, err = s.request(ctx, http.MethodPatch, "/api/v10/channels/"+url.PathEscape(task.ChannelID)+"/messages/"+url.PathEscape(task.ExternalMessageID), payload)
	return task.ExternalMessageID, err
}

func (s *DiscordSender) deleteMessage(ctx context.Context, task DiscordOutboxTask) (string, error) {
	if task.ChannelID == "" || task.ExternalMessageID == "" {
		return "", errors.New("Discord delete data is incomplete")
	}
	_, err := s.request(ctx, http.MethodDelete, "/api/v10/channels/"+url.PathEscape(task.ChannelID)+"/messages/"+url.PathEscape(task.ExternalMessageID), nil)
	return task.ExternalMessageID, err
}

func (s *DiscordSender) archiveChannel(ctx context.Context, task DiscordOutboxTask) (string, error) {
	if strings.TrimSpace(task.ChannelID) == "" {
		return "", errors.New("Discord archive channel is missing")
	}
	payload := map[string]string{"name": "closed-" + discordChannelName(task.TicketNumber)}
	if s.config.ArchiveCategoryID != "" {
		payload["parent_id"] = s.config.ArchiveCategoryID
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	_, err = s.request(ctx, http.MethodPatch, "/api/v10/channels/"+url.PathEscape(task.ChannelID), body)
	return task.ChannelID, err
}

func (s *DiscordSender) reopenChannel(ctx context.Context, task DiscordOutboxTask) (string, error) {
	if strings.TrimSpace(task.ChannelID) == "" {
		return "", errors.New("Discord reopen channel is missing")
	}
	payload := map[string]string{
		"name":      discordChannelName(task.TicketNumber),
		"parent_id": s.config.CategoryID,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	_, err = s.request(ctx, http.MethodPatch, "/api/v10/channels/"+url.PathEscape(task.ChannelID), body)
	return task.ChannelID, err
}

func (s *DiscordSender) request(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, method, "https://discord.com"+path, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("Discord request could not be created")
	}
	request.Header.Set("Authorization", "Bot "+s.config.Token)
	request.Header.Set("User-Agent", "STA-support/1.0")
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	client := s.client
	if client == nil {
		client = &http.Client{Timeout: discordRequestTimeout}
	}
	response, err := client.Do(request)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return nil, errors.New("Discord request timed out")
		}
		return nil, errors.New("Discord request failed")
	}
	defer response.Body.Close()
	responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if readErr != nil {
		return nil, readErr
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("Discord returned status %d", response.StatusCode)
	}
	return responseBody, nil
}

func discordChannelName(ticketNumber int64) string {
	return "ticket-" + fmt.Sprintf("%06d", ticketNumber)
}

func discordMessageContent(task DiscordOutboxTask) string {
	prefix := "客服"
	if task.AuthorType == "user" {
		prefix = "使用者"
	}
	body := strings.TrimSpace(task.Body)
	content := "[" + discordChannelName(task.TicketNumber) + "] " + prefix + "：" + body
	if len(content) > 1900 {
		content = content[:1900] + "…（完整內容請至 STA 查看）"
	}
	return content
}

func discordMessageID(value string) string {
	if _, err := strconv.ParseInt(value, 10, 64); err == nil {
		return value
	}
	return value
}
