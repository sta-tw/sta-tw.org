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
	embedColorInbound      = 0x5865F2
	embedColorOutbound     = 0x57F287
	embedColorContribution = 0x84CC16
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
	} else if embed.Kind == EmbedKindContribution {
		color = embedColorContribution
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
	case EmbedKindContribution:
		return "📊 資料回報"
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

// CreateAccountApplicationThread creates the review post for a no-school-email
// account application. The actions are Discord message buttons; their custom
// IDs are signed by Discord and routed back through the existing interactions
// endpoint.
func (n *HTTPDiscordNotifier) CreateAccountApplicationThread(ctx context.Context, forumChannelID, subject, body string, mentionRoleIDs []string, approveCustomID, rejectCustomID string) (string, error) {
	name := strings.TrimSpace(subject)
	if name == "" {
		name = "帳號申請"
	}
	if len(name) > 100 {
		name = name[:100]
	}
	components := []map[string]any{{
		"type": 1,
		"components": []map[string]any{
			{"type": 2, "style": 3, "label": "通過", "custom_id": approveCustomID},
			{"type": 2, "style": 4, "label": "不通過", "custom_id": rejectCustomID},
		},
	}}
	payload, err := json.Marshal(struct {
		Name    string `json:"name"`
		Message struct {
			Content         string           `json:"content,omitempty"`
			Embeds          []discordEmbed   `json:"embeds"`
			Components      []map[string]any `json:"components"`
			AllowedMentions allowedMentions  `json:"allowed_mentions"`
		} `json:"message"`
	}{Name: name, Message: struct {
		Content         string           `json:"content,omitempty"`
		Embeds          []discordEmbed   `json:"embeds"`
		Components      []map[string]any `json:"components"`
		AllowedMentions allowedMentions  `json:"allowed_mentions"`
	}{
		Content:         mentionContent(mentionRoleIDs),
		Embeds:          []discordEmbed{{Title: "待審核帳號申請", Description: truncateForDiscord(body), Color: embedColorInbound, Timestamp: time.Now().UTC().Format(time.RFC3339)}},
		Components:      components,
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

// PostAccountApplicationMessage appends a follow-up to the application's
// Discord forum thread, matching the inbound/outbound embed style used by
// ordinary mail inquiries.
func (n *HTTPDiscordNotifier) PostAccountApplicationMessage(ctx context.Context, threadID, title, author, body string, inbound bool) error {
	kind := EmbedKindOutbound
	if inbound {
		kind = EmbedKindInbound
	}
	if strings.TrimSpace(title) == "" {
		title = embedTitle(kind)
	}
	embed := buildEmbed(Embed{Kind: kind, Author: author, Body: body}, title)
	payload, err := json.Marshal(struct {
		Embeds []discordEmbed `json:"embeds"`
	}{Embeds: []discordEmbed{embed}})
	if err != nil {
		return err
	}
	_, err = n.request(ctx, http.MethodPost, "/channels/"+url.PathEscape(threadID)+"/messages", payload)
	return err
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

// ResolvePlatformContributionThread returns the contribution UUID only when
// the Discord thread can be verified as a legacy platform-created report:
// it must belong to the configured forum, be owned by this bot, and have the
// expected report embed in its starter message.
func (n *HTTPDiscordNotifier) ResolvePlatformContributionThread(ctx context.Context, forumChannelID, threadID string) (string, error) {
	threadID = strings.TrimSpace(threadID)
	forumChannelID = strings.TrimSpace(forumChannelID)
	if threadID == "" || forumChannelID == "" {
		return "", errors.New("platform contribution thread could not be verified")
	}

	channelResponse, err := n.request(ctx, http.MethodGet, "/channels/"+url.PathEscape(threadID), nil)
	if err != nil {
		return "", err
	}
	var channel struct {
		ID       string `json:"id"`
		Type     int    `json:"type"`
		ParentID string `json:"parent_id"`
		OwnerID  string `json:"owner_id"`
	}
	if err := json.Unmarshal(channelResponse, &channel); err != nil {
		return "", errors.New("discord thread response could not be decoded")
	}
	if channel.ID != threadID || channel.ParentID != forumChannelID || channel.Type != 11 || channel.OwnerID == "" {
		return "", errors.New("discord thread is not a platform contribution thread")
	}

	botResponse, err := n.request(ctx, http.MethodGet, "/users/@me", nil)
	if err != nil {
		return "", err
	}
	var bot struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(botResponse, &bot); err != nil || bot.ID == "" || channel.OwnerID != bot.ID {
		return "", errors.New("discord thread is not owned by the platform bot")
	}

	messageResponse, err := n.request(ctx, http.MethodGet, "/channels/"+url.PathEscape(threadID)+"/messages/"+url.PathEscape(threadID), nil)
	if err != nil {
		return "", err
	}
	var message struct {
		ID     string `json:"id"`
		Author struct {
			ID string `json:"id"`
		} `json:"author"`
		Embeds []struct {
			Title       string `json:"title"`
			Description string `json:"description"`
		} `json:"embeds"`
	}
	if err := json.Unmarshal(messageResponse, &message); err != nil {
		return "", errors.New("discord starter message response could not be decoded")
	}
	if message.ID != threadID || message.Author.ID != bot.ID || len(message.Embeds) != 1 || message.Embeds[0].Title != "📊 資料回報" {
		return "", errors.New("discord starter message is not a platform contribution report")
	}

	const reportIDPrefix = "回報編號："
	for _, line := range strings.Split(message.Embeds[0].Description, "\n") {
		if strings.HasPrefix(line, reportIDPrefix) {
			id := strings.TrimSpace(strings.TrimPrefix(line, reportIDPrefix))
			if len(id) == 36 && id[8] == '-' && id[13] == '-' && id[18] == '-' && id[23] == '-' {
				return id, nil
			}
		}
	}
	return "", errors.New("discord starter message has no contribution report ID")
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
