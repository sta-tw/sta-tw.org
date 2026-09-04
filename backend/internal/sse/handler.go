// Package sse exposes GET /api/v1/events as a Server-Sent Events stream of the
// caller's notifications plus the shared chat lounge.
package sse

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"sta-backend/internal/auth"
	"sta-backend/internal/events"
)

// Topic names. Notification topics are per-account.
const LoungeTopic = "chat:lounge"

// maxChannelSubscriptions bounds how many chat channels one stream may follow.
const maxChannelSubscriptions = 10

func NotificationsTopic(accountID string) string { return "notifications:" + accountID }

// ChatTopic is the SSE topic for one chat channel.
func ChatTopic(channelKey string) string { return "chat:" + channelKey }

// parseChannelTopics turns ?channel=a,b,c into distinct chat topics, defaulting
// to the lounge. Unknown-looking keys are ignored rather than erroring so a
// stale client never loses its stream.
func parseChannelTopics(raw string) []string {
	seen := map[string]struct{}{}
	topics := make([]string, 0, 4)
	for _, part := range strings.Split(raw, ",") {
		key := strings.ToLower(strings.TrimSpace(part))
		if key == "" || len(key) > 32 || !isChannelKey(key) {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		topics = append(topics, ChatTopic(key))
		if len(topics) == maxChannelSubscriptions {
			break
		}
	}
	if len(topics) == 0 {
		topics = append(topics, LoungeTopic)
	}
	return topics
}

func isChannelKey(s string) bool {
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

type Handler struct {
	auth *auth.Service
	hub  *events.Hub
}

func NewHandler(authService *auth.Service, hub *events.Hub) (*Handler, error) {
	if authService == nil || hub == nil {
		return nil, errors.New("sse handler dependencies are missing")
	}
	return &Handler{auth: authService, hub: hub}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/events", h.stream)
}

func (h *Handler) stream(w http.ResponseWriter, r *http.Request) {
	session, err := h.auth.Authenticate(r.Context(), r)
	if err != nil {
		http.Error(w, `{"error":{"code":"unauthorized","message":"authentication is required"}}`, http.StatusUnauthorized)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"error":{"code":"unsupported","message":"streaming is not supported"}}`, http.StatusInternalServerError)
		return
	}

	accountID := session.Session.Account.ID.String()
	topics := append(parseChannelTopics(r.URL.Query().Get("channel")), NotificationsTopic(accountID))
	sub := h.hub.Subscribe(r.Context(), topics...)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable proxy buffering (nginx)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "retry: 3000\n\n")
	flusher.Flush()

	keepalive := time.NewTicker(25 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepalive.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case ev, open := <-sub:
			if !open {
				return
			}
			payload, err := json.Marshal(map[string]any{"topic": ev.Topic, "kind": ev.Kind, "data": ev.Data})
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Kind, payload); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
