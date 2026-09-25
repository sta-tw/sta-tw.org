package admin

import "testing"

func TestAuditCursorRoundTrip(t *testing.T) {
	for _, id := range []int64{1, 42, 9_999_999_999} {
		token := encodeAuditCursor(id)
		got, err := decodeAuditCursor(token)
		if err != nil {
			t.Fatalf("decode(%q): %v", token, err)
		}
		if got == nil || *got != id {
			t.Fatalf("round trip: got %v want %d", got, id)
		}
	}
}

func TestDecodeAuditCursorEmpty(t *testing.T) {
	got, err := decodeAuditCursor("")
	if err != nil || got != nil {
		t.Fatalf("empty cursor: got %v err %v, want nil/nil", got, err)
	}
}

func TestDecodeAuditCursorGarbage(t *testing.T) {
	for _, token := range []string{"!!!", "bm90LWFuLWludA"} { // "not-an-int"
		if _, err := decodeAuditCursor(token); err == nil {
			t.Fatalf("decode(%q) = nil error, want failure", token)
		}
	}
}
