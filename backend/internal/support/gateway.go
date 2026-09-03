package support

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

const (
	discordGatewayURL           = "wss://gateway.discord.gg/?v=10&encoding=json"
	discordOpDispatch           = 0
	discordOpHeartbeat          = 1
	discordOpIdentify           = 2
	discordOpReconnect          = 7
	discordOpInvalidSession     = 9
	discordOpHello              = 10
	discordOpHeartbeatACK       = 11
	discordIntentGuilds         = 1
	discordIntentGuildMessages  = 1 << 9
	discordIntentMessageContent = 1 << 15
)

type DiscordGateway struct {
	Token      string
	GuildID    string
	Store      DiscordInboundStore
	LookupKey  []byte
	Logger     *slog.Logger
	GatewayURL string
}

type gatewayEnvelope struct {
	Operation int             `json:"op"`
	Sequence  *int            `json:"s"`
	Event     string          `json:"t"`
	Data      json.RawMessage `json:"d"`
}

type gatewayHello struct {
	HeartbeatInterval int `json:"heartbeat_interval"`
}

type gatewayIdentify struct {
	Token      string `json:"token"`
	Intents    int    `json:"intents"`
	Properties struct {
		OS      string `json:"os"`
		Browser string `json:"browser"`
		Device  string `json:"device"`
	} `json:"properties"`
}

type gatewayMessageCreate struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	GuildID   string `json:"guild_id"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
	Author    struct {
		ID  string `json:"id"`
		Bot bool   `json:"bot"`
	} `json:"author"`
}

func (g *DiscordGateway) Run(ctx context.Context) error {
	if g == nil || strings.TrimSpace(g.Token) == "" || strings.TrimSpace(g.GuildID) == "" || g.Store == nil {
		return errors.New("Discord gateway is not configured")
	}
	gatewayURL := strings.TrimSpace(g.GatewayURL)
	if gatewayURL == "" {
		gatewayURL = discordGatewayURL
	}
	backoff := time.Second
	for {
		err := g.runConnection(ctx, gatewayURL)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil && g.Logger != nil {
			g.Logger.Warn("Discord gateway disconnected", "error", safeDiscordWorkerError(err))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}
	}
}

func (g *DiscordGateway) runConnection(ctx context.Context, gatewayURL string) error {
	config, err := websocket.NewConfig(gatewayURL, "https://discord.com")
	if err != nil {
		return err
	}
	conn, err := websocket.DialConfig(config)
	if err != nil {
		return errors.New("connect Discord gateway")
	}
	defer conn.Close()

	var hello gatewayEnvelope
	if err := receiveGatewayEnvelope(conn, &hello); err != nil {
		return fmt.Errorf("receive Discord gateway hello: %w", err)
	}
	if hello.Operation != discordOpHello {
		return errors.New("Discord gateway did not send hello")
	}
	var helloData gatewayHello
	if err := json.Unmarshal(hello.Data, &helloData); err != nil || helloData.HeartbeatInterval < 1000 {
		return errors.New("Discord gateway heartbeat interval is invalid")
	}

	identify := gatewayIdentify{Token: g.Token, Intents: discordIntentGuilds | discordIntentGuildMessages | discordIntentMessageContent}
	identify.Properties.OS = "linux"
	identify.Properties.Browser = "sta-support"
	identify.Properties.Device = "sta-support"
	if err := sendGatewayEnvelope(conn, gatewayEnvelope{Operation: discordOpIdentify, Data: mustJSON(identify)}); err != nil {
		return fmt.Errorf("identify Discord gateway: %w", err)
	}

	state := &gatewayState{conn: conn}
	heartbeatCtx, heartbeatCancel := context.WithCancel(ctx)
	defer heartbeatCancel()
	go state.heartbeat(heartbeatCtx, time.Duration(helloData.HeartbeatInterval)*time.Millisecond)

	for {
		var envelope gatewayEnvelope
		if err := receiveGatewayEnvelope(conn, &envelope); err != nil {
			return err
		}
		if envelope.Sequence != nil {
			state.mu.Lock()
			state.sequence = envelope.Sequence
			state.mu.Unlock()
		}
		switch envelope.Operation {
		case discordOpDispatch:
			if envelope.Event == "MESSAGE_CREATE" {
				g.handleMessage(ctx, envelope.Data)
			}
		case discordOpHeartbeat:
			if err := state.sendHeartbeat(); err != nil {
				return err
			}
		case discordOpReconnect:
			return errors.New("Discord gateway requested reconnect")
		case discordOpInvalidSession:
			return errors.New("Discord gateway rejected session")
		case discordOpHeartbeatACK:
			// Heartbeat acknowledgement is intentionally only used as a health signal.
		}
	}
}

func (g *DiscordGateway) handleMessage(ctx context.Context, raw json.RawMessage) {
	var input gatewayMessageCreate
	if err := json.Unmarshal(raw, &input); err != nil {
		return
	}
	if input.GuildID != g.GuildID || input.Author.Bot || strings.TrimSpace(input.ID) == "" || strings.TrimSpace(input.ChannelID) == "" {
		return
	}
	createdAt := time.Now().UTC()
	if input.Timestamp != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, input.Timestamp); err == nil {
			createdAt = parsed
		}
	}
	_, err := g.Store.ApplyDiscordMessage(ctx, ExternalMessage{
		ChannelID: input.ChannelID, ExternalMessageID: input.ID, ExternalAuthorID: input.Author.ID,
		Body: input.Content, Operation: OperationCreate, CreatedAt: createdAt,
	}, g.LookupKey)
	if err != nil && !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrConflict) && g.Logger != nil {
		g.Logger.Warn("Discord support message could not be imported", "error", safeDiscordWorkerError(err))
	}
}

type gatewayState struct {
	conn     *websocket.Conn
	mu       sync.Mutex
	sequence *int
}

func (s *gatewayState) heartbeat(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = s.conn.Close()
			return
		case <-ticker.C:
			_ = s.sendHeartbeat()
		}
	}
}

func (s *gatewayState) sendHeartbeat() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return sendGatewayEnvelope(s.conn, gatewayEnvelope{Operation: discordOpHeartbeat, Data: mustJSON(s.sequence)})
}

func receiveGatewayEnvelope(conn *websocket.Conn, destination *gatewayEnvelope) error {
	var raw []byte
	if err := websocket.Message.Receive(conn, &raw); err != nil {
		return err
	}
	return json.Unmarshal(raw, destination)
}

func sendGatewayEnvelope(conn *websocket.Conn, envelope gatewayEnvelope) error {
	payload, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	return websocket.Message.Send(conn, payload)
}

func mustJSON(value any) json.RawMessage {
	payload, _ := json.Marshal(value)
	return payload
}
