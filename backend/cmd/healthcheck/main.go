// Command healthcheck performs a single HTTP GET against the local API and exits
// 0 on a 2xx response, 1 otherwise. It exists so container orchestrators can
// probe the API image, which is built on distroless and ships no shell or curl.
//
// Usage:
//
//	healthcheck [path]
//
// The target is http://127.0.0.1:<port><path>, where <port> is taken from the
// port component of STA_HTTP_ADDR (default 8080) and <path> defaults to
// "/healthz". Set STA_HEALTHCHECK_URL to override the whole URL.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	target := os.Getenv("STA_HEALTHCHECK_URL")
	if target == "" {
		path := "/healthz"
		if len(args) > 0 && args[0] != "" {
			path = args[0]
		}
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		target = fmt.Sprintf("http://127.0.0.1:%s%s", httpPort(), path)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d from %s", response.StatusCode, target)
	}
	return nil
}

func httpPort() string {
	addr := os.Getenv("STA_HTTP_ADDR")
	if addr == "" {
		return "8080"
	}
	if idx := strings.LastIndex(addr, ":"); idx >= 0 && idx < len(addr)-1 {
		return addr[idx+1:]
	}
	return "8080"
}
