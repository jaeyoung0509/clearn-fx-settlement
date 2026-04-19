package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"fx-settlement-lab/go-backend/internal/adapter/outbound/observability"
	platformpostgres "fx-settlement-lab/go-backend/internal/adapter/outbound/postgres"
	"fx-settlement-lab/go-backend/internal/domain/conversion"
	"fx-settlement-lab/go-backend/internal/domain/outbox"
	"fx-settlement-lab/go-backend/internal/domain/quote"
	"fx-settlement-lab/go-backend/internal/domain/shared"
	"fx-settlement-lab/go-backend/internal/domain/webhook"
	"fx-settlement-lab/go-backend/internal/port"
	"fx-settlement-lab/go-backend/internal/testutil"
	"fx-settlement-lab/go-backend/internal/usecase"
)

func TestStoreIntegration(t *testing.T) {
	instance := testutil.StartPostgres(t)
	store := platformpostgres.NewStore(instance.Gorm, zap.NewNop())
	now := time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC)

	t.Run("upsert and list latest rates", func(t *testing.T) {
		instance.Reset(t)

		err := store.UpsertRates(context.Background(), []shared.ExchangeRate{
			{
				Base:       shared.CurrencyKRW,
				Quote:      shared.CurrencyUSD,
				Provider:   shared.ProviderFrankfurter,
				Rate:       decimal.RequireFromString("0.00074"),
				ObservedAt: now,
				FetchedAt:  now,
			},
			{
				Base:       shared.CurrencyKRW,
				Quote:      shared.CurrencyJPY,
				Provider:   shared.ProviderFrankfurter,
				Rate:       decimal.RequireFromString("0.1101"),
				ObservedAt: now,
				FetchedAt:  now,
			},
		})
		if err != nil {
			t.Fatalf("upsert rates: %v", err)
		}

		rates, err := store.ListLatestRates(context.Background(), shared.CurrencyKRW, []shared.Currency{shared.CurrencyUSD, shared.CurrencyJPY})
		if err != nil {
			t.Fatalf("list rates: %v", err)
		}
		if len(rates) != 2 {
			t.Fatalf("unexpected number of rates: %d", len(rates))
		}
	})

	t.Run("persist quote conversion webhook and outbox", func(t *testing.T) {
		instance.Reset(t)

		sourceQuote := mustQuote(t, now)
		savedQuote, err := store.CreateQuote(context.Background(), sourceQuote)
		if err != nil {
			t.Fatalf("create quote: %v", err)
		}

		loadedByKey, found, err := store.GetQuoteByIdempotencyKey(context.Background(), sourceQuote.IdempotencyKey)
		if err != nil {
			t.Fatalf("get quote by key: %v", err)
		}
		if !found || loadedByKey.ID != savedQuote.ID {
			t.Fatalf("unexpected quote by key: found=%v quote=%#v", found, loadedByKey)
		}

		createdConversion, err := conversion.FromQuote(
			"01JS33BAW5S8R1V4AKTMM5YG8B",
			shared.IdempotencyKey("conversion-idempotency"),
			savedQuote,
			shared.ProviderToss,
			shared.ProviderWise,
			now.Add(time.Minute),
		)
		if err != nil {
			t.Fatalf("create conversion from quote: %v", err)
		}

		err = store.WithinTransaction(context.Background(), func(ctx context.Context, repos port.TransactionRepositories) error {
			if _, err := repos.Conversions.CreateConversion(ctx, createdConversion); err != nil {
				return err
			}
			if err := repos.Quotes.MarkQuoteAccepted(ctx, savedQuote.ID, now.Add(time.Minute)); err != nil {
				return err
			}
			if err := repos.Outbox.Enqueue(ctx, outbox.Event{
				ID:            "01JS33BAW5S8R1V4AKTMM5YG8C",
				AggregateType: "conversion",
				AggregateID:   createdConversion.ID,
				EventType:     "conversion.created",
				Payload:       []byte(`{"conversionId":"` + createdConversion.ID + `"}`),
				CreatedAt:     now.Add(time.Minute),
			}); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			t.Fatalf("transactional conversion creation: %v", err)
		}

		storedConversion, err := store.GetConversionByID(context.Background(), createdConversion.ID)
		if err != nil {
			t.Fatalf("get conversion: %v", err)
		}
		if storedConversion.Status != conversion.StatusAwaitingPayment {
			t.Fatalf("unexpected conversion status: %s", storedConversion.Status)
		}

		processedAt := now.Add(2 * time.Minute)
		inserted, err := store.StoreIfAbsent(context.Background(), webhook.InboxMessage{
			ID:              "01JS33BAW5S8R1V4AKTMM5YG8D",
			Provider:        shared.ProviderToss,
			Topic:           "payment.succeeded",
			ExternalEventID: "evt-1",
			ConversionID:    createdConversion.ID,
			Payload:         []byte(`{"event":"payment.succeeded"}`),
			ReceivedAt:      processedAt,
			ProcessedAt:     &processedAt,
		})
		if err != nil {
			t.Fatalf("store inbox message: %v", err)
		}
		if !inserted {
			t.Fatal("expected first inbox insert to succeed")
		}

		inserted, err = store.StoreIfAbsent(context.Background(), webhook.InboxMessage{
			ID:              "01JS33BAW5S8R1V4AKTMM5YG8E",
			Provider:        shared.ProviderToss,
			Topic:           "payment.succeeded",
			ExternalEventID: "evt-1",
			ConversionID:    createdConversion.ID,
			Payload:         []byte(`{"event":"payment.succeeded"}`),
			ReceivedAt:      processedAt,
			ProcessedAt:     &processedAt,
		})
		if err != nil {
			t.Fatalf("store duplicate inbox message: %v", err)
		}
		if inserted {
			t.Fatal("expected duplicate inbox insert to be ignored")
		}

		pending, err := store.ListPending(context.Background(), 10)
		if err != nil {
			t.Fatalf("list pending outbox: %v", err)
		}
		if len(pending) != 1 {
			t.Fatalf("unexpected pending outbox count: %d", len(pending))
		}
	})

	t.Run("publish outbox with retry accounting", func(t *testing.T) {
		instance.Reset(t)

		if err := store.Enqueue(context.Background(), outbox.Event{
			ID:            "01JS33BAW5S8R1V4AKTMM5YG8F",
			AggregateType: "conversion",
			AggregateID:   "01JS33BAW5S8R1V4AKTMM5YG8G",
			EventType:     "conversion.created",
			Payload:       []byte(`{"conversionId":"c1"}`),
			CreatedAt:     now,
		}); err != nil {
			t.Fatalf("enqueue first event: %v", err)
		}
		if err := store.Enqueue(context.Background(), outbox.Event{
			ID:            "01JS33BAW5S8R1V4AKTMM5YG8H",
			AggregateType: "conversion",
			AggregateID:   "01JS33BAW5S8R1V4AKTMM5YG8I",
			EventType:     "conversion.completed",
			Payload:       []byte(`{"conversionId":"c2"}`),
			CreatedAt:     now.Add(time.Second),
		}); err != nil {
			t.Fatalf("enqueue second event: %v", err)
		}

		publisher := &fakePublisher{failEventType: "conversion.created"}
		publishOutbox := usecase.NewPublishOutboxUsecase(store, publisher, observability.NewTelemetry())

		result, err := publishOutbox.Execute(context.Background(), 10)
		if err != nil {
			t.Fatalf("publish outbox: %v", err)
		}
		if result.Processed != 2 || result.Published != 1 || result.Failed != 1 {
			t.Fatalf("unexpected publish result: %#v", result)
		}

		pending, err := store.ListPending(context.Background(), 10)
		if err != nil {
			t.Fatalf("list pending outbox after publish: %v", err)
		}
		if len(pending) != 1 || pending[0].EventType != "conversion.created" {
			t.Fatalf("unexpected pending outbox after publish: %#v", pending)
		}
		if pending[0].PublishAttempts != 1 {
			t.Fatalf("expected failed event attempts to increment: %#v", pending[0])
		}
	})
}

func mustQuote(t *testing.T, now time.Time) quote.Quote {
	t.Helper()

	value, err := quote.New(
		"01JS33BAW5S8R1V4AKTMM5YG8A",
		shared.IdempotencyKey("quote-idempotency"),
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

	return value
}

type fakePublisher struct {
	failEventType string
}

func (f *fakePublisher) Publish(_ context.Context, event outbox.Event) error {
	if event.EventType == f.failEventType {
		return errors.New("forced publish failure")
	}
	return nil
}
