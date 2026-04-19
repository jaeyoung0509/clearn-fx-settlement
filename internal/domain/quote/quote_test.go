package quote

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"fx-settlement-lab/go-backend/internal/domain/shared"
)

func TestNewQuoteCalculatesAmountsAndExpiry(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC)
	value, err := New(
		"01JS2Y7R0M4P9A8S7N6D5F4E3C",
		shared.IdempotencyKey("quote-key"),
		shared.MustMoney(shared.CurrencyKRW, 100000),
		shared.CurrencyUSD,
		shared.ExchangeRate{
			Base:       shared.CurrencyKRW,
			Quote:      shared.CurrencyUSD,
			Provider:   shared.ProviderFrankfurter,
			Rate:       decimal.RequireFromString("0.00074"),
			ObservedAt: now,
			FetchedAt:  now,
		},
		50,
		shared.MustMoney(shared.CurrencyKRW, 500),
		now,
		15*time.Minute,
	)
	if err != nil {
		t.Fatalf("create quote: %v", err)
	}

	if value.QuoteAmount.Currency != shared.CurrencyUSD || value.QuoteAmount.MinorUnits != 7400 {
		t.Fatalf("unexpected quote amount: %#v", value.QuoteAmount)
	}
	if value.FeeAmount.MinorUnits != 500 {
		t.Fatalf("unexpected fee amount: %#v", value.FeeAmount)
	}
	if value.TotalDebitAmount.MinorUnits != 100500 {
		t.Fatalf("unexpected total debit amount: %#v", value.TotalDebitAmount)
	}
	if !value.ExpiresAt.Equal(now.Add(15 * time.Minute)) {
		t.Fatalf("unexpected quote expiry: %s", value.ExpiresAt)
	}
}

func TestQuoteCanAcceptRejectsExpiredQuote(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC)
	value, err := New(
		"01JS2Y7R0M4P9A8S7N6D5F4E3C",
		shared.IdempotencyKey("quote-key"),
		shared.MustMoney(shared.CurrencyKRW, 100000),
		shared.CurrencyUSD,
		shared.ExchangeRate{
			Base:       shared.CurrencyKRW,
			Quote:      shared.CurrencyUSD,
			Provider:   shared.ProviderFrankfurter,
			Rate:       decimal.RequireFromString("0.00074"),
			ObservedAt: now,
			FetchedAt:  now,
		},
		50,
		shared.MustMoney(shared.CurrencyKRW, 500),
		now,
		time.Minute,
	)
	if err != nil {
		t.Fatalf("create quote: %v", err)
	}

	if err := value.CanAccept(now.Add(2 * time.Minute)); err == nil {
		t.Fatal("expected expired quote rejection")
	}
}
