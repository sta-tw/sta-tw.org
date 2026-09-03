package chat

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

func VerifyWebhookSignature(secret, body, signature string) bool {
	if secret == "" || signature == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(body))
	want := mac.Sum(nil)
	got, err := hex.DecodeString(signature)
	return err == nil && hmac.Equal(got, want)
}
