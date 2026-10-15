package admissions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const cloudflareAPIBaseURL = "https://api.cloudflare.com/client/v4"

// CloudflareProgramPagePurger clears cached program-page HTML after an admin
// changes published admissions data. The current cache rule can override
// origin no-store headers, so the program-page URL prefix must be purged when
// its underlying records change.
type CloudflareProgramPagePurger struct {
	token    string
	zoneID   string
	prefixes []string
	endpoint string
	client   *http.Client
	logger   *slog.Logger
}

func NewCloudflareProgramPagePurger(token, zoneID, publicBaseURL string, logger *slog.Logger) *CloudflareProgramPagePurger {
	token = strings.TrimSpace(token)
	zoneID = strings.TrimSpace(zoneID)
	if token == "" || zoneID == "" {
		return nil
	}

	base, err := url.Parse(strings.TrimSpace(publicBaseURL))
	if err != nil || base.Hostname() == "" {
		if logger != nil {
			logger.Warn("Cloudflare program-page purge disabled: public base URL is invalid")
		}
		return nil
	}
	path := strings.TrimRight(base.EscapedPath(), "/")
	host := strings.ToLower(base.Hostname())
	if base.Port() != "" {
		host += ":" + base.Port()
	}
	prefixes := []string{
		host + path + "/bochures",
		host + path + "/bochures/program/",
		host + path + "/api/v1/admissions/programs",
		host + path + "/sitemap.xml",
		host + path + "/llms.txt",
	}

	return &CloudflareProgramPagePurger{
		token:    token,
		zoneID:   zoneID,
		prefixes: prefixes,
		endpoint: cloudflareAPIBaseURL + "/zones/" + url.PathEscape(zoneID) + "/purge_cache",
		client:   &http.Client{Timeout: 5 * time.Second},
		logger:   logger,
	}
}

func (p *CloudflareProgramPagePurger) Invalidate(ctx context.Context, programIdentifiers ...string) {
	if p == nil || len(programIdentifiers) == 0 {
		return
	}

	body, err := json.Marshal(map[string][]string{"prefixes": p.prefixes})
	if err != nil {
		p.logFailure("encode request", err)
		return
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		p.logFailure("create request", err)
		return
	}
	request.Header.Set("Authorization", "Bearer "+p.token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")

	response, err := p.client.Do(request)
	if err != nil {
		p.logFailure("send request", err)
		return
	}
	defer response.Body.Close()

	var result struct {
		Success bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil {
		p.logFailure("decode response", err)
		return
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || !result.Success {
		message := fmt.Sprintf("Cloudflare returned status %d", response.StatusCode)
		if len(result.Errors) > 0 && result.Errors[0].Message != "" {
			message += ": " + result.Errors[0].Message
		}
		p.logFailure("purge program pages", fmt.Errorf("%s", message))
	}
}

func (p *CloudflareProgramPagePurger) logFailure(operation string, err error) {
	if p.logger != nil {
		p.logger.Error("Cloudflare program-page cache purge failed", "operation", operation, "error", err)
	}
}
