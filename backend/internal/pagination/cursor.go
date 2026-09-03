// Package pagination provides opaque keyset cursors for list endpoints.
//
// A cursor carries the sort timestamp and row id of the last item on the
// previous page. Queries keyset on the pair (sort_col, id) so the position is
// unique and stable even when rows are inserted concurrently — unlike LIMIT/
// OFFSET, which skips or repeats rows when the underlying set shifts and grows
// linearly more expensive as the offset increases.
package pagination

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Cursor is an opaque keyset position.
type Cursor struct {
	Time time.Time `json:"t"`
	ID   string    `json:"id"`
}

// ErrInvalidCursor is returned when a client-supplied cursor cannot be decoded.
var ErrInvalidCursor = errors.New("invalid cursor")

// Zero reports whether the cursor points at the start of the list.
func (c Cursor) Zero() bool { return c.ID == "" }

// Encode renders the cursor as a URL-safe opaque token.
func Encode(c Cursor) string {
	raw, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

// Decode parses a token produced by Encode. An empty string yields the zero
// Cursor and no error, meaning "from the beginning".
func Decode(token string) (Cursor, error) {
	if token == "" {
		return Cursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return Cursor{}, ErrInvalidCursor
	}
	var c Cursor
	if err := json.Unmarshal(raw, &c); err != nil || c.ID == "" || c.Time.IsZero() {
		return Cursor{}, ErrInvalidCursor
	}
	if _, err := uuid.Parse(c.ID); err != nil {
		return Cursor{}, ErrInvalidCursor
	}
	return c, nil
}

// UUID returns the cursor id parsed as a UUID. Callers that reach here have
// already passed Decode, so the parse cannot fail.
func (c Cursor) UUID() uuid.UUID {
	id, _ := uuid.Parse(c.ID)
	return id
}

// ClampLimit bounds a requested page size to [1, max], substituting def for
// non-positive input.
func ClampLimit(requested, def, max int) int {
	switch {
	case requested <= 0:
		return def
	case requested > max:
		return max
	default:
		return requested
	}
}

// Next builds the cursor a client should send to fetch the page after one that
// ended on (t, id). It returns "" when the page was not full, signalling that
// there are no more rows.
func Next(pageLen, limit int, t time.Time, id uuid.UUID) string {
	if pageLen < limit {
		return ""
	}
	return Encode(Cursor{Time: t, ID: id.String()})
}
