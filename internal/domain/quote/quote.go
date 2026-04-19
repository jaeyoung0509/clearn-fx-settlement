package quote

import (
	"time"

	"github.com/shopspring/decimal"

	"fx-settlement-lab/go-backend/internal/domain"
	"fx-settlement-lab/go-backend/internal/domain/shared"
)

type Quote struct {
	ID               string
	IdempotencyKey   shared.IdempotencyKey
	BaseAmount       shared.Money
	QuoteAmount      shared.Money
	FeeAmount        shared.Money
	TotalDebitAmount shared.Money
	Rate             decimal.Decimal
	RateProvider     string
	ExpiresAt        time.Time
	AcceptedAt       *time.Time
	CreatedAt        time.Time
}

func New(
	id string,
	idempotencyKey shared.IdempotencyKey,
	baseAmount shared.Money,
	targetCurrency shared.Currency,
	exchangeRate shared.ExchangeRate,
	feeBPS int64,
	minimumFee shared.Money,
	now time.Time,
	ttl time.Duration,
) (Quote, error) {
	if id == "" {
		return Quote{}, domain.Validation("Quote ID is required", nil)
	}
	if err := baseAmount.ValidatePositive("baseAmount"); err != nil {
		return Quote{}, err
	}
	if targetCurrency == "" {
		return Quote{}, domain.Validation("Quote currency is required", nil)
	}
	if ttl <= 0 {
		return Quote{}, domain.Validation("Quote TTL must be positive", map[string]any{
			"ttl": ttl.String(),
		})
	}
	if exchangeRate.Base != baseAmount.Currency || exchangeRate.Quote != targetCurrency {
		return Quote{}, domain.Validation("Reference rate does not match quote corridor", map[string]any{
			"rateBase":     exchangeRate.Base,
			"rateQuote":    exchangeRate.Quote,
			"baseCurrency": baseAmount.Currency,
			"quote":        targetCurrency,
		})
	}

	quoteAmount, err := shared.Convert(baseAmount, targetCurrency, exchangeRate.Rate)
	if err != nil {
		return Quote{}, err
	}

	feeAmount, err := shared.FeeFromBPS(baseAmount, feeBPS, minimumFee)
	if err != nil {
		return Quote{}, err
	}

	totalDebitAmount, err := baseAmount.Add(feeAmount)
	if err != nil {
		return Quote{}, err
	}

	return Quote{
		ID:               id,
		IdempotencyKey:   idempotencyKey,
		BaseAmount:       baseAmount,
		QuoteAmount:      quoteAmount,
		FeeAmount:        feeAmount,
		TotalDebitAmount: totalDebitAmount,
		Rate:             exchangeRate.Rate,
		RateProvider:     exchangeRate.Provider,
		ExpiresAt:        now.Add(ttl).UTC(),
		CreatedAt:        now.UTC(),
	}, nil
}

func (q Quote) CanAccept(at time.Time) error {
	if q.AcceptedAt != nil {
		return domain.Conflict("Quote already accepted", map[string]any{
			"quoteId": q.ID,
		})
	}
	if at.UTC().After(q.ExpiresAt) {
		return domain.Conflict("Quote expired", map[string]any{
			"quoteId":   q.ID,
			"expiresAt": q.ExpiresAt,
		})
	}

	return nil
}
