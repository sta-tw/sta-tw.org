package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

// RunWorkerHealth serves GET /healthz (always 200) and GET /readyz (200, or 503
// when checkReady returns an error) on addr until ctx is cancelled. It is for
// the port-less worker processes so an orchestrator has something to probe.
// A blank addr disables it and returns nil immediately.
func RunWorkerHealth(ctx context.Context, addr string, logger *slog.Logger, checkReady func(context.Context) error) error {
	if addr == "" {
		<-ctx.Done()
		return nil
	}
	if logger == nil {
		logger = slog.Default()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if checkReady != nil {
			cctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()
			if err := checkReady(cctx); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"status":"not_ready"}`))
				return
			}
		}
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})

	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	errc := make(chan error, 1)
	go func() {
		logger.Info("worker health server listening", "addr", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
			return
		}
		errc <- nil
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}
