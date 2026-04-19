package shared

import (
	"time"

	"github.com/shopspring/decimal"

	"fx-settlement-lab/go-backend/internal/domain"
)

type ExchangeRate struct {
	Base       Currency
	Quote      Currency
	Provider   string
	Rate       decimal.Decimal
	ObservedAt time.Time
	FetchedAt  time.Time
}

func (r ExchangeRate) Validate() error {
	if r.Base == "" || r.Quote == "" {
		return domain.Validation("Reference rate currencies are required", nil)
	}
	if r.Provider == "" {
		return domain.Validation("Reference rate provider is required", nil)
	}
	if r.Rate.LessThanOrEqual(decimal.Zero) {
		return domain.Validation("Reference rate must be positive", map[string]any{
			"provider": r.Provider,
			"base":     r.Base,
			"quote":    r.Quote,
		})
	}

	return nil
}
