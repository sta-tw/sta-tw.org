package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"sta-backend/internal/auth"
)

type fakeGrantStore struct {
	ciphertext []byte
	scope      string
	deleted    bool
}

func (f *fakeGrantStore) SaveCalendarGrant(context.Context, uuid.UUID, string, []byte, string) error {
	return errors.New("not used")
}

func (f *fakeGrantStore) GetCalendarGrant(context.Context, uuid.UUID, string) ([]byte, string, error) {
	if f.ciphertext == nil {
		return nil, "", auth.ErrNotFound
	}
	return f.ciphertext, f.scope, nil
}

func (f *fakeGrantStore) DeleteCalendarGrant(context.Context, uuid.UUID, string) error {
	f.deleted = true
	return nil
}

type fakeEventLinkStore struct {
	links map[string]string // externalID -> googleEventID
}

func (f *fakeEventLinkStore) SaveEventLink(_ context.Context, _ uuid.UUID, externalID, googleEventID string) error {
	if f.links == nil {
		f.links = map[string]string{}
	}
	f.links[externalID] = googleEventID
	return nil
}

func (f *fakeEventLinkStore) GetEventLink(_ context.Context, _ uuid.UUID, externalID string) (string, error) {
	googleEventID, ok := f.links[externalID]
	if !ok {
		return "", ErrEventNotLinked
	}
	return googleEventID, nil
}

func (f *fakeEventLinkStore) DeleteEventLink(_ context.Context, _ uuid.UUID, externalID string) error {
	delete(f.links, externalID)
	return nil
}

func (f *fakeEventLinkStore) ListEventLinks(context.Context, uuid.UUID) ([]string, error) {
	ids := make([]string, 0, len(f.links))
	for id := range f.links {
		ids = append(ids, id)
	}
	return ids, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
}

func newTestService(t *testing.T, grants *fakeGrantStore, transport roundTripFunc) *Service {
	t.Helper()
	service, _ := newTestServiceWithLinks(t, grants, &fakeEventLinkStore{}, transport)
	return service
}

func newTestServiceWithLinks(t *testing.T, grants *fakeGrantStore, links *fakeEventLinkStore, transport roundTripFunc) (*Service, *fakeEventLinkStore) {
	t.Helper()
	cipher, err := auth.NewFieldCipher(make([]byte, 32))
	if err != nil {
		t.Fatalf("NewFieldCipher() error = %v", err)
	}
	ciphertext, err := cipher.Seal("test-refresh-token")
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}
	grants.ciphertext = ciphertext
	grants.scope = "openid " + auth.CalendarScope
	service, err := NewService(grants, links, cipher, "client-id", "client-secret")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	service.httpClient = &http.Client{Transport: transport}
	return service, links
}

func TestCreateEventsReturnsErrNotLinkedWithoutAGrant(t *testing.T) {
	grants := &fakeGrantStore{}
	service := newTestService(t, grants, nil)
	grants.ciphertext = nil // undo newTestService's default grant

	created, err := service.CreateEvents(context.Background(), uuid.New(), []EventInput{{Title: "x", ISOStart: "2026-10-01", ISOEnd: "2026-10-01"}})
	if !errors.Is(err, ErrNotLinked) {
		t.Fatalf("CreateEvents() error = %v, want ErrNotLinked", err)
	}
	if created != 0 {
		t.Fatalf("CreateEvents() created = %d, want 0", created)
	}
}

