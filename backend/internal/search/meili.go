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
// taiVariantSynonyms maps 台/臺 both directions so either variant matches.
var taiVariantSynonyms = map[string][]string{
	"台": {"臺"}, "臺": {"台"},
	"台灣": {"臺灣"}, "臺灣": {"台灣"},
	"台北": {"臺北"}, "臺北": {"台北"},
	"台中": {"臺中"}, "臺中": {"台中"},
	"台南": {"臺南"}, "臺南": {"台南"},
	"台東": {"臺東"}, "臺東": {"台東"},
}

// schoolAbbreviationSynonyms covers common short forms of a school's name
// that AREN'T a contiguous substring of the official name (so plain
// substring/prefix matching alone can't find them) — e.g. "交大" doesn't
// literally appear in "國立陽明交通大學" (the sequence there is 交-通-大,
// not 交-大). A name like "中興大學", where the short form IS already a
// contiguous substring (中興 or 興大), needs no entry here.
var schoolAbbreviationSynonyms = map[string][]string{
	"政大": {"國立政治大學"}, "國立政治大學": {"政大"},
	"清大": {"國立清華大學"}, "國立清華大學": {"清大"},
	"台大": {"國立臺灣大學"}, "臺大": {"國立臺灣大學"}, "國立臺灣大學": {"台大", "臺大"},
	"師大": {"國立臺灣師範大學"}, "國立臺灣師範大學": {"師大"},
	"成大": {"國立成功大學"}, "國立成功大學": {"成大"},
	"交大": {"國立陽明交通大學"}, "陽明交大": {"國立陽明交通大學"}, "國立陽明交通大學": {"交大", "陽明交大"},
	"中大": {"國立中央大學", "國立中山大學", "中原大學"},
	"海大": {"國立臺灣海洋大學"}, "國立臺灣海洋大學": {"海大"},
	"高師大": {"國立高雄師範大學"}, "國立高雄師範大學": {"高師大"},
	"彰師大": {"國立彰化師範大學"}, "國立彰化師範大學": {"彰師大"},
	"嘉大": {"國立嘉義大學"}, "國立嘉義大學": {"嘉大"},
	"高大": {"國立高雄大學"}, "國立高雄大學": {"高大"},
	"暨大": {"國立暨南國際大學"}, "國立暨南國際大學": {"暨大"},
	"北藝大": {"國立臺北藝術大學"}, "國立臺北藝術大學": {"北藝大"},
	"台藝大": {"國立臺灣藝術大學"}, "臺藝大": {"國立臺灣藝術大學"}, "國立臺灣藝術大學": {"台藝大", "臺藝大"},
	"宜大": {"國立宜蘭大學"}, "國立宜蘭大學": {"宜大"},
	"南大": {"國立臺南大學"}, "國立臺南大學": {"南大"},
	"北教大": {"國立臺北教育大學"}, "國立臺北教育大學": {"北教大"},
	"中教大": {"國立臺中教育大學"}, "國立臺中教育大學": {"中教大"},
	"屏大": {"國立屏東大學"}, "國立屏東大學": {"屏大"},
	"台科大": {"國立臺灣科技大學"}, "臺科大": {"國立臺灣科技大學"}, "國立臺灣科技大學": {"台科大", "臺科大"},
	"雲科大": {"國立雲林科技大學"}, "國立雲林科技大學": {"雲科大"},
	"屏科大": {"國立屏東科技大學"}, "國立屏東科技大學": {"屏科大"},
	"北科大": {"國立臺北科技大學"}, "國立臺北科技大學": {"北科大"},
	"虎科大": {"國立虎尾科技大學"}, "國立虎尾科技大學": {"虎科大"},
	"高餐大": {"國立高雄餐旅大學"}, "高餐": {"國立高雄餐旅大學"}, "國立高雄餐旅大學": {"高餐大", "高餐"},
	"台中科大": {"國立臺中科技大學"}, "中科大": {"國立臺中科技大學"}, "國立臺中科技大學": {"台中科大", "中科大"},
	"北商大": {"國立臺北商業大學"}, "國立臺北商業大學": {"北商大"},
	"高科大": {"國立高雄科技大學"}, "國立高雄科技大學": {"高科大"},
	"輔大": {"輔仁大學"}, "輔仁大學": {"輔大"},
	"淡大": {"淡江大學"}, "淡江大學": {"淡大"},
	"高醫": {"高雄醫學大學"}, "高醫大": {"高雄醫學大學"}, "高雄醫學大學": {"高醫", "高醫大"},
	"北醫": {"臺北醫學大學"}, "北醫大": {"臺北醫學大學"}, "臺北醫學大學": {"北醫", "北醫大"},
	"亞大": {"亞洲大學"}, "亞洲大學": {"亞大"},
}

// searchSynonyms merges the two maps above; since neither has overlapping
// keys today a plain union is enough (last-write-wins would apply if they
// ever did overlap).
var searchSynonyms = mergeSynonyms(taiVariantSynonyms, schoolAbbreviationSynonyms)

func mergeSynonyms(maps ...map[string][]string) map[string][]string {
	out := make(map[string][]string)
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// cjkTypoTolerance lowers Meilisearch's typo-tolerance word-length
// thresholds (default: oneTypo at 5 letters, twoTypos at 9) way down — those
// defaults are calibrated for Latin alphabetic words and effectively never
// fire for Chinese queries, where a 2-4 character term is already a normal
// full search phrase, not a fragment of a longer word. Without this, a
// search only ever matched exact character sequences, which read as "too
// strict" for CJK input despite typo tolerance being nominally "on".
var cjkTypoTolerance = map[string]any{
	"minWordSizeForTypos": map[string]int{"oneTypo": 2, "twoTypos": 4},
}

func (c *Client) EnsureIndexes(ctx context.Context) error {
	settings := map[string]map[string]any{
		IndexSchools: {
			"searchableAttributes": []string{"school_name", "school_code"},
			"filterableAttributes": []string{"institution_type", "is_active"},
			"synonyms":             searchSynonyms,
			"typoTolerance":        cjkTypoTolerance,
		},
		IndexPrograms: {
			"searchableAttributes": []string{"admission_program_name", "school_name", "program_identifier", "special_talent_target"},
			"filterableAttributes": []string{"academic_year", "school_code"},
			"synonyms":             searchSynonyms,
			"typoTolerance":        cjkTypoTolerance,
		},
		IndexExperiences: {
			"searchableAttributes": []string{"title", "snippet"},
			"synonyms":             searchSynonyms,
			"typoTolerance":        cjkTypoTolerance,
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
