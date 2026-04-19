package webhooksigning

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"

	"fx-settlement-lab/go-backend/internal/domain"
)

type HMACVerifier struct {
	secret string
}

func NewHMACVerifier(secret string) *HMACVerifier {
	return &HMACVerifier{secret: secret}
}

func (v *HMACVerifier) Verify(signature string, payload []byte) error {
	if v.secret == "" {
		return nil
	}
	if signature == "" {
		return domain.Unauthorized("Webhook signature is required", nil)
	}

	mac := hmac.New(sha256.New, []byte(v.secret))
	if _, err := mac.Write(payload); err != nil {
		return domain.Internal("Failed to compute webhook signature", nil).WithCause(err)
	}

	expected := hex.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) != 1 {
		return domain.Unauthorized("Webhook signature is invalid", nil)
	}

	return nil
}