func TestCreateEventsInsertsEachEventWithExclusiveEndDate(t *testing.T) {
	var insertedBodies []map[string]any
	grants := &fakeGrantStore{}
	service := newTestService(t, grants, func(r *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(r.URL.Host, "oauth2.googleapis.com"):
			return jsonResponse(http.StatusOK, `{"access_token":"access-token","token_type":"Bearer","expires_in":3600}`), nil
		case r.URL.String() == eventsEndpoint:
			if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
				t.Fatalf("insert request Authorization = %q, want bearer access token", got)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode insert body: %v", err)
			}
			insertedBodies = append(insertedBodies, body)
			return jsonResponse(http.StatusOK, `{"id":"evt"}`), nil
		default:
			t.Fatalf("unexpected request to %s", r.URL.String())
			return nil, nil
		}
	})

	events := []EventInput{
		{Title: "簡章公告", ISOStart: "2026-10-01", ISOEnd: "2026-10-01", Details: "single day"},
		{Title: "報名期間", ISOStart: "2026-10-07", ISOEnd: "2026-10-13", Details: "ranged"},
	}
	created, err := service.CreateEvents(context.Background(), uuid.New(), events)
	if err != nil {
		t.Fatalf("CreateEvents() error = %v", err)
	}
	if created != 2 {
		t.Fatalf("CreateEvents() created = %d, want 2", created)
	}
	if len(insertedBodies) != 2 {
		t.Fatalf("insert calls = %d, want 2", len(insertedBodies))
	}

	single := insertedBodies[0]["start"].(map[string]any)["date"]
	singleEnd := insertedBodies[0]["end"].(map[string]any)["date"]
	if single != "2026-10-01" || singleEnd != "2026-10-02" {
		t.Fatalf("single-day event start/end = %v/%v, want 2026-10-01/2026-10-02", single, singleEnd)
	}

	ranged := insertedBodies[1]["start"].(map[string]any)["date"]
	rangedEnd := insertedBodies[1]["end"].(map[string]any)["date"]
	if ranged != "2026-10-07" || rangedEnd != "2026-10-14" {
		t.Fatalf("ranged event start/end = %v/%v, want 2026-10-07/2026-10-14 (exclusive end from ISOEnd, not ISOStart)", ranged, rangedEnd)
	}
}

func TestCreateEventsDeletesGrantAndReturnsErrNotLinkedOnRevocation(t *testing.T) {
	grants := &fakeGrantStore{}
	service := newTestService(t, grants, func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Host, "oauth2.googleapis.com") {
			return jsonResponse(http.StatusOK, `{"access_token":"access-token","token_type":"Bearer","expires_in":3600}`), nil
		}
		return jsonResponse(http.StatusBadRequest, `{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`), nil
	})

	_, err := service.CreateEvents(context.Background(), uuid.New(), []EventInput{{Title: "x", ISOStart: "2026-10-01", ISOEnd: "2026-10-01"}})
	if !errors.Is(err, ErrNotLinked) {
		t.Fatalf("CreateEvents() error = %v, want ErrNotLinked", err)
	}
	if !grants.deleted {
		t.Fatal("CreateEvents() did not delete the revoked grant")
	}
}

// TestCreateEventsDeletesGrantAndReturnsErrNotLinkedOnInsufficientScope
// covers a stored grant whose refresh token is valid but was never actually
// authorized for calendar.events (e.g. Google silently dropped that
// sensitive scope for a non-test-listed account while our OAuth consent
// screen is in Testing status — a real incident this reproduces exactly).
// The token refresh succeeds; the Calendar API insert itself is what
// reports insufficientPermissions.
func TestCreateEventsDeletesGrantAndReturnsErrNotLinkedOnInsufficientScope(t *testing.T) {
	grants := &fakeGrantStore{}
	service := newTestService(t, grants, func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Host, "oauth2.googleapis.com") {
			return jsonResponse(http.StatusOK, `{"access_token":"access-token","token_type":"Bearer","expires_in":3600}`), nil
		}
		return jsonResponse(http.StatusForbidden, `{"error":{"code":403,"message":"Request had insufficient authentication scopes.","status":"PERMISSION_DENIED","details":[{"reason":"ACCESS_TOKEN_SCOPE_INSUFFICIENT"}],"errors":[{"reason":"insufficientPermissions"}]}}`), nil
	})

	_, err := service.CreateEvents(context.Background(), uuid.New(), []EventInput{{Title: "x", ISOStart: "2026-10-01", ISOEnd: "2026-10-01"}})
	if !errors.Is(err, ErrNotLinked) {
		t.Fatalf("CreateEvents() error = %v, want ErrNotLinked", err)
	}
	if !grants.deleted {
		t.Fatal("CreateEvents() did not delete the scope-insufficient grant")
	}
}

