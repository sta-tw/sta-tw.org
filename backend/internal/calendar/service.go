// Package calendar creates events directly on a signed-in user's Google
// Calendar. It reuses the refresh token captured during Google OAuth login
// (see internal/auth's CalendarScope) instead of running its own consent
// flow — a user who has never granted calendar access simply has no grant
// row, and CreateEvents reports that as ErrNotLinked so the caller can send
// them through auth's OAuth bind flow first.
package calendar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"sta-backend/internal/auth"
)

// ErrNotLinked means the account has never granted calendar access, or the
// grant it had was revoked (Google's refresh call itself reports this as an
// invalid_grant error — the grant row is deleted in that case, so this is
// the same "please (re)authorize" signal either way).
var ErrNotLinked = errors.New("google calendar is not linked for this account")

// ErrEventNotLinked means there's no record of this account having added an
// event under that external_id — either it was never added, or it (and its
// link row) was already removed.
var ErrEventNotLinked = errors.New("no calendar event is linked for that id")

// EventLinkStore remembers which Google Calendar event a (account,
// external_id) pair created, so a later "remove from calendar" request
// knows which Google event to delete. external_id is caller-supplied and
// opaque to this package — the admissions frontend uses a program's
// timeline entry as its stable identifier, but nothing here assumes that.
type EventLinkStore interface {
	SaveEventLink(ctx context.Context, accountID uuid.UUID, externalID, googleEventID string) error
	GetEventLink(ctx context.Context, accountID uuid.UUID, externalID string) (googleEventID string, err error)
	DeleteEventLink(ctx context.Context, accountID uuid.UUID, externalID string) error
	ListEventLinks(ctx context.Context, accountID uuid.UUID) ([]string, error)
}

// EventInput is one all-day event to create. ISOStart/ISOEnd are inclusive
// "YYYY-MM-DD" dates (equal for a single-day event); the exclusive end date
// the Calendar API wants is computed from ISOEnd, not ISOStart, so a
// multi-day range (e.g. a registration window) survives intact. ExternalID
// is optional — when set, a successful insert is recorded under it so the
// event can later be removed via DeleteEvent; leave it blank for a
// fire-and-forget insert with no remove path.
type EventInput struct {
	Title      string
	ISOStart   string
	ISOEnd     string
	Details    string
	ExternalID string
}

const eventsEndpoint = "https://www.googleapis.com/calendar/v3/calendars/primary/events"

type Service struct {
	grants      auth.CalendarGrantStore
	links       EventLinkStore
	cipher      *auth.FieldCipher
	oauthConfig oauth2.Config
	httpClient  *http.Client
}

func NewService(grants auth.CalendarGrantStore, links EventLinkStore, cipher *auth.FieldCipher, clientID, clientSecret string) (*Service, error) {
	if grants == nil {
		return nil, errors.New("calendar service requires a grant store")
	}
	if links == nil {
		return nil, errors.New("calendar service requires an event link store")
	}
	if cipher == nil {
		return nil, errors.New("calendar service requires a field cipher")
	}
	if clientID == "" || clientSecret == "" {
		return nil, errors.New("calendar service requires a Google OAuth client id/secret")
	}
	return &Service{
		grants: grants,
		links:  links,
		cipher: cipher,
		oauthConfig: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint:     google.Endpoint,
		},
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}, nil
}

// ListLinkedEvents returns the external_ids this account currently has a
// calendar event recorded for — used by the frontend to show "已加入" /
// offer removal instead of "新增至日曆" for items already on the calendar.
func (s *Service) ListLinkedEvents(ctx context.Context, accountID uuid.UUID) ([]string, error) {
	return s.links.ListEventLinks(ctx, accountID)
}

// IsLinked reports whether accountID has a usable calendar grant, without
// spending a call against Google — used by the frontend to decide whether
// to send the visitor through OAuth first.
func (s *Service) IsLinked(ctx context.Context, accountID uuid.UUID) (bool, error) {
	_, _, err := s.grants.GetCalendarGrant(ctx, accountID, "google")
	if errors.Is(err, auth.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load calendar grant: %w", err)
	}
	return true, nil
}

