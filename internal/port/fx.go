package port

import (
	"context"
	"time"

	"fx-settlement-lab/go-backend/internal/domain/conversion"
	"fx-settlement-lab/go-backend/internal/domain/outbox"
	"fx-settlement-lab/go-backend/internal/domain/quote"
	"fx-settlement-lab/go-backend/internal/domain/shared"
	"fx-settlement-lab/go-backend/internal/domain/webhook"
)

type ReferenceRateRepository interface {
	UpsertRates(ctx context.Context, rates []shared.ExchangeRate) error
	ListLatestRates(ctx context.Context, base shared.Currency, quotes []shared.Currency) ([]shared.ExchangeRate, error)
}

type QuoteRepository interface {
	GetQuoteByID(ctx context.Context, quoteID string) (quote.Quote, error)
	GetQuoteByIdempotencyKey(ctx context.Context, key shared.IdempotencyKey) (quote.Quote, bool, error)
	CreateQuote(ctx context.Context, value quote.Quote) (quote.Quote, error)
	MarkQuoteAccepted(ctx context.Context, quoteID string, acceptedAt time.Time) error
}

type ConversionRepository interface {
	GetConversionByID(ctx context.Context, conversionID string) (conversion.Conversion, error)
	GetConversionByIdempotencyKey(ctx context.Context, key shared.IdempotencyKey) (conversion.Conversion, bool, error)
	CreateConversion(ctx context.Context, value conversion.Conversion) (conversion.Conversion, error)
	UpdateConversion(ctx context.Context, value conversion.Conversion) (conversion.Conversion, error)
}

type WebhookInboxRepository interface {
	StoreIfAbsent(ctx context.Context, message webhook.InboxMessage) (bool, error)
}

type OutboxRepository interface {
	Enqueue(ctx context.Context, event outbox.Event) error
	ListPending(ctx context.Context, limit int) ([]outbox.Event, error)
	MarkPublished(ctx context.Context, eventID string, publishedAt time.Time) error
	MarkPublishFailed(ctx context.Context, eventID string, lastError string) error
}

type RateProvider interface {
	GetReferenceRates(ctx context.Context, base shared.Currency, quotes []shared.Currency) ([]shared.ExchangeRate, error)
}

type EventPublisher interface {
	Publish(ctx context.Context, event outbox.Event) error
}

type Clock interface {
	Now() time.Time
}

type Telemetry interface {
	StartSpan(ctx context.Context, name string) (context.Context, func(err error))
	RecordInboundRequest(ctx context.Context, transport string, operation string, outcome string, duration time.Duration)
	RecordWebhookDuplicate(ctx context.Context, provider string)
	RecordWebhookAccepted(ctx context.Context, provider string)
	RecordOutboxPublished(ctx context.Context, eventType string)
	RecordOutboxPublishFailure(ctx context.Context, eventType string)
}

type TransactionRepositories struct {
	Rates        ReferenceRateRepository
	Quotes       QuoteRepository
	Conversions  ConversionRepository
	WebhookInbox WebhookInboxRepository
	Outbox       OutboxRepository
}

type UnitOfWork interface {
	WithinTransaction(ctx context.Context, fn func(ctx context.Context, repos TransactionRepositories) error) error
}
