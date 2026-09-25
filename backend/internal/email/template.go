package email

import (
	"html"
	"strings"
)

// securityEmailData is the shared content for the branded, table-based
// layout used by both the password-reset and account-activation emails —
// same visual language as the website (ink, pale green, warm yellow), kept
// as inline-styled tables so it renders consistently in webmail/desktop
// clients that strip <style> blocks or ignore remote CSS.
type securityEmailData struct {
	LogoURL      string
	Title        string // <title>
	Preheader    string // hidden preview text
	Eyebrow      string // small label above the heading, e.g. "密碼重設"
	Heading      string
	Paragraphs   []string
	ButtonLabel  string
	ButtonURL    string
	ValidityNote string
	InfoTitle    string
	InfoBody     string
	FooterNote   string
}

func renderSecurityEmail(data securityEmailData) string {
	logo := `<span style="display:inline-block;font-family:Arial,sans-serif;font-size:19px;line-height:1;font-weight:800;letter-spacing:0.08em;color:#363535;">S.T.A</span>`
	if logoURL := strings.TrimSpace(data.LogoURL); logoURL != "" {
		logo = `<img src="` + html.EscapeString(logoURL) + `" width="32" height="32" alt="S.T.A" style="display:block;width:32px;height:32px;object-fit:contain;border:0;">`
	}

	var paragraphs strings.Builder
	for i, p := range data.Paragraphs {
		color := "#363535"
		if i > 0 {
			color = "#5f615c"
		}
		margin := "margin:0 0 14px;"
		if i == 0 {
			margin = "margin:24px 0 14px;"
		}
		if i == len(data.Paragraphs)-1 {
			margin = "margin:0;"
			if i == 0 {
				margin = "margin:24px 0 0;"
			}
		}
		paragraphs.WriteString(`<p style="` + margin + `font-size:` + fontSize(i) + `;line-height:1.8;color:` + color + `;">` + html.EscapeString(p) + `</p>`)
	}

	buttonURL := html.EscapeString(strings.TrimSpace(data.ButtonURL))
	return `<!doctype html>
<html lang="zh-Hant">
<head>
  <meta charset="utf-8">
  <meta name="x-apple-disable-message-reformatting">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>` + html.EscapeString(data.Title) + `</title>
</head>
<body style="margin:0;padding:0;background-color:#f5f6f1;color:#363535;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI','Noto Sans TC',Arial,sans-serif;">
  <div style="display:none;max-height:0;overflow:hidden;opacity:0;color:transparent;">
    ` + html.EscapeString(data.Preheader) + `
  </div>
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="width:100%;background-color:#f5f6f1;">
    <tr>
      <td align="center" style="padding:36px 16px 48px;">
        <table role="presentation" width="600" cellpadding="0" cellspacing="0" border="0" style="width:100%;max-width:600px;background-color:#ffffff;border:1px solid #dce1d7;">
          <tr>
            <td style="height:6px;background-color:#ffe184;font-size:0;line-height:0;">&nbsp;</td>
          </tr>
          <tr>
            <td style="padding:28px 36px 24px;border-bottom:1px solid #e3e6df;">
              <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
                <tr>
                  <td valign="middle" style="width:32px;">
                    ` + logo + `
                  </td>
                  <td valign="middle" style="padding-left:10px;">
                    <span style="font-family:Georgia,'Noto Serif TC',serif;font-size:17px;line-height:1.3;color:#363535;">S.T.A</span>
                    <span style="padding:0 7px;font-size:14px;color:#9a9c96;">|</span>
                    <span style="font-family:Georgia,'Noto Serif TC',serif;font-size:16px;line-height:1.3;color:#363535;">特殊選才資源網</span>
                  </td>
                </tr>
              </table>
            </td>
          </tr>
          <tr>
            <td style="padding:40px 36px 42px;">
              <p style="margin:0;font-size:12px;line-height:1.4;font-weight:700;letter-spacing:0.08em;color:#557b3e;">` + html.EscapeString(data.Eyebrow) + `</p>
              <h1 style="margin:12px 0 0;font-size:32px;line-height:1.25;font-weight:800;letter-spacing:-0.04em;color:#363535;">` + html.EscapeString(data.Heading) + `</h1>
              ` + paragraphs.String() + `

              <table role="presentation" cellpadding="0" cellspacing="0" border="0" align="center" style="margin:32px auto 0;">
                <tr>
                  <td align="center" style="background-color:#363535;">
                    <a href="` + buttonURL + `" target="_blank" style="display:inline-block;padding:15px 28px;border:1px solid #363535;font-size:15px;line-height:1.2;font-weight:700;color:#ffffff;text-decoration:none;">` + html.EscapeString(data.ButtonLabel) + `&nbsp;&nbsp;→</a>
                  </td>
                </tr>
              </table>
              <p style="margin:12px 0 0;text-align:center;font-size:12px;line-height:1.5;color:#8b8d87;">` + html.EscapeString(data.ValidityNote) + `</p>

              <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="margin:40px 0 0;background-color:#f3f8ec;border-top:1px solid #d9edbf;border-bottom:1px solid #d9edbf;border-left:4px solid #badaaf;border-radius:3px;">
                <tr>
                  <td align="center" style="padding:17px 18px;">
                    <p style="margin:0 0 6px;text-align:center;font-size:13px;line-height:1.5;font-weight:700;color:#363535;">` + html.EscapeString(data.InfoTitle) + `</p>
                    <p style="margin:0;text-align:center;font-size:13px;line-height:1.8;color:#777a73;">` + html.EscapeString(data.InfoBody) + `</p>
                  </td>
                </tr>
              </table>

              <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="margin:30px 0 0;border-top:1px solid #e3e6df;">
                <tr>
                  <td align="center" style="padding:17px 0 0;">
                    <p style="margin:0 0 7px;text-align:center;font-size:12px;line-height:1.5;color:#8b8d87;">按鈕無法使用？請複製以下網址到瀏覽器：</p>
                    <p style="margin:0;text-align:center;font-size:12px;line-height:1.8;word-break:break-all;overflow-wrap:anywhere;"><a href="` + buttonURL + `" target="_blank" style="color:#557b3e;text-decoration:underline;">` + buttonURL + `</a></p>
                  </td>
                </tr>
              </table>
            </td>
          </tr>
          <tr>
            <td align="center" style="padding:22px 36px 26px;border-top:1px solid #e3e6df;">
              <p style="margin:0 0 5px;text-align:center;font-size:12px;line-height:1.5;font-weight:700;color:#363535;">S.T.A 特殊選才資源網</p>
              <p style="margin:0;text-align:center;font-size:11px;line-height:1.7;color:#8b8d87;">` + html.EscapeString(data.FooterNote) + `</p>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`
}

