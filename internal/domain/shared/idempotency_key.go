package shared

import (
	"strings"

	"fx-settlement-lab/go-backend/internal/domain"
)

const maxIdempotencyKeyLength = 128

type IdempotencyKey string

func ParseIdempotencyKey(raw string) (IdempotencyKey, error) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return "", domain.Validation("Idempotency-Key header is required", nil)
	}
	if len(key) > maxIdempotencyKeyLength {
		return "", domain.Validation("Idempotency-Key header is too long", map[string]any{
			"maxLength": maxIdempotencyKeyLength,
		})
	}

	return IdempotencyKey(key), nil
}

func (k IdempotencyKey) String() string {
	return string(k)
}
