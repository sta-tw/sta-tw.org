package obs

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const tracerName = "sta-backend"

var tracingActive atomic.Bool

// TracingConfig drives InitTracing. A blank Endpoint disables the exporter and
// the request path falls back to the dependency-free traceparent propagation in
// trace.go.
type TracingConfig struct {
	Endpoint    string  // OTLP/HTTP endpoint, e.g. "http://collector:4318" or "collector:4318"
	Insecure    bool    // send over plain HTTP when Endpoint carries no scheme
	ServiceName string  // resource service.name; defaults to "sta-backend"
	Environment string  // resource deployment.environment, when set
	SampleRatio float64 // parent-based ratio sampler; <=0 never, >=1 always
}

// InitTracing wires an OTLP/HTTP span exporter into the global OpenTelemetry
// tracer provider and installs the W3C trace-context + baggage propagators. It
// returns a shutdown func that flushes pending spans; when tracing is disabled
// the shutdown is a no-op. Call once at startup.
func InitTracing(ctx context.Context, cfg TracingConfig) (func(context.Context) error, error) {
	noop := func(context.Context) error { return nil }
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		return noop, nil
	}

	var opts []otlptracehttp.Option
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		opts = append(opts, otlptracehttp.WithEndpointURL(endpoint))
	} else {
		opts = append(opts, otlptracehttp.WithEndpoint(endpoint))
		if cfg.Insecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
	}

	exporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return noop, fmt.Errorf("otlp trace exporter: %w", err)
	}

	serviceName := strings.TrimSpace(cfg.ServiceName)
	if serviceName == "" {
		serviceName = tracerName
	}

	ratio := cfg.SampleRatio
	var sampler sdktrace.Sampler
	switch {
	case ratio >= 1:
		sampler = sdktrace.ParentBased(sdktrace.AlwaysSample())
	case ratio <= 0:
		sampler = sdktrace.ParentBased(sdktrace.NeverSample())
	default:
		sampler = sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(buildResource(serviceName, cfg.Environment)),
		sdktrace.WithSampler(sampler),
	)

	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	tracingActive.Store(true)

	return provider.Shutdown, nil
}

func buildResource(serviceName, env string) *resource.Resource {
	attrs := []attribute.KeyValue{semconv.ServiceName(serviceName)}
	if env != "" {
		attrs = append(attrs, semconv.DeploymentEnvironment(env))
	}
	res, err := resource.Merge(resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL, attrs...))
	if err != nil {
		return resource.Default()
	}
	return res
}

// TracingEnabled reports whether InitTracing installed a real exporter.
func TracingEnabled() bool { return tracingActive.Load() }

// Tracer returns the package tracer from the global provider.
func Tracer() oteltrace.Tracer { return otel.Tracer(tracerName) }

// TraceContextFromSpan renders an OTel span context into the local TraceContext
// used for log correlation and cross-process traceparent propagation.
func TraceContextFromSpan(sc oteltrace.SpanContext) (TraceContext, bool) {
	if !sc.HasTraceID() || !sc.HasSpanID() {
		return TraceContext{}, false
	}
	flags := "00"
	if sc.IsSampled() {
		flags = "01"
	}
	traceID := sc.TraceID().String()
	spanID := sc.SpanID().String()
	return TraceContext{
		TraceID:     traceID,
		SpanID:      spanID,
		Traceparent: "00-" + traceID + "-" + spanID + "-" + flags,
	}, true
}
