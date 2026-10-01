package discordmail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/gorilla/websocket"
)

const gatewayURL = "wss://gateway.discord.gg/?v=10&encoding=json"

// gatewayOp is a Discord Gateway payload's top-level "op" field.
// https://discord.com/developers/docs/events/gateway#gateway-opcodes
const (
	opDispatch       = 0
	opHeartbeat      = 1
	opIdentify       = 2
	opInvalidSession = 9
	opHello          = 10
	opHeartbeatACK   = 11
)

type gatewayPayload struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d,omitempty"`
	S  *int            `json:"s,omitempty"`
	T  string          `json:"t,omitempty"`
}

// RunPresence keeps a Discord Gateway connection open purely so this bot
// shows "online" in Discord's member list — every actual feature (creating
// forum threads, posting messages, the /re command) is plain HTTPS and
// doesn't need this connection at all. It exists only because Discord has
// no other way to report a bot as online: showing status requires an
// IDENTIFY'd Gateway session, full stop.
//
// Intents are 0 (no events requested) and every dispatch payload is
// discarded unread — this never becomes a second, event-driven command
// path; /re stays on the Interactions webhook in handler.go. Reconnects
// forever on any error with exponential backoff (capped, with jitter); meant
// to be run in its own goroutine for the lifetime of the process.
//
// The backoff is load-bearing, not cosmetic: a session that fails instantly
// every time (bad token, Discord-side issue, anything that closes the
// connection before it even reaches READY) previously retried on a flat 5s
// timer with no cap, which is exactly the "IDENTIFY spam" pattern Discord's
// abuse detection flags and resets bot tokens over — it doesn't take long to
// hit four digits of connection attempts at a fixed 5s cadence.
func RunPresence(ctx context.Context, botToken string, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	const (
		baseBackoff = 5 * time.Second
		maxBackoff  = 5 * time.Minute
		// A connection that survives this long is treated as healthy and
		// resets the backoff back to baseBackoff, so a single blip doesn't
		// leave us stuck at the max delay for the rest of the process's life.
		healthyConnectionDuration = 2 * time.Minute
	)
	backoff := baseBackoff
	for {
		if ctx.Err() != nil {
			return
		}
		connectedAt := time.Now()
		if err := runPresenceOnce(ctx, botToken, logger); err != nil && ctx.Err() == nil {
			logger.Warn("discord presence gateway disconnected, reconnecting", "error", err, "backoff", backoff)
		}
		if time.Since(connectedAt) >= healthyConnectionDuration {
			backoff = baseBackoff
		} else {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

func runPresenceOnce(ctx context.Context, botToken string, logger *slog.Logger) error {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, gatewayURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	var hello struct {
		HeartbeatInterval int `json:"heartbeat_interval"`
	}
	var first gatewayPayload
	if err := conn.ReadJSON(&first); err != nil {
		return err
	}
	if first.Op != opHello {
		return fmt.Errorf("unexpected gateway opcode %d before HELLO", first.Op)
	}
	if err := json.Unmarshal(first.D, &hello); err != nil {
		return err
	}

	identify := gatewayPayload{Op: opIdentify}
	identify.D, _ = json.Marshal(map[string]any{
		"token":   botToken,
		"intents": 0,
		"properties": map[string]string{
			"os":      "linux",
			"browser": "sta-mail",
			"device":  "sta-mail",
		},
	})
	if err := conn.WriteJSON(identify); err != nil {
		return err
	}

	var ready gatewayPayload
	if err := conn.ReadJSON(&ready); err != nil {
		return fmt.Errorf("waiting for READY after identify: %w", err)
	}
	if ready.Op != opDispatch || ready.T != "READY" {
		return fmt.Errorf("expected READY dispatch after identify, got op=%d t=%q", ready.Op, ready.T)
	}
	logger.Info("discord presence gateway connected")

	interval := time.Duration(hello.HeartbeatInterval) * time.Millisecond
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	errCh := make(chan error, 1)
	go func() {
		for {
			var payload gatewayPayload
			if err := conn.ReadJSON(&payload); err != nil {
				errCh <- err
				return
			}
			if payload.Op == opInvalidSession {
				errCh <- errInvalidSession
				return
			}
			// Every other opcode/dispatch is intentionally ignored — see
			// RunPresence's doc comment.
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errCh:
			return err
		case <-ticker.C:
			// Discord's heartbeat spec requires "d" to be present (the last
			// sequence number, or null if none) — gatewayPayload.D uses
			// `omitempty`, so leaving D unset here drops the "d" key
			// entirely instead of sending "d":null, which Discord rejects
			// with close code 4002 "Error while decoding payload". This
			// client never tracks the sequence number (every dispatch is
			// discarded unread by design), so null is always correct here.
			if err := conn.WriteJSON(gatewayPayload{Op: opHeartbeat, D: json.RawMessage("null")}); err != nil {
				return err
			}
		}
	}
}

var errInvalidSession = errors.New("gateway invalidated the session")