func fontSize(paragraphIndex int) string {
	if paragraphIndex == 0 {
		return "16px"
	}
	return "15px"
}

// PasswordResetEmailData contains the URLs needed by the account security
// email. LogoURL may be empty; the template then falls back to a text-only
// brand lockup because some email clients block remote images by default.
type PasswordResetEmailData struct {
	LogoURL  string
	ResetURL string
}

// PasswordResetEmail renders the account password-reset email.
func PasswordResetEmail(data PasswordResetEmailData) string {
	return renderSecurityEmail(securityEmailData{
		LogoURL:   data.LogoURL,
		Title:     "重設你的 STA 密碼",
		Preheader: "你的 STA 密碼重設連結已準備好，連結將在 30 分鐘後失效。",
		Eyebrow:   "密碼重設",
		Heading:   "重設你的密碼",
		Paragraphs: []string{
			"我們收到一個重設 S.T.A 帳號密碼的請求。",
			"如果是你本人，請在 30 分鐘內按下方按鈕設定新密碼。若不是你本人，可以直接忽略這封信，現有密碼不會被更改。",
		},
		ButtonLabel:  "設定新密碼",
		ButtonURL:    data.ResetURL,
		ValidityNote: "此連結將在 30 分鐘後失效",
		InfoTitle:    "安全提醒",
		InfoBody:     "為了保護你的帳號，系統會讓其他裝置上的登入失效；之後請使用新密碼重新登入。",
		FooterNote:   "這是系統自動寄出的帳號安全通知，請勿直接回覆。",
	})
}

// AccountActivationEmailData contains the URL needed by the registration
// activation email — the account stays 'pending_verification' until this
// link is used to set a password. See auth.Service.Register.
type AccountActivationEmailData struct {
	LogoURL string
	SetURL  string
}

// AccountActivationEmail renders the "welcome, set your password to
// activate your account" email sent to the school address on registration.
// Same visual language as PasswordResetEmail — it's functionally the same
// kind of link (set-password), just framed as onboarding instead of a
// security notice.
func AccountActivationEmail(data AccountActivationEmailData) string {
	return renderSecurityEmail(securityEmailData{
		LogoURL:   data.LogoURL,
		Title:     "歡迎加入 STA",
		Preheader: "你的 STA 帳號啟用連結已準備好，連結將在 24 小時後失效。",
		Eyebrow:   "新帳號",
		Heading:   "歡迎加入 STA",
		Paragraphs: []string{
			"感謝你註冊 S.T.A 特殊選才資源網。",
			"請在 24 小時內按下方按鈕，設定密碼以啟用帳號；完成後就能用帳號密碼登入。",
		},
		ButtonLabel:  "設定密碼",
		ButtonURL:    data.SetURL,
		ValidityNote: "連結有效期限：24 小時",
		InfoTitle:    "為什麼要設定密碼？",
		InfoBody:     "這封信寄到你的學校信箱，用來確認帳號屬於你本人；設定密碼後帳號才會正式啟用。",
		FooterNote:   "這是系統自動寄出的帳號啟用通知，請勿直接回覆。",
	})
}

