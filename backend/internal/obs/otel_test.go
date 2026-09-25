package obs

import (
	"context"
	"testing"

	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestInitTracingNoEndpointIsNoop(t *testing.T) {
	shutdown, err := InitTracing(context.Background(), TracingConfig{})
	if err != nil {
		t.Fatalf("InitTracing(empty) error = %v", err)
	}
	if TracingEnabled() {
		t.Fatal("TracingEnabled() true after a no-endpoint init")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("noop shutdown error = %v", err)
	}
}

func TestTraceContextFromSpan(t *testing.T) {
	traceID, _ := oteltrace.TraceIDFromHex("0123456789abcdef0123456789abcdef")
	spanID, _ := oteltrace.SpanIDFromHex("0123456789abcdef")

	sampled := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: oteltrace.FlagsSampled,
	})
	tc, ok := TraceContextFromSpan(sampled)
	if !ok {
		t.Fatal("expected a trace context for a valid sampled span")
	}
	if tc.TraceID != "0123456789abcdef0123456789abcdef" || tc.SpanID != "0123456789abcdef" {
		t.Fatalf("ids not carried through: %+v", tc)
	}
	if tc.Traceparent != "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01" {
		t.Fatalf("sampled traceparent = %q", tc.Traceparent)
	}

	unsampled := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{TraceID: traceID, SpanID: spanID})
	tc, ok = TraceContextFromSpan(unsampled)
	if !ok || tc.Traceparent != "00-0123456789abcdef0123456789abcdef-0123456789abcdef-00" {
		t.Fatalf("unsampled traceparent = %q (ok=%v)", tc.Traceparent, ok)
	}

	if _, ok := TraceContextFromSpan(oteltrace.SpanContext{}); ok {
		t.Fatal("empty span context should not yield a trace context")
	}
}