// TestCreateEventsDeletesGrantAndReturnsErrNotLinkedOnClientRotation covers
// the case where the token *refresh* call itself fails (e.g. our Google
// OAuth client_id/secret was rotated after the grant was captured, so
// Google's token endpoint now rejects the stored refresh token as
// invalid_client) rather than the later Calendar API call. oauth2.Transport
// refreshes transparently before the request ever reaches Calendar, so this
// surfaces as a *oauth2.RetrieveError from client.Do, not an HTTP response —
// a different code path than the invalid_grant case above, and one that
// must be recognized the same way (delete the now-dead grant, ask for
// re-auth) instead of falling through to a bare 502.
func TestCreateEventsDeletesGrantAndReturnsErrNotLinkedOnClientRotation(t *testing.T) {
	grants := &fakeGrantStore{}
	service := newTestService(t, grants, func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Host, "oauth2.googleapis.com") {
			return jsonResponse(http.StatusBadRequest, `{"error":"invalid_client","error_description":"The OAuth client was not found."}`), nil
		}
		t.Fatal("CreateEvents() called the Calendar API despite a failed token refresh")
		return nil, nil
	})

	_, err := service.CreateEvents(context.Background(), uuid.New(), []EventInput{{Title: "x", ISOStart: "2026-10-01", ISOEnd: "2026-10-01"}})
	if !errors.Is(err, ErrNotLinked) {
		t.Fatalf("CreateEvents() error = %v, want ErrNotLinked", err)
	}
	if !grants.deleted {
		t.Fatal("CreateEvents() did not delete the grant orphaned by the client rotation")
	}
}

func TestCreateEventsStopsAtFirstFailureAndReportsPartialProgress(t *testing.T) {
	calls := 0
	grants := &fakeGrantStore{}
	service := newTestService(t, grants, func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Host, "oauth2.googleapis.com") {
			return jsonResponse(http.StatusOK, `{"access_token":"access-token","token_type":"Bearer","expires_in":3600}`), nil
		}
		calls++
		if calls == 1 {
			return jsonResponse(http.StatusOK, `{"id":"evt-1"}`), nil
		}
		return jsonResponse(http.StatusInternalServerError, `{"error":"server_error"}`), nil
	})

	events := []EventInput{
		{Title: "one", ISOStart: "2026-10-01", ISOEnd: "2026-10-01"},
		{Title: "two", ISOStart: "2026-10-02", ISOEnd: "2026-10-02"},
	}
	created, err := service.CreateEvents(context.Background(), uuid.New(), events)
	if err == nil {
		t.Fatal("CreateEvents() error = nil, want the second insert's failure")
	}
	if errors.Is(err, ErrNotLinked) {
		t.Fatalf("CreateEvents() error = %v, want a plain API failure, not ErrNotLinked", err)
	}
	if created != 1 {
		t.Fatalf("CreateEvents() created = %d, want 1 (first event should have succeeded)", created)
	}
}

func TestIsLinkedReflectsGrantPresence(t *testing.T) {
	grants := &fakeGrantStore{}
	service := newTestService(t, grants, nil)

	linked, err := service.IsLinked(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("IsLinked() error = %v", err)
	}
	if !linked {
		t.Fatal("IsLinked() = false, want true when a grant exists")
	}

	grants.ciphertext = nil
	linked, err = service.IsLinked(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("IsLinked() error = %v", err)
	}
	if linked {
		t.Fatal("IsLinked() = true, want false when no grant exists")
	}
}