// InfoEmail renders the same styling as ButtonEmail but without a button —
// for a notice with nothing to click (e.g. a rejection notice where the
// reply-by-email itself is the call to action).
func InfoEmail(heading string, paragraphs []string) string {
	var body strings.Builder
	for _, p := range paragraphs {
		body.WriteString(`<p style="margin:0 0 16px;font-size:15px;line-height:1.7;color:#3a3a3a;">`)
		body.WriteString(html.EscapeString(p))
		body.WriteString(`</p>`)
	}
	return `<!doctype html>
<html>
<body style="margin:0;padding:0;background-color:#f4f3ef;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'Noto Sans TC',sans-serif;">
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color:#f4f3ef;padding:32px 16px;">
    <tr>
      <td align="center">
        <table role="presentation" width="480" cellpadding="0" cellspacing="0" style="max-width:480px;width:100%;background-color:#ffffff;border-radius:16px;overflow:hidden;">
          <tr>
            <td style="background-color:#2f6f4f;padding:24px 32px;">
              <span style="font-size:18px;font-weight:700;color:#ffffff;letter-spacing:0.04em;">S.T.A 特殊選才資源網</span>
            </td>
          </tr>
          <tr>
            <td style="padding:32px;">
              <h1 style="margin:0 0 20px;font-size:20px;line-height:1.4;color:#1a1a1a;">` + html.EscapeString(heading) + `</h1>
              ` + body.String() + `
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`
}

// ButtonEmail renders a small, self-contained HTML email: a heading, one or
// more paragraphs, and a prominent call-to-action button. Styles are inlined
// (email clients strip <style> blocks unpredictably) and the layout is a
// single centred column, which is the safest baseline across clients.
//
// The button's href is the only place the link appears — callers should not
// also print the raw URL/token as plain text; that's what a plain-text
// fallback part is for, and every part still gets Subject/body caught by the
// spam filters that judge token-only mail harshly.
func ButtonEmail(heading string, paragraphs []string, buttonLabel, buttonURL string) string {
	var body strings.Builder
	for _, p := range paragraphs {
		body.WriteString(`<p style="margin:0 0 16px;font-size:15px;line-height:1.7;color:#3a3a3a;">`)
		body.WriteString(html.EscapeString(p))
		body.WriteString(`</p>`)
	}
	return `<!doctype html>
<html>
<body style="margin:0;padding:0;background-color:#f4f3ef;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'Noto Sans TC',sans-serif;">
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color:#f4f3ef;padding:32px 16px;">
    <tr>
      <td align="center">
        <table role="presentation" width="480" cellpadding="0" cellspacing="0" style="max-width:480px;width:100%;background-color:#ffffff;border-radius:16px;overflow:hidden;">
          <tr>
            <td style="background-color:#2f6f4f;padding:24px 32px;">
              <span style="font-size:18px;font-weight:700;color:#ffffff;letter-spacing:0.04em;">S.T.A 特殊選才資源網</span>
            </td>
          </tr>
          <tr>
            <td style="padding:32px;">
              <h1 style="margin:0 0 20px;font-size:20px;line-height:1.4;color:#1a1a1a;">` + html.EscapeString(heading) + `</h1>
              ` + body.String() + `
              <table role="presentation" cellpadding="0" cellspacing="0" style="margin:24px 0 8px;">
                <tr>
                  <td style="border-radius:10px;background-color:#2f6f4f;">
                    <a href="` + html.EscapeString(buttonURL) + `" target="_blank" style="display:inline-block;padding:14px 32px;font-size:15px;font-weight:700;color:#ffffff;text-decoration:none;border-radius:10px;">` + html.EscapeString(buttonLabel) + `</a>
                  </td>
                </tr>
              </table>
              <p style="margin:24px 0 0;font-size:12px;line-height:1.6;color:#9a9a9a;">如果按鈕沒有反應，請複製這個網址到瀏覽器開啟：<br>` + html.EscapeString(buttonURL) + `</p>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`
}
