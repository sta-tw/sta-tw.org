// Package mailintake holds the RFC 5322 parsing shared by the two things
// that read mail sent to account@: internal/accountapplications (a reply on
// an existing application thread) and internal/emailinquiries (everything
// else). Neither owns this logic — it's pure MIME plumbing, not domain
// behavior.
package mailintake

import (
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"regexp"
	"strings"
)

// Attachment is one file pulled out of an inbound email.
type Attachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

// ParseBody extracts a best-effort plain-text body and any non-text
// attachments from a (possibly multipart) MIME message. transferEncoding is
// the top-level Content-Transfer-Encoding header — Gmail's "Forward" in
// particular sends a single-part message base64-encoded, and without
// decoding it here the raw base64 text ends up as the "body".
func ParseBody(contentType, transferEncoding string, body io.Reader) (string, []Attachment) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		data, _ := io.ReadAll(io.LimitReader(decodeTransfer(transferEncoding, body), 5<<20))
		return string(data), nil
	}
	boundary := params["boundary"]
	if boundary == "" {
		data, _ := io.ReadAll(io.LimitReader(decodeTransfer(transferEncoding, body), 5<<20))
		return string(data), nil
	}
	reader := multipart.NewReader(body, boundary)
	var bodyText string
	var attachments []Attachment
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		partContentType := part.Header.Get("Content-Type")
		partMediaType, partParams, _ := mime.ParseMediaType(partContentType)
		disposition := part.Header.Get("Content-Disposition")

		if strings.HasPrefix(partMediaType, "multipart/") {
			if partParams["boundary"] != "" {
				innerBody, innerAttachments := ParseBody(partContentType, part.Header.Get("Content-Transfer-Encoding"), part)
				if bodyText == "" {
					bodyText = innerBody
				}
				attachments = append(attachments, innerAttachments...)
			}
			continue
		}
		data, _ := io.ReadAll(io.LimitReader(decodeTransfer(part.Header.Get("Content-Transfer-Encoding"), part), 20<<20))
		isAttachment := strings.Contains(strings.ToLower(disposition), "attachment") || part.FileName() != ""
		if !isAttachment && strings.HasPrefix(partMediaType, "text/plain") && bodyText == "" {
			bodyText = string(data)
			continue
		}
		if isAttachment && len(data) > 0 {
			filename := part.FileName()
			if filename == "" {
				filename = "attachment"
			}
			attachments = append(attachments, Attachment{Filename: filename, ContentType: partMediaType, Data: data})
		}
	}
	return bodyText, attachments
}

func decodeTransfer(encoding string, r io.Reader) io.Reader {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		return base64.NewDecoder(base64.StdEncoding, r)
	case "quoted-printable":
		return quotedprintable.NewReader(r)
	default:
		return r
	}
}

// StripQuotedReply cuts off a reply email's body at the point the mail
// client starts quoting what it's replying to — Gmail (and most clients)
// prepend a header line like "<sender> 於 <date> 寫道：" or "On <date>,
// <name> wrote:" followed by the entire original message re-quoted with
// "> " prefixes. Without this, every reply's stored/displayed body is
// polluted with a verbatim copy of our own previous message.
func StripQuotedReply(body string) string {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	kept := lines[:0:0]
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, ">") || looksLikeQuoteHeader(trimmed) {
			break
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func looksLikeQuoteHeader(line string) bool {
	if line == "" {
		return false
	}
	if strings.Contains(strings.ToLower(line), "wrote:") {
		return true
	}
	if strings.Contains(line, "寫道") && (strings.HasSuffix(line, ":") || strings.HasSuffix(line, "：")) {
		return true
	}
	return false
}

var messageIDPattern = regexp.MustCompile(`^<[^<>\s]+>$`)

// SanitizeMessageID only accepts a well-formed "<local@domain>" msg-id —
// this value gets stored and later echoed back verbatim as In-Reply-To on
// an outbound email, so anything that doesn't look like a real Message-ID
// (or could smuggle a header via CR/LF) is dropped rather than trusted.
func SanitizeMessageID(value string) string {
	value = strings.TrimSpace(value)
	if !messageIDPattern.MatchString(value) {
		return ""
	}
	return value
}

// DecodeHeaderWord decodes a MIME encoded-word header value (e.g. a UTF-8
// Subject line), falling back to the raw value if it isn't encoded.
func DecodeHeaderWord(value string) string {
	decoded, err := (&mime.WordDecoder{}).DecodeHeader(value)
	if err != nil {
		return value
	}
	return decoded
}
