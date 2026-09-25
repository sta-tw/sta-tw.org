// Command email-preview renders the production password-reset email to a
// standalone HTML file without connecting to SMTP or starting the API.
package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sta-backend/internal/email"
)

func main() {
	outputPath := flag.String(
		"out",
		"temporary-console/email-password-reset-preview.html",
		"output HTML file path, relative to the backend directory",
	)
	baseURL := flag.String("base-url", "https://sta-tw.org", "base URL used in the preview reset link")
	logoPath := flag.String("logo", "../frontend/public/logo.svg", "path to the existing SVG logo")
	flag.Parse()

	logo, err := os.ReadFile(*logoPath)
	if err != nil {
		fail("read logo", err)
	}

	resetURL := strings.TrimRight(strings.TrimSpace(*baseURL), "/") + "/reset-password?token=preview-token"
	htmlBody := email.PasswordResetEmail(email.PasswordResetEmailData{
		LogoURL:  "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(logo),
		ResetURL: resetURL,
	})

	if err := os.MkdirAll(filepath.Dir(*outputPath), 0o755); err != nil {
		fail("create preview directory", err)
	}
	if err := os.WriteFile(*outputPath, []byte(htmlBody), 0o644); err != nil {
		fail("write preview", err)
	}
	fmt.Printf("email preview written to %s\n", *outputPath)
}

func fail(action string, err error) {
	fmt.Fprintf(os.Stderr, "email-preview: %s: %v\n", action, err)
	os.Exit(1)
}
