package email

import (
	"strings"
	"testing"
)

func TestPasswordResetEmailUsesSiteBrandAndEscapesURLs(t *testing.T) {
	body := PasswordResetEmail(PasswordResetEmailData{
		LogoURL:  "https://sta.example/logo.svg?source=email&theme=light",
		ResetURL: "https://sta.example/reset-password?token=SECRET&from=email",
	})

	for _, want := range []string{
		"#d9edbf",
		"#ffe184",
		"#badaaf",
		"#363535",
		"特殊選才資源網",
		"重設你的密碼",
		"30 分鐘",
		"安全提醒",
		"logo.svg?source=email&amp;theme=light",
		"reset-password?token=SECRET&amp;from=email",
		`align="center" style="margin:32px auto 0;"`,
		`text-align:center;font-size:12px`,
		`border-radius:3px`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("PasswordResetEmail() missing %q", want)
		}
	}
}

func TestPasswordResetEmailFallsBackToTextBrandWithoutLogoURL(t *testing.T) {
	body := PasswordResetEmail(PasswordResetEmailData{ResetURL: "https://sta.example/reset-password"})

	if !strings.Contains(body, ">S.T.A</span>") {
		t.Fatal("PasswordResetEmail() did not render the text brand fallback")
	}
	if strings.Contains(body, "<img ") {
		t.Fatal("PasswordResetEmail() rendered an image without a logo URL")
	}
}
