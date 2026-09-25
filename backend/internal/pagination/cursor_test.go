package pagination

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	want := Cursor{Time: time.Now().UTC().Truncate(time.Microsecond), ID: uuid.NewString()}
	got, err := Decode(Encode(want))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !got.Time.Equal(want.Time) || got.ID != want.ID {
		t.Fatalf("round trip mismatch: got %+v want %+v", got, want)
	}
}

func TestDecodeEmptyIsZero(t *testing.T) {
	c, err := Decode("")
	if err != nil {
		t.Fatalf("Decode(\"\"): %v", err)
	}
	if !c.Zero() {
		t.Fatalf("empty token should decode to zero cursor")
	}
}

func TestDecodeRejectsGarbage(t *testing.T) {
	for _, token := range []string{"not-base64!!", "eyJ0IjoiIn0", "e30"} {
		if _, err := Decode(token); err != ErrInvalidCursor {
			t.Fatalf("Decode(%q) err = %v, want ErrInvalidCursor", token, err)
		}
	}
}

func TestDecodeRejectsNonUUIDID(t *testing.T) {
	tok := Encode(Cursor{Time: time.Now(), ID: "1234"})
	if _, err := Decode(tok); err != ErrInvalidCursor {
		t.Fatalf("err = %v, want ErrInvalidCursor", err)
	}
}

func TestClampLimit(t *testing.T) {
	cases := []struct{ in, def, max, want int }{
		{0, 50, 100, 50},
		{-5, 50, 100, 50},
		{20, 50, 100, 20},
		{500, 50, 100, 100},
	}
	for _, c := range cases {
		if got := ClampLimit(c.in, c.def, c.max); got != c.want {
			t.Fatalf("ClampLimit(%d,%d,%d) = %d, want %d", c.in, c.def, c.max, got, c.want)
		}
	}
}

func TestNextEmptyOnShortPage(t *testing.T) {
	if got := Next(3, 10, time.Now(), uuid.New()); got != "" {
		t.Fatalf("Next on short page = %q, want empty", got)
	}
}

func TestNextSetOnFullPage(t *testing.T) {
	if got := Next(10, 10, time.Now(), uuid.New()); got == "" {
		t.Fatalf("Next on full page returned empty")
	}
}