// CreateEvents inserts each event into the account's primary Google
// Calendar, in order, stopping at the first failure. It returns how many
// were created even on error, so a partial batch isn't silently invisible
// to the caller.
func (s *Service) CreateEvents(ctx context.Context, accountID uuid.UUID, events []EventInput) (int, error) {
	if len(events) == 0 {
		return 0, nil
	}
	ciphertext, _, err := s.grants.GetCalendarGrant(ctx, accountID, "google")
	if errors.Is(err, auth.ErrNotFound) {
		return 0, ErrNotLinked
	}
	if err != nil {
		return 0, fmt.Errorf("load calendar grant: %w", err)
	}
	refreshToken, err := s.cipher.Open(ciphertext)
	if err != nil {
		return 0, fmt.Errorf("decrypt calendar refresh token: %w", err)
	}
	// Both the token refresh (via the ctx-supplied client) and the actual
	// event insert (via Base) must go through the same transport, or a test
	// double injected into s.httpClient only covers half the round trip.
	ctx = context.WithValue(ctx, oauth2.HTTPClient, s.httpClient)
	tokenSource := s.oauthConfig.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken})
	client := &http.Client{
		Transport: &oauth2.Transport{Source: tokenSource, Base: s.httpClient.Transport},
		Timeout:   s.httpClient.Timeout,
	}

	created := 0
	for _, event := range events {
		googleEventID, err := s.insertEvent(ctx, client, event)
		if err != nil {
			if isRevokedGrant(err) {
				_ = s.grants.DeleteCalendarGrant(ctx, accountID, "google")
				return created, ErrNotLinked
			}
			return created, err
		}
		created++
		if event.ExternalID != "" {
			if err := s.links.SaveEventLink(ctx, accountID, event.ExternalID, googleEventID); err != nil {
				// The Google event was created successfully; losing the
				// link just means this one won't offer a "remove" affordance
				// later, which isn't worth failing an otherwise-successful
				// batch over.
				continue
			}
		}
	}
	return created, nil
}

// DeleteEvent removes a previously-created event from the account's Google
// Calendar by its external_id, and forgets the link either way once Google
// confirms the event is gone (including if it was already gone — deleted
// directly in Google Calendar, say — which is treated as success, not an
// error, since the end state the caller wants is already true).
func (s *Service) DeleteEvent(ctx context.Context, accountID uuid.UUID, externalID string) error {
	googleEventID, err := s.links.GetEventLink(ctx, accountID, externalID)
	if errors.Is(err, ErrEventNotLinked) {
		return ErrEventNotLinked
	}
	if err != nil {
		return fmt.Errorf("load calendar event link: %w", err)
	}
	ciphertext, _, err := s.grants.GetCalendarGrant(ctx, accountID, "google")
	if errors.Is(err, auth.ErrNotFound) {
		// No grant left at all — nothing to delete on Google's side either
		// way; just drop our own now-meaningless link row.
		_ = s.links.DeleteEventLink(ctx, accountID, externalID)
		return ErrNotLinked
	}
	if err != nil {
		return fmt.Errorf("load calendar grant: %w", err)
	}
	refreshToken, err := s.cipher.Open(ciphertext)
	if err != nil {
		return fmt.Errorf("decrypt calendar refresh token: %w", err)
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, s.httpClient)
	tokenSource := s.oauthConfig.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken})
	client := &http.Client{
		Transport: &oauth2.Transport{Source: tokenSource, Base: s.httpClient.Transport},
		Timeout:   s.httpClient.Timeout,
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, eventsEndpoint+"/"+googleEventID, nil)
	if err != nil {
		return fmt.Errorf("build calendar delete request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		var retrieveErr *oauth2.RetrieveError
		if errors.As(err, &retrieveErr) {
			_ = s.grants.DeleteCalendarGrant(ctx, accountID, "google")
			return ErrNotLinked
		}
		return fmt.Errorf("call Google Calendar API: %w", err)
	}
	defer response.Body.Close()
	// 404/410 means the event is already gone on Google's side (removed by
	// hand, or a stale link from a re-run) — that's the outcome we want
	// either way, so it's success here, not an error to surface.
	if (response.StatusCode < 200 || response.StatusCode >= 300) &&
		response.StatusCode != http.StatusNotFound && response.StatusCode != http.StatusGone {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		if response.StatusCode == http.StatusUnauthorized {
			_ = s.grants.DeleteCalendarGrant(ctx, accountID, "google")
			return ErrNotLinked
		}
		return fmt.Errorf("Google Calendar API returned %s: %s", response.Status, string(body))
	}
	if err := s.links.DeleteEventLink(ctx, accountID, externalID); err != nil {
		return fmt.Errorf("delete calendar event link: %w", err)
	}
	return nil
}

