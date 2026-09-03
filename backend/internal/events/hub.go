// Package events is a lightweight fan-out bus for Server-Sent Events. It uses
// PostgreSQL LISTEN/NOTIFY so a publish on any API replica (or a worker) reaches
// SSE subscribers on every replica. Each process runs one listener connection
// and delivers to its local subscribers.
package events

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Channel is the single Postgres NOTIFY channel all events travel on.
const Channel = "sta_events"

// Event is one message. Topic routes it to subscribers; Kind and Data are
// opaque to the hub and forwarded to the client.
type Event struct {
	Topic string          `json:"topic"`
	Kind  string          `json:"kind"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// Publisher records an event so subscribers receive it. Safe for concurrent use.
type Publisher interface {
	Publish(ctx context.Context, ev Event) error
}

type subscriber struct {
	topics map[string]struct{}
	ch     chan Event
}

// Hub owns the listener connection and the local subscriber set.
type Hub struct {
	pool   *pgxpool.Pool
	logger *slog.Logger

	mu   sync.RWMutex
	subs map[*subscriber]struct{}
}

// NewHub starts the listener goroutine; it stops when ctx is cancelled.
func NewHub(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) *Hub {
	if logger == nil {
		logger = slog.Default()
	}
	h := &Hub{pool: pool, logger: logger, subs: make(map[*subscriber]struct{})}
	go h.listenLoop(ctx)
	return h
}

// Publish sends an event to every replica via pg_notify.
func (h *Hub) Publish(ctx context.Context, ev Event) error {
	if h == nil || h.pool == nil {
		return errors.New("event hub is not configured")
	}
	if ev.Topic == "" {
		return errors.New("event topic is required")
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	_, err = h.pool.Exec(ctx, "SELECT pg_notify($1, $2)", Channel, string(payload))
	return err
}

// PublishData is a convenience wrapper that marshals data and publishes an
// Event. Producers can depend on a local interface with just this method to
// avoid importing this package.
func (h *Hub) PublishData(ctx context.Context, topic, kind string, data any) error {
	var raw json.RawMessage
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return err
		}
		raw = b
	}
	return h.Publish(ctx, Event{Topic: topic, Kind: kind, Data: raw})
}

// Subscribe returns a channel of events matching any of topics. The channel is
// closed when ctx is done. A slow consumer drops events rather than blocking
// the hub.
func (h *Hub) Subscribe(ctx context.Context, topics ...string) <-chan Event {
	s := &subscriber{topics: make(map[string]struct{}, len(topics)), ch: make(chan Event, 32)}
	for _, t := range topics {
		s.topics[t] = struct{}{}
	}
	h.mu.Lock()
	h.subs[s] = struct{}{}
	h.mu.Unlock()

	go func() {
		<-ctx.Done()
		h.mu.Lock()
		delete(h.subs, s)
		h.mu.Unlock()
		close(s.ch)
	}()
	return s.ch
}

func (h *Hub) deliver(ev Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for s := range h.subs {
		if _, ok := s.topics[ev.Topic]; !ok {
			continue
		}
		select {
		case s.ch <- ev:
		default: // subscriber is behind; drop
		}
	}
}

func (h *Hub) listenLoop(ctx context.Context) {
	backoff := time.Second
	for ctx.Err() == nil {
		if err := h.listenOnce(ctx); err != nil && ctx.Err() == nil {
			h.logger.Warn("event listener dropped; reconnecting", "error", err, "retry_in", backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
	}
}

func (h *Hub) listenOnce(ctx context.Context) error {
	conn, err := h.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "LISTEN "+Channel); err != nil {
		return err
	}
	h.logger.Info("event listener connected", "channel", Channel)
	for ctx.Err() == nil {
		notification, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			return err
		}
		var ev Event
		if err := json.Unmarshal([]byte(notification.Payload), &ev); err != nil {
			h.logger.Warn("dropping malformed event payload")
			continue
		}
		h.deliver(ev)
	}
	return ctx.Err()
}
