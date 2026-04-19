package usecase

import (
	"context"
	"encoding/json"
	"time"

	"github.com/oklog/ulid/v2"

	"fx-settlement-lab/go-backend/internal/domain"
	"fx-settlement-lab/go-backend/internal/domain/outbox"
	"fx-settlement-lab/go-backend/internal/domain/shared"
	"fx-settlement-lab/go-backend/internal/port"
)

func newULID() string {
	return ulid.Make().String()
}

func ensureReferenceRates(
	ctx context.Context,
	repository port.ReferenceRateRepository,
	provider port.RateProvider,
	base shared.Currency,
	quotes []shared.Currency,
) ([]shared.ExchangeRate, error) {
	rates, err := repository.ListLatestRates(ctx, base, quotes)
	if err != nil {
		return nil, err
	}
	if len(rates) == len(quotes) {
		return sortRates(rates, quotes), nil
	}

	fetched, err := provider.GetReferenceRates(ctx, base, quotes)
	if err != nil {
		return nil, err
	}
	if err := repository.UpsertRates(ctx, fetched); err != nil {
		return nil, err
	}

	rates, err = repository.ListLatestRates(ctx, base, quotes)
	if err != nil {
		return nil, err
	}
	if len(rates) != len(quotes) {
		return nil, domain.Internal("Reference rates are incomplete after sync", map[string]any{
			"base":         base,
			"quoteCount":   len(quotes),
			"returnedRate": len(rates),
		})
	}

	return sortRates(rates, quotes), nil
}

func sortRates(rates []shared.ExchangeRate, quotes []shared.Currency) []shared.ExchangeRate {
	indexByQuote := make(map[shared.Currency]int, len(quotes))
	for index, quoteCurrency := range quotes {
		indexByQuote[quoteCurrency] = index
	}

	ordered := make([]shared.ExchangeRate, len(rates))
	for _, rate := range rates {
		ordered[indexByQuote[rate.Quote]] = rate
	}

	return ordered
}

func marshalPayload(value any) json.RawMessage {
	payload, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}

	return payload
}

func newOutboxEvent(aggregateType string, aggregateID string, eventType string, payload any, now time.Time) outbox.Event {
	return outbox.Event{
		ID:            newULID(),
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		EventType:     eventType,
		Payload:       marshalPayload(payload),
		CreatedAt:     now.UTC(),
	}
}
