package emailinquiries

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

// Embed colors (Discord decimal RGB) — inbound (blurple, Discord's own
// brand color for "incoming"), outbound (green, a completed action).
const (
	embedColorInbound  = 0x5865F2
	embedColorOutbound = 0x57F287
)

// HTTPDiscordNotifier implements ForumNotifier against Discord's REST API,
// posting each new inquiry as a Forum channel thread and each follow-up as
// an embed inside that thread (a thread's id doubles as its channel id for
// POST .../messages) — embeds instead of plain content so a thread's whole
// conversation can be read back by scrolling through them.
type HTTPDiscordNotifier struct {
	botToken string
	client   *http.Client
}

func NewHTTPDiscordNotifier(botToken string) (*HTTPDiscordNotifier, error) {
	botToken = strings.TrimSpace(botToken)
	if botToken == "" {
		return nil, errors.New("discord bot token is required")
	}
	return &HTTPDiscordNotifier{botToken: botToken, client: &http.Client{Timeout: discordRequestTimeout}}, nil
}

type discordEmbed struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description"`
	Color       int    `json:"color"`
	Footer      *struct {
		Text string `json:"text"`
	} `json:"footer,omitempty"`
	Timestamp string `json:"timestamp"`
}

func buildEmbed(embed Embed, title string) discordEmbed {
	color := embedColorInbound
	if embed.Kind == EmbedKindOutbound {
		color = embedColorOutbound
	}
	d := discordEmbed{
		Title:       title,
		Description: truncateForDiscord(embed.Body),
		Color:       color,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
	if embed.Author != "" {
		d.Footer = &struct {
			Text string `json:"text"`
		}{Text: embed.Author}
	}
	return d
}

func embedTitle(kind EmbedKind) string {
	switch kind {
	case EmbedKindOutbound:
		return "✅ 已回覆"
	case EmbedKindSystem:
		return "系統訊息"
	default:
		return "📨 來信"
	}
}

// mentionContent renders the plain message content Discord actually pings
// from — an embed alone, however role-ID-looking its text is, never
// notifies anyone; only content plus allowed_mentions.roles does.
func mentionContent(roleIDs []string) string {
	if len(roleIDs) == 0 {
		return ""
	}
	mentions := make([]string, len(roleIDs))
	for i, id := range roleIDs {
		mentions[i] = "<@&" + id + ">"
	}
	return strings.Join(mentions, " ")
}

type allowedMentions struct {
	Roles []string `json:"roles"`
}

func (n *HTTPDiscordNotifier) CreateThread(ctx context.Context, forumChannelID, subject string, embed Embed, mentionRoleIDs []string) (string, error) {
	name := strings.TrimSpace(subject)
	if name == "" {
		name = "（無主旨）"
	}
	if len(name) > 100 {
		name = name[:100]
	}
	payload, err := json.Marshal(struct {
		Name    string `json:"name"`
		Message struct {
			Content         string          `json:"content,omitempty"`
			Embeds          []discordEmbed  `json:"embeds"`
			AllowedMentions allowedMentions `json:"allowed_mentions"`
		} `json:"message"`
	}{Name: name, Message: struct {
		Content         string          `json:"content,omitempty"`
		Embeds          []discordEmbed  `json:"embeds"`
		AllowedMentions allowedMentions `json:"allowed_mentions"`
	}{
		Content:         mentionContent(mentionRoleIDs),
		Embeds:          []discordEmbed{buildEmbed(embed, embedTitle(embed.Kind))},
		AllowedMentions: allowedMentions{Roles: mentionRoleIDs},
	}})
	if err != nil {
		return "", err
	}
	response, err := n.request(ctx, http.MethodPost, "/channels/"+url.PathEscape(forumChannelID)+"/threads", payload)
	if err != nil {
		return "", err
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(response, &result); err != nil || strings.TrimSpace(result.ID) == "" {
		return "", errors.New("discord thread response did not contain an id")
	}
	return result.ID, nil
}

func (n *HTTPDiscordNotifier) PostEmbed(ctx context.Context, threadID string, embed Embed, mentionRoleIDs []string) error {
	payload, err := json.Marshal(struct {
		Content         string          `json:"content,omitempty"`
		Embeds          []discordEmbed  `json:"embeds"`
		AllowedMentions allowedMentions `json:"allowed_mentions"`
	}{
		Content:         mentionContent(mentionRoleIDs),
		Embeds:          []discordEmbed{buildEmbed(embed, embedTitle(embed.Kind))},
		AllowedMentions: allowedMentions{Roles: mentionRoleIDs},
	})
	if err != nil {
		return err
	}
	_, err = n.request(ctx, http.MethodPost, "/channels/"+url.PathEscape(threadID)+"/messages", payload)
	return err
}

func (n *HTTPDiscordNotifier) request(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, method, "https://discord.com/api/v10"+path, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("discord request could not be created")
	}
	request.Header.Set("Authorization", "Bot "+n.botToken)
	request.Header.Set("User-Agent", "STA-mail/1.0")
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := n.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("discord request failed: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("discord returned status %d: %s", response.StatusCode, truncateForDiscord(string(responseBody)))
	}
	return responseBody, nil
}

func truncateForDiscord(value string) string {
	// Discord embed description limit is 4096 chars; stay comfortably under it.
	const limit = 3900
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "\n…（過長，已截斷）"
}
