package sse

import (
	"reflect"
	"testing"
)

func TestParseChannelTopics(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{"empty defaults to lounge", "", []string{"chat:lounge"}},
		{"single", "study", []string{"chat:study"}},
		{"multi trims and dedups", " lounge , study ,study,", []string{"chat:lounge", "chat:study"}},
		{"uppercase folded", "STUDY", []string{"chat:study"}},
		{"rejects bad chars, falls back", "a/b, c d", []string{"chat:lounge"}},
		{"rejects overlong, keeps valid", "toolongtoolongtoolongtoolongtoolongX,ok", []string{"chat:ok"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseChannelTopics(tc.raw); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseChannelTopics(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}

	// The subscription count is capped.
	many := ""
	for i := 0; i < 20; i++ {
		many += string(rune('a'+i)) + ","
	}
	if got := parseChannelTopics(many); len(got) != maxChannelSubscriptions {
		t.Fatalf("cap not applied: got %d topics", len(got))
	}
}
