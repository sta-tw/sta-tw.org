// Package obs holds the process-wide Prometheus metrics for the API and workers.
// It is the only package that imports the Prometheus client; everything else
// receives small callbacks or interfaces.
package obs

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
)

// Registry is the shared registry. It carries the Go runtime and process
// collectors plus everything registered below.
var Registry = prometheus.NewRegistry()

var (
	httpRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "sta_http_requests_total",
		Help: "HTTP requests handled by the API, by method, route pattern and status.",
	}, []string{"method", "route", "status"})

	httpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "sta_http_request_duration_seconds",
		Help:    "HTTP request latency in seconds, by method and route pattern.",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	}, []string{"method", "route"})

	httpResponseBytes = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "sta_http_response_bytes",
		Help:    "HTTP response body size in bytes, by route pattern.",
		Buckets: prometheus.ExponentialBuckets(128, 4, 8),
	}, []string{"route"})

	outboxDepth = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sta_outbox_pending",
		Help: "Pending items in a durable outbox, as last reported by its worker.",
	}, []string{"outbox"})

	outboxProcessed = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "sta_outbox_processed_total",
		Help: "Outbox items processed by a worker, by outbox and outcome (sent|failed|abandoned).",
	}, []string{"outbox", "outcome"})

	brokerPublishes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "sta_broker_publishes_total",
		Help: "AMQP publishes attempted by the API, by outcome (ok|error).",
	}, []string{"outcome"})

	workerLoops = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "sta_worker_loops_total",
		Help: "Worker processing-loop iterations, by worker and outcome (ok|error).",
	}, []string{"worker", "outcome"})
)

func init() {
	Registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		httpRequests, httpDuration, httpResponseBytes,
		outboxDepth, outboxProcessed, brokerPublishes, workerLoops,
	)
}

// Handler serves the shared registry at /metrics.
func Handler() http.Handler {
	return promhttp.HandlerFor(Registry, promhttp.HandlerOpts{Registry: Registry})
}

// ObserveHTTP matches httpapi.RequestObserver and records one finished request.
func ObserveHTTP(method, route string, status int, duration time.Duration, responseBytes int64) {
	code := statusClass(status)
	httpRequests.WithLabelValues(method, route, code).Inc()
	httpDuration.WithLabelValues(method, route).Observe(duration.Seconds())
	if responseBytes >= 0 {
		httpResponseBytes.WithLabelValues(route).Observe(float64(responseBytes))
	}
}

func statusClass(status int) string {
	switch {
	case status >= 500:
		return "5xx"
	case status >= 400:
		return "4xx"
	case status >= 300:
		return "3xx"
	case status >= 200:
		return "2xx"
	default:
		return "1xx"
	}
}

// SetOutboxDepth reports the current pending count for a named outbox.
func SetOutboxDepth(outbox string, pending int) {
	outboxDepth.WithLabelValues(outbox).Set(float64(pending))
}

// OutboxProcessed counts one processed outbox item.
func OutboxProcessed(outbox, outcome string) {
	outboxProcessed.WithLabelValues(outbox, outcome).Inc()
}

// BrokerPublish counts one AMQP publish attempt.
func BrokerPublish(err error) {
	outcome := "ok"
	if err != nil {
		outcome = "error"
	}
	brokerPublishes.WithLabelValues(outcome).Inc()
}

// WorkerLoop counts one worker loop iteration.
func WorkerLoop(worker string, err error) {
	outcome := "ok"
	if err != nil {
		outcome = "error"
	}
	workerLoops.WithLabelValues(worker, outcome).Inc()
}

// StartCollector runs fn every interval until ctx is done; use it to periodically
// refresh a gauge (e.g. outbox depth) from a worker.
func StartCollector(ctx context.Context, interval time.Duration, fn func(context.Context)) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				fn(ctx)
			}
		}
	}()
}
