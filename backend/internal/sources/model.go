package sources

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"sta-backend/internal/admissions"
)

var (
	ErrInvalid       = errors.New("admissions source input is invalid")
	ErrNotFound      = errors.New("admissions source not found")
	ErrConflict      = errors.New("admissions source already exists")
	ErrAdminRequired = errors.New("administrator role is required")
)

const (
	StatusCandidate = "candidate"
	StatusActive    = "active"
	StatusRejected  = "rejected"
	StatusExpired   = "expired"
)

type Evidence struct {
	URL     string `json:"url"`
	Locator string `json:"page_or_locator"`
	Text    string `json:"text"`
}

type Source struct {
	ID                    uuid.UUID  `json:"id"`
	SchoolCode            string     `json:"school_code"`
	AcademicYear          int        `json:"academic_year"`
	SourceURL             string     `json:"source_url"`
	NormalizedURL         string     `json:"normalized_url"`
	Hostname              string     `json:"hostname"`
	SourceType            string     `json:"source_type"`
	Status                string     `json:"status"`
	DecisionMode          string     `json:"decision_mode"`
	AffiliationConfidence string     `json:"affiliation_confidence"`
	DiscoveryMethod       string     `json:"discovery_method"`
	Evidence              []Evidence `json:"evidence"`
	FirstSeenAt           time.Time  `json:"first_seen_at"`
	LastSeenAt            time.Time  `json:"last_seen_at"`
	LastCrawledAt         *time.Time `json:"last_crawled_at,omitempty"`
	LastDiscoveryAt       *time.Time `json:"last_discovery_at,omitempty"`
	DiscoveryNeeded       bool       `json:"discovery_needed"`
	DiscoveryReason       string     `json:"discovery_reason,omitempty"`
	RejectedReason        string     `json:"rejected_reason,omitempty"`
	ManualNote            string     `json:"manual_note,omitempty"`
	CreatedBy             *uuid.UUID `json:"created_by,omitempty"`
	UpdatedBy             *uuid.UUID `json:"updated_by,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

type Input struct {
	SchoolCode            string     `json:"school_code"`
	AcademicYear          int        `json:"academic_year"`
	SourceURL             string     `json:"source_url"`
	SourceType            string     `json:"source_type"`
	Status                string     `json:"status"`
	DecisionMode          string     `json:"decision_mode"`
	AffiliationConfidence string     `json:"affiliation_confidence"`
	DiscoveryMethod       string     `json:"discovery_method"`
	Evidence              []Evidence `json:"evidence"`
	DiscoveryNeeded       bool       `json:"discovery_needed"`
	DiscoveryReason       string     `json:"discovery_reason"`
	RejectedReason        string     `json:"rejected_reason"`
	ManualNote            string     `json:"manual_note"`
}

type Query struct {
	SchoolCode   string
	AcademicYear int
	Status       string
	Limit        int
	Offset       int
}

type Repository interface {
	IsAdmin(context.Context, uuid.UUID) (bool, error)
	List(context.Context, uuid.UUID, Query) ([]Source, error)
	Create(context.Context, uuid.UUID, Input) (Source, error)
	Update(context.Context, uuid.UUID, uuid.UUID, Input) (Source, error)
}

func ValidateInput(input Input) (Input, error) {
	input.SchoolCode = strings.TrimSpace(input.SchoolCode)
	input.SourceURL = strings.TrimSpace(input.SourceURL)
	input.SourceType = strings.TrimSpace(input.SourceType)
	input.Status = strings.TrimSpace(input.Status)
	input.DecisionMode = strings.TrimSpace(input.DecisionMode)
	input.AffiliationConfidence = strings.TrimSpace(input.AffiliationConfidence)
	input.DiscoveryMethod = strings.TrimSpace(input.DiscoveryMethod)
	input.DiscoveryReason = strings.TrimSpace(input.DiscoveryReason)
	input.RejectedReason = strings.TrimSpace(input.RejectedReason)
	input.ManualNote = strings.TrimSpace(input.ManualNote)
	if input.AcademicYear < 100 || input.AcademicYear > 999 || len(input.SchoolCode) != 3 || input.SourceURL == "" {
		return Input{}, ErrInvalid
	}
	for _, character := range input.SchoolCode {
		if character < '0' || character > '9' {
			return Input{}, ErrInvalid
		}
	}
	if err := admissions.ValidateOfficialURL(input.SourceURL); err != nil {
		return Input{}, ErrInvalid
	}
	parsed, err := url.Parse(input.SourceURL)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return Input{}, ErrInvalid
	}
	if input.SourceType == "" {
		input.SourceType = "official_entry"
	}
	if !validSourceType(input.SourceType) || !validStatus(input.Status) && input.Status != "" {
		return Input{}, ErrInvalid
	}
	if input.Status == "" {
		input.Status = StatusCandidate
	}
	if input.DecisionMode == "" {
		input.DecisionMode = "agent"
	}
	if input.DecisionMode != "agent" && input.DecisionMode != "manual" {
		return Input{}, ErrInvalid
	}
	if input.AffiliationConfidence == "" {
		input.AffiliationConfidence = "unknown"
	}
	if input.AffiliationConfidence != "high" && input.AffiliationConfidence != "medium" && input.AffiliationConfidence != "low" && input.AffiliationConfidence != "unknown" {
		return Input{}, ErrInvalid
	}
	if input.DiscoveryMethod == "" {
		input.DiscoveryMethod = "manual"
	}
	if input.DiscoveryMethod != "official_link" && input.DiscoveryMethod != "search_discovery" && input.DiscoveryMethod != "page_link" && input.DiscoveryMethod != "manual" {
		return Input{}, ErrInvalid
	}
	if input.Status == StatusRejected && input.RejectedReason == "" {
		return Input{}, ErrInvalid
	}
	if len(input.Evidence) > 50 || len([]rune(input.SourceURL)) > 4000 || len([]rune(input.ManualNote)) > 4000 || len([]rune(input.DiscoveryReason)) > 1000 || len([]rune(input.RejectedReason)) > 2000 {
		return Input{}, ErrInvalid
	}
	if input.Evidence == nil {
		// Marshalling a nil slice yields JSON "null", which violates the
		// evidence-is-an-array check constraint.
		input.Evidence = []Evidence{}
	}
	for index := range input.Evidence {
		input.Evidence[index].URL = strings.TrimSpace(input.Evidence[index].URL)
		input.Evidence[index].Locator = strings.TrimSpace(input.Evidence[index].Locator)
		input.Evidence[index].Text = strings.TrimSpace(input.Evidence[index].Text)
		if input.Evidence[index].URL == "" || input.Evidence[index].Locator == "" || input.Evidence[index].Text == "" || len([]rune(input.Evidence[index].Locator)) > 500 || len([]rune(input.Evidence[index].Text)) > 4000 || admissions.ValidateOfficialURL(input.Evidence[index].URL) != nil {
			return Input{}, ErrInvalid
		}
	}
	return input, nil
}

func NormalizeURL(raw string) (string, string, error) {
	if err := admissions.ValidateOfficialURL(raw); err != nil {
		return "", "", ErrInvalid
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", "", ErrInvalid
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	normalized := parsed.String()
	return normalized, strings.ToLower(parsed.Hostname()), nil
}

func validSourceType(value string) bool {
	switch value {
	case "official_entry", "brochure", "announcement", "stage_notice", "result", "waitlist_notice", "unknown":
		return true
	default:
		return false
	}
}

func validStatus(value string) bool {
	switch value {
	case StatusCandidate, StatusActive, StatusRejected, StatusExpired:
		return true
	default:
		return false
	}
}
