// Package search is a thin Meilisearch integration: a stdlib HTTP client, a
// full reindex that projects public rows straight from PostgreSQL, and the
// GET /api/v1/search + POST /api/v1/admin/search/reindex handlers.
package search

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

// Index names.
const (
	IndexSchools     = "schools"
	IndexPrograms    = "programs"
	IndexExperiences = "experiences"
)

// AllIndexes is the default multi-search set.
var AllIndexes = []string{IndexSchools, IndexPrograms, IndexExperiences}

// Client talks to a Meilisearch instance.
type Client struct {
	baseURL string
	key     string
	http    *http.Client
}

// NewClient validates the URL and returns a client. A blank baseURL disables
// search: NewClient returns (nil, nil).
func NewClient(baseURL, key string) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, nil
	}
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("STA_MEILISEARCH_URL must be an absolute http(s) URL")
	}
	return &Client{baseURL: baseURL, key: strings.TrimSpace(key), http: &http.Client{Timeout: 15 * time.Second}}, nil
}

func (c *Client) request(ctx context.Context, method, path string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("meilisearch %s %s: HTTP %d: %s", method, path, resp.StatusCode, truncate(string(data), 300))
	}
	return data, nil
}

// EnsureIndexes creates the indexes (id primary key) and sets searchable /
// filterable attributes. Idempotent.
func (c *Client) EnsureIndexes(ctx context.Context) error {
	settings := map[string]map[string]any{
		IndexSchools: {
			"searchableAttributes": []string{"school_name", "school_code"},
			"filterableAttributes": []string{"institution_type", "is_active"},
		},
		IndexPrograms: {
			"searchableAttributes": []string{"admission_program_name", "school_name", "program_identifier", "special_talent_target"},
			"filterableAttributes": []string{"academic_year", "school_code"},
		},
		IndexExperiences: {
			"searchableAttributes": []string{"title", "snippet"},
		},
	}
	for name, s := range settings {
		if _, err := c.request(ctx, http.MethodPost, "/indexes", map[string]string{"uid": name, "primaryKey": "id"}); err != nil &&
			!strings.Contains(err.Error(), "index_already_exists") {
			return err
		}
		if _, err := c.request(ctx, http.MethodPatch, "/indexes/"+name+"/settings", s); err != nil {
			return err
		}
	}
	return nil
}

// Replace swaps the whole content of an index for docs.
func (c *Client) Replace(ctx context.Context, index string, docs []map[string]any) error {
	if _, err := c.request(ctx, http.MethodDelete, "/indexes/"+index+"/documents", nil); err != nil {
		return err
	}
	if len(docs) == 0 {
		return nil
	}
	_, err := c.request(ctx, http.MethodPut, "/indexes/"+index+"/documents", docs)
	return err
}

// Hit is one search result with its source index.
type Hit struct {
	Index    string         `json:"index"`
	Document map[string]any `json:"document"`
}

// Search runs a multi-index query and returns hits grouped by index.
func (c *Client) Search(ctx context.Context, query string, indexes []string, limitPerIndex int) (map[string][]map[string]any, error) {
	if limitPerIndex < 1 || limitPerIndex > 50 {
		limitPerIndex = 10
	}
	if len(indexes) == 0 {
		indexes = AllIndexes
	}
	queries := make([]map[string]any, 0, len(indexes))
	for _, idx := range indexes {
		queries = append(queries, map[string]any{"indexUid": idx, "q": query, "limit": limitPerIndex})
	}
	data, err := c.request(ctx, http.MethodPost, "/multi-search", map[string]any{"queries": queries})
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Results []struct {
			IndexUID string           `json:"indexUid"`
			Hits     []map[string]any `json:"hits"`
		} `json:"results"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	out := make(map[string][]map[string]any, len(parsed.Results))
	for _, r := range parsed.Results {
		out[r.IndexUID] = r.Hits
	}
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
