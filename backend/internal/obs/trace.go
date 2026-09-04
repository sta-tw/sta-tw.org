package obs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// This is deliberately not the OpenTelemetry SDK. It carries a single W3C
// `traceparent` value (version 00) end to end — HTTP request, extraction job,
// Python worker, result callback — so one trace_id ties the logs together.
// Swapping in the real SDK later only means replacing this file; the
// propagation points already speak traceparent.

type traceContextKey struct{}

// TraceContext is the parsed pieces of a traceparent plus its wire form.
type TraceContext struct {
	TraceID     string // 32 lowercase hex
	SpanID      string // 16 lowercase hex (this hop)
	Traceparent string // full "00-<trace>-<span>-01"
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// rand failure is exceptional; a zeroed value is still a valid,
		// if useless, id and keeps the pipeline moving.
		return strings.Repeat("0", n*2)
	}
	return hex.EncodeToString(b)
}

// NewTrace starts a fresh trace with its own root span.
func NewTrace() TraceContext {
	return childOf(randomHex(16))
}

func childOf(traceID string) TraceContext {
	span := randomHex(8)
	return TraceContext{
		TraceID:     traceID,
		SpanID:      span,
		Traceparent: "00-" + traceID + "-" + span + "-01",
	}
}

// ParseTraceparent extracts a trace from an inbound header. It accepts only
// version 00 with 32/16 hex fields; anything else starts a fresh trace so a
// malformed upstream value never poisons correlation.
func ParseTraceparent(header string) (TraceContext, bool) {
	parts := strings.Split(strings.TrimSpace(header), "-")
	if len(parts) != 4 || parts[0] != "00" {
		return TraceContext{}, false
	}
	traceID, parentSpan := strings.ToLower(parts[1]), strings.ToLower(parts[2])
	if !isHex(traceID, 32) || !isHex(parentSpan, 16) || traceID == strings.Repeat("0", 32) {
		return TraceContext{}, false
	}
	// New span id for this hop, same trace.
	return childOf(traceID), true
}

func isHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// TraceFromInbound returns the continued trace for an inbound header, or a new
// one when the header is absent or malformed.
func TraceFromInbound(header string) TraceContext {
	if tc, ok := ParseTraceparent(header); ok {
		return tc
	}
	return NewTrace()
}

// WithTrace stores tc on ctx; TraceFromContext reads it back.
func WithTrace(ctx context.Context, tc TraceContext) context.Context {
	return context.WithValue(ctx, traceContextKey{}, tc)
}

func TraceFromContext(ctx context.Context) (TraceContext, bool) {
	tc, ok := ctx.Value(traceContextKey{}).(TraceContext)
	return tc, ok
}

// TraceparentFromContext is the wire value to forward on an outbound call or
// embed in a job, or "" when the context carries no trace.
func TraceparentFromContext(ctx context.Context) string {
	if tc, ok := TraceFromContext(ctx); ok {
		return tc.Traceparent
	}
	return ""
}

// TraceIDFromContext is the value to log, or "" when absent.
func TraceIDFromContext(ctx context.Context) string {
	if tc, ok := TraceFromContext(ctx); ok {
		return tc.TraceID
	}
	return ""
}
