package webhooksigning

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"fx-settlement-lab/go-backend/internal/domain"
)

func TestHMACVerifier(t *testing.T) {
	t.Parallel()

	secret := "super-secret"
	payload := []byte(`{"event":"payment.succeeded"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	signature := hex.EncodeToString(mac.Sum(nil))

	verifier := NewHMACVerifier(secret)
	if err := verifier.Verify(signature, payload); err != nil {
		t.Fatalf("verify signature: %v", err)
	}

	err := verifier.Verify("wrong", payload)
	if err == nil {
		t.Fatal("expected invalid signature error")
	}
	if domain.AsAppError(err).Code != domain.ErrorCodeUnauthorized {
		t.Fatalf("unexpected error: %#v", err)
	}
}
