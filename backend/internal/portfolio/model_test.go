package portfolio

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAdminFileQueryValidate(t *testing.T) {
	tests := []struct {
		name    string
		query   AdminFileQuery
		wantErr bool
	}{
		{name: "valid default page", query: AdminFileQuery{Limit: 50}},
		{name: "valid status and project", query: AdminFileQuery{Status: FileStatusPendingReview, ProjectID: uuid.New(), Limit: 100, Offset: 10000}},
		{name: "unknown status", query: AdminFileQuery{Status: "draft", Limit: 50}, wantErr: true},
		{name: "zero limit", query: AdminFileQuery{Limit: 0}, wantErr: true},
		{name: "limit too large", query: AdminFileQuery{Limit: 101}, wantErr: true},
		{name: "negative offset", query: AdminFileQuery{Limit: 50, Offset: -1}, wantErr: true},
		{name: "offset too large", query: AdminFileQuery{Limit: 50, Offset: 10001}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.query.Validate()
			if tt.wantErr && err != ErrInvalidQuery {
				t.Fatalf("Validate() error = %v, want %v", err, ErrInvalidQuery)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestPortfolioJSONDoesNotExposePrivateFields(t *testing.T) {
	accountID := uuid.New()
	file := File{
		ID:               uuid.New(),
		ProjectID:        uuid.New(),
		VersionNumber:    1,
		OriginalFileName: "portfolio.pdf",
		StorageKey:       "portfolio/private/object",
		MimeType:         "application/pdf",
		FileSizeBytes:    128,
		SHA256Hex:        strings.Repeat("a", 64),
		Status:           FileStatusHidden,
		OwnerAccountID:   accountID,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	payload, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	encoded := string(payload)
	for _, privateField := range []string{"storage_key", "owner_account_id"} {
		if strings.Contains(encoded, privateField) {
			t.Fatalf("JSON contains private field %q: %s", privateField, encoded)
		}
	}

	eventPayload, err := json.Marshal(FileEvent{ActorAccountID: &accountID})
	if err != nil {
		t.Fatalf("json.Marshal(event) error = %v", err)
	}
	if strings.Contains(string(eventPayload), "actor_account_id") {
		t.Fatalf("event JSON contains private actor field: %s", eventPayload)
	}
}