func TestCreateEventsSavesLinkForEventsWithAnExternalID(t *testing.T) {
	grants := &fakeGrantStore{}
	service, links := newTestServiceWithLinks(t, grants, &fakeEventLinkStore{}, func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Host, "oauth2.googleapis.com") {
			return jsonResponse(http.StatusOK, `{"access_token":"access-token","token_type":"Bearer","expires_in":3600}`), nil
		}
		return jsonResponse(http.StatusOK, `{"id":"evt-123"}`), nil
	})
	accountID := uuid.New()

	events := []EventInput{
		{Title: "有 external_id", ISOStart: "2026-10-01", ISOEnd: "2026-10-01", ExternalID: "116-002-001:1"},
		{Title: "沒有 external_id", ISOStart: "2026-10-02", ISOEnd: "2026-10-02"},
	}
	created, err := service.CreateEvents(context.Background(), accountID, events)
	if err != nil {
		t.Fatalf("CreateEvents() error = %v", err)
	}
	if created != 2 {
		t.Fatalf("CreateEvents() created = %d, want 2", created)
	}
	if len(links.links) != 1 {
		t.Fatalf("links saved = %d, want 1 (only the event with an external_id)", len(links.links))
	}
	if googleID := links.links["116-002-001:1"]; googleID != "evt-123" {
		t.Fatalf("saved link google event id = %q, want evt-123", googleID)
	}
}

func TestDeleteEventRemovesGoogleEventAndLink(t *testing.T) {
	grants := &fakeGrantStore{}
	links := &fakeEventLinkStore{links: map[string]string{"116-002-001:1": "evt-123"}}
	var deletedURL string
	service, _ := newTestServiceWithLinks(t, grants, links, func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Host, "oauth2.googleapis.com") {
			return jsonResponse(http.StatusOK, `{"access_token":"access-token","token_type":"Bearer","expires_in":3600}`), nil
		}
		deletedURL = r.URL.String()
		if r.Method != http.MethodDelete {
			t.Fatalf("request method = %s, want DELETE", r.Method)
		}
		return jsonResponse(http.StatusNoContent, ""), nil
	})

	if err := service.DeleteEvent(context.Background(), uuid.New(), "116-002-001:1"); err != nil {
		t.Fatalf("DeleteEvent() error = %v", err)
	}
	if !strings.HasSuffix(deletedURL, "/evt-123") {
		t.Fatalf("deleted URL = %q, want it to target evt-123", deletedURL)
	}
	if _, ok := links.links["116-002-001:1"]; ok {
		t.Fatal("DeleteEvent() did not remove the link row")
	}
}

func TestDeleteEventTreatsAlreadyGoneAsSuccess(t *testing.T) {
	grants := &fakeGrantStore{}
	links := &fakeEventLinkStore{links: map[string]string{"x": "evt-already-gone"}}
	service, _ := newTestServiceWithLinks(t, grants, links, func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Host, "oauth2.googleapis.com") {
			return jsonResponse(http.StatusOK, `{"access_token":"access-token","token_type":"Bearer","expires_in":3600}`), nil
		}
		return jsonResponse(http.StatusNotFound, `{"error":{"code":404,"message":"Not Found"}}`), nil
	})

	if err := service.DeleteEvent(context.Background(), uuid.New(), "x"); err != nil {
		t.Fatalf("DeleteEvent() error = %v, want nil (404 from Google is treated as already removed)", err)
	}
	if _, ok := links.links["x"]; ok {
		t.Fatal("DeleteEvent() did not remove the link row after a 404")
	}
}

func TestDeleteEventReturnsErrEventNotLinkedWhenNeverAdded(t *testing.T) {
	grants := &fakeGrantStore{}
	service := newTestService(t, grants, nil)

	err := service.DeleteEvent(context.Background(), uuid.New(), "never-added")
	if !errors.Is(err, ErrEventNotLinked) {
		t.Fatalf("DeleteEvent() error = %v, want ErrEventNotLinked", err)
	}
}
