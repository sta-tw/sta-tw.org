// Package community exposes small public read-only endpoints for community
// widgets on the marketing site (e.g. the Discord member count shown on the
// homepage "join our Discord" section).
package community

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Handler serves community-facing read-only stats.
type Handler struct {
	inviteCode string
	httpClient *http.Client
	cacheTTL   time.Duration

	mu        sync.Mutex
	cached    *discordStats
	cachedErr error
	fetchedAt time.Time
}

type discordStats struct {
	ApproximateMemberCount int `json:"approximate_member_count"`
}

// NewHandler builds a community handler. inviteCode is the Discord invite
// code (the part after discord.gg/); an empty code disables the endpoint.
func NewHandler(inviteCode string) *Handler {
	return &Handler{
		inviteCode: strings.TrimSpace(inviteCode),
		httpClient: &http.Client{Timeout: 5 * time.Second},
		cacheTTL:   1 * time.Hour,
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/community/discord", h.getDiscordStats)
}

func (h *Handler) getDiscordStats(w http.ResponseWriter, r *http.Request) {
	if h.inviteCode == "" {
		writeCommunityError(w, http.StatusNotFound, "not_configured", "Discord community stats are not configured")
		return
	}

	stats, err := h.discordStats(r.Context())
	if err != nil {
		writeCommunityError(w, http.StatusBadGateway, "upstream_error", "could not fetch Discord member count")
		return
	}

	writeCommunityJSON(w, http.StatusOK, map[string]any{
		"member_count":             roundToHundred(stats.ApproximateMemberCount),
		"approximate_member_count": stats.ApproximateMemberCount,
	})
}

// discordStats returns the cached invite stats, refreshing them from Discord
// when the cache is empty or older than cacheTTL. A stale cache is still
// served if the refresh fails.
func (h *Handler) discordStats(ctx context.Context) (*discordStats, error) {
	h.mu.Lock()
	fresh := h.cached != nil && time.Since(h.fetchedAt) < h.cacheTTL
	cached := h.cached
	h.mu.Unlock()
	if fresh {
		return cached, nil
	}

	stats, err := h.fetchDiscordStats(ctx)
	h.mu.Lock()
	defer h.mu.Unlock()
	if err != nil {
		h.cachedErr = err
		if h.cached != nil {
			return h.cached, nil
		}
		return nil, err
	}
	h.cached = stats
	h.cachedErr = nil
	h.fetchedAt = time.Now()
	return stats, nil
}

func (h *Handler) fetchDiscordStats(ctx context.Context) (*discordStats, error) {
	url := fmt.Sprintf("https://discord.com/api/v10/invites/%s?with_counts=true", h.inviteCode)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build Discord invite request: %w", err)
	}
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call Discord invite API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Discord invite API returned status %d", resp.StatusCode)
	}
	var stats discordStats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, fmt.Errorf("decode Discord invite response: %w", err)
	}
	if stats.ApproximateMemberCount <= 0 {
		return nil, errors.New("Discord invite response missing member count")
	}
	return &stats, nil
}

func roundToHundred(n int) int {
	return int(float64(n)/100+0.5) * 100
}

type communityErrorBody struct {
	Error communityError `json:"error"`
}
type communityError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeCommunityJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if status != http.StatusNoContent {
		_ = json.NewEncoder(w).Encode(payload)
	}
}

func writeCommunityError(w http.ResponseWriter, status int, code, message string) {
	writeCommunityJSON(w, status, communityErrorBody{Error: communityError{Code: code, Message: message}})
}
