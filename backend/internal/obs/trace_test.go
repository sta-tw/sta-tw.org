package obs

import (
	"context"
	"strings"
	"testing"
)

func TestNewTraceShape(t *testing.T) {
	tc := NewTrace()
	if len(tc.TraceID) != 32 || len(tc.SpanID) != 16 {
		t.Fatalf("ids wrong length: %+v", tc)
	}
	if tc.Traceparent != "00-"+tc.TraceID+"-"+tc.SpanID+"-01" {
		t.Fatalf("traceparent malformed: %q", tc.Traceparent)
	}
}

func TestParseTraceparentContinuesTrace(t *testing.T) {
	upstream := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	tc, ok := ParseTraceparent(upstream)
	if !ok {
		t.Fatal("valid traceparent rejected")
	}
	if tc.TraceID != "0af7651916cd43dd8448eb211c80319c" {
		t.Fatalf("trace id not carried: %q", tc.TraceID)
	}
	if tc.SpanID == "b7ad6b7169203331" {
		t.Fatal("span id should be regenerated for this hop")
	}
}

func TestParseTraceparentRejectsBad(t *testing.T) {
	for _, bad := range []string{
		"",
		"garbage",
		"01-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01", // version != 00
		"00-tooshort-b7ad6b7169203331-01",
		"00-" + strings.Repeat("0", 32) + "-b7ad6b7169203331-01", // all-zero trace id
		"00-0af7651916cd43dd8448eb211c80319c-XYZ-01",
	} {
		if _, ok := ParseTraceparent(bad); ok {
			t.Fatalf("accepted bad traceparent %q", bad)
		}
	}
}

func TestTraceFromInboundFallsBack(t *testing.T) {
	tc := TraceFromInbound("nonsense")
	if len(tc.TraceID) != 32 {
		t.Fatalf("fallback trace malformed: %+v", tc)
	}
}

func TestContextRoundTrip(t *testing.T) {
	tc := NewTrace()
	ctx := WithTrace(context.Background(), tc)
	if got := TraceIDFromContext(ctx); got != tc.TraceID {
		t.Fatalf("trace id round trip: %q != %q", got, tc.TraceID)
	}
	if got := TraceparentFromContext(ctx); got != tc.Traceparent {
		t.Fatalf("traceparent round trip: %q != %q", got, tc.Traceparent)
	}
	if TraceIDFromContext(context.Background()) != "" {
		t.Fatal("empty context should yield empty trace id")
	}
}
