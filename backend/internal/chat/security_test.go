package chat

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestVerifyWebhookSignature(t *testing.T) {
	mac := hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write([]byte("body"))
	signature := hex.EncodeToString(mac.Sum(nil))
	if !VerifyWebhookSignature("secret", "body", signature) {
		t.Fatal("valid signature was rejected")
	}
	if VerifyWebhookSignature("secret", "tampered", signature) {
		t.Fatal("tampered signature was accepted")
	}
}
