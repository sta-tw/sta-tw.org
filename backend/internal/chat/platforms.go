package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const platformRequestTimeout = 10 * time.Second

var ErrPlatformNotConfigured = errors.New("chat platform is not configured")

type DiscordSender struct {
	Token     string
	ChannelID string
	Client    *http.Client
}

func NewDiscordSender(token, channelID string) (*DiscordSender, error) {
	token = strings.TrimSpace(token)
	channelID = strings.TrimSpace(channelID)
	if token == "" || channelID == "" {
		return nil, ErrPlatformNotConfigured
	}
	return &DiscordSender{Token: token, ChannelID: channelID, Client: &http.Client{Timeout: platformRequestTimeout}}, nil
}

func (s *DiscordSender) Send(ctx context.Context, task OutboxTask) (string, error) {
	if s == nil || s.Token == "" || s.ChannelID == "" {
		return "", ErrPlatformNotConfigured
	}
	if task.TargetPlatform != PlatformDiscord {
		return "", fmt.Errorf("discord sender received %s task", task.TargetPlatform)
	}
	path := "/api/v10/channels/" + url.PathEscape(s.ChannelID) + "/messages"
	method := http.MethodPost
	var body []byte
	var err error
	switch task.Operation {
	case OperationCreate:
		body, err = json.Marshal(struct {
			Content string `json:"content"`
		}{Content: task.Body})
	case OperationEdit:
		if task.ExternalMessageID == "" {
			return "", errors.New("discord edit is missing external message id")
		}
		method = http.MethodPatch
		path += "/" + url.PathEscape(task.ExternalMessageID)
		body, err = json.Marshal(struct {
			Content string `json:"content"`
		}{Content: task.Body})
	case OperationDelete:
		if task.ExternalMessageID == "" {
			return "", errors.New("discord delete is missing external message id")
		}
		method = http.MethodDelete
		path += "/" + url.PathEscape(task.ExternalMessageID)
	default:
		return "", ErrInvalidMessage
	}
	if err != nil {
		return "", err
	}
	response, err := doPlatformRequest(ctx, s.Client, method, "https://discord.com"+path, s.Token, body, true)
	if err != nil {
		return "", err
	}
	if task.Operation != OperationCreate {
		return task.ExternalMessageID, nil
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(response, &result); err != nil || result.ID == "" {
		return "", errors.New("discord response did not contain a message id")
	}
	return result.ID, nil
}

type TelegramSender struct {
	Token  string
	ChatID string
	Client *http.Client
}

func NewTelegramSender(token, chatID string) (*TelegramSender, error) {
	token = strings.TrimSpace(token)
	chatID = strings.TrimSpace(chatID)
	if token == "" || chatID == "" {
		return nil, ErrPlatformNotConfigured
	}
	return &TelegramSender{Token: token, ChatID: chatID, Client: &http.Client{Timeout: platformRequestTimeout}}, nil
}

func (s *TelegramSender) Send(ctx context.Context, task OutboxTask) (string, error) {
	if s == nil || s.Token == "" || s.ChatID == "" {
		return "", ErrPlatformNotConfigured
	}
	if task.TargetPlatform != PlatformTelegram {
		return "", fmt.Errorf("telegram sender received %s task", task.TargetPlatform)
	}
	methodName := "sendMessage"
	payload := map[string]any{"chat_id": s.ChatID}
	switch task.Operation {
	case OperationCreate:
		payload["text"] = task.Body
	case OperationEdit:
		if task.ExternalMessageID == "" {
			return "", errors.New("telegram edit is missing external message id")
		}
		methodName = "editMessageText"
		messageID, err := telegramMessageID(task.ExternalMessageID)
		if err != nil {
			return "", err
		}
		payload["message_id"] = messageID
		payload["text"] = task.Body
	case OperationDelete:
		if task.ExternalMessageID == "" {
			return "", errors.New("telegram delete is missing external message id")
		}
		methodName = "deleteMessage"
		messageID, err := telegramMessageID(task.ExternalMessageID)
		if err != nil {
			return "", err
		}
		payload["message_id"] = messageID
	default:
		return "", ErrInvalidMessage
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	response, err := doPlatformRequest(ctx, s.Client, http.MethodPost, "https://api.telegram.org/bot"+s.Token+"/"+methodName, "", body, false)
	if err != nil {
		return "", err
	}
	if task.Operation == OperationDelete {
		return task.ExternalMessageID, nil
	}
	if task.Operation == OperationEdit {
		return task.ExternalMessageID, nil
	}
	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			MessageID int64 `json:"message_id"`
		} `json:"result"`
	}
	if err := json.Unmarshal(response, &result); err != nil || !result.OK || result.Result.MessageID <= 0 {
		return "", errors.New("telegram response did not contain a message id")
	}
	return strconv.FormatInt(result.Result.MessageID, 10), nil
}

func telegramMessageID(value string) (int64, error) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, errors.New("telegram external message id is invalid")
	}
	return parsed, nil
}

func doPlatformRequest(ctx context.Context, client *http.Client, method, endpoint, token string, body []byte, discordAuth bool) ([]byte, error) {
	if client == nil {
		client = &http.Client{Timeout: platformRequestTimeout}
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("chat platform request could not be created")
	}
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	if discordAuth {
		request.Header.Set("Authorization", "Bot "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		// The Telegram bot token is part of its endpoint URL. Never return the
		// underlying url.Error to logs or the retry outbox.
		return nil, errors.New("chat platform request failed")
	}
	defer response.Body.Close()
	responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if readErr != nil {
		return nil, readErr
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("chat platform returned status %d", response.StatusCode)
	}
	return responseBody, nil
}