type revokedGrantError struct{ detail string }

func (e revokedGrantError) Error() string { return "google calendar grant was revoked: " + e.detail }

func isRevokedGrant(err error) bool {
	var revoked revokedGrantError
	return errors.As(err, &revoked)
}

func (s *Service) insertEvent(ctx context.Context, client *http.Client, event EventInput) (string, error) {
	payload, err := json.Marshal(map[string]any{
		"summary":     event.Title,
		"description": event.Details,
		"start":       map[string]string{"date": event.ISOStart},
		// The Calendar API's all-day "date" end is exclusive, same
		// convention as the .ics fallback this replaces.
		"end": map[string]string{"date": nextDay(event.ISOEnd)},
	})
	if err != nil {
		return "", fmt.Errorf("encode calendar event: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, eventsEndpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build calendar request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		// The oauth2.Transport refreshes the access token transparently
		// before this request ever reaches Google Calendar, so a stale or
		// no-longer-valid refresh token (revoked by the user, or orphaned
		// by an OAuth client credential rotation on our side — the refresh
		// token was minted for the old client_id/secret and Google's token
		// endpoint rejects it as invalid_client under the new one) surfaces
		// here as a *oauth2.RetrieveError, not as an HTTP response status.
		var retrieveErr *oauth2.RetrieveError
		if errors.As(err, &retrieveErr) {
			return "", revokedGrantError{detail: retrieveErr.Error()}
		}
		return "", fmt.Errorf("call Google Calendar API: %w", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		var created struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(body, &created); err != nil || created.ID == "" {
			return "", fmt.Errorf("decode created calendar event: %w", err)
		}
		return created.ID, nil
	}
	// insufficientPermissions/ACCESS_TOKEN_SCOPE_INSUFFICIENT means the
	// stored grant genuinely never carried calendar.events — most often
	// because Google silently dropped that sensitive scope for an account
	// not allow-listed as a test user while our OAuth consent screen is in
	// Testing status (auth.Service.saveCalendarGrantIfPresent now checks
	// Google's returned scope before saving to catch this earlier, but an
	// already-saved bad grant from before that fix still needs to be
	// treated as unusable here). No amount of retrying an insert fixes a
	// token that was never actually authorized for Calendar, so this is
	// "please (re)link", the same as a revoked grant.
	if response.StatusCode == http.StatusUnauthorized || strings.Contains(string(body), "invalid_grant") ||
		strings.Contains(string(body), "insufficientPermissions") || strings.Contains(string(body), "ACCESS_TOKEN_SCOPE_INSUFFICIENT") {
		return "", revokedGrantError{detail: string(body)}
	}
	return "", fmt.Errorf("Google Calendar API returned %s: %s", response.Status, string(body))
}

func nextDay(isoDate string) string {
	parsed, err := time.Parse("2006-01-02", isoDate)
	if err != nil {
		return isoDate
	}
	return parsed.AddDate(0, 0, 1).Format("2006-01-02")
}
