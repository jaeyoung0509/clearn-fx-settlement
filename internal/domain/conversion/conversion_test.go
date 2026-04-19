package conversion

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"fx-settlement-lab/go-backend/internal/domain/quote"
	"fx-settlement-lab/go-backend/internal/domain/shared"
)

func TestConversionStateTransitions(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC)
	sourceQuote, err := quote.New(
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

	value, err := FromQuote(
		"01JS2Y7R0M4P9A8S7N6D5F4E3D",
		shared.IdempotencyKey("conversion-key"),
		sourceQuote,
		shared.ProviderToss,
		shared.ProviderWise,
		now,
	)
	if err != nil {
		t.Fatalf("create conversion: %v", err)
	}
	if value.Status != StatusAwaitingPayment {
		t.Fatalf("unexpected initial status: %s", value.Status)
	}

	value, err = value.AdvanceForPayment("pay_123", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("advance for payment: %v", err)
	}
	if value.Status != StatusProcessing {
		t.Fatalf("unexpected status after payment: %s", value.Status)
	}

	value, err = value.CompleteTransfer("tr_123", now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("complete transfer: %v", err)
	}
	if value.Status != StatusCompleted {
		t.Fatalf("unexpected status after transfer completion: %s", value.Status)
	}
	if value.ExternalTransferID == nil || *value.ExternalTransferID != "tr_123" {
		t.Fatalf("unexpected external transfer id: %#v", value.ExternalTransferID)
	}
}
