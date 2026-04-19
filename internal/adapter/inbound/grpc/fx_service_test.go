package grpcadapter_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	grpcpkg "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	grpcadapter "fx-settlement-lab/go-backend/internal/adapter/inbound/grpc"
	"fx-settlement-lab/go-backend/internal/adapter/outbound/observability"
	platformpostgres "fx-settlement-lab/go-backend/internal/adapter/outbound/postgres"
	"fx-settlement-lab/go-backend/internal/domain/shared"
	"fx-settlement-lab/go-backend/internal/testutil"
	"fx-settlement-lab/go-backend/internal/usecase"
	fxv1 "fx-settlement-lab/go-backend/proto/fx/v1"
)

func TestFXService(t *testing.T) {
	instance := testutil.StartPostgres(t)
	clock := &fixedClock{now: time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC)}
	client, cleanup := newClient(t, instance, clock)
	defer cleanup()

	t.Run("rates quote and conversion lifecycle", func(t *testing.T) {
		instance.Reset(t)

		ratesResponse, err := client.GetRates(context.Background(), &fxv1.GetRatesRequest{
			Base:   "KRW",
			Quotes: []string{"USD", "JPY"},
		})
		if err != nil {
			t.Fatalf("get rates: %v", err)
		}
		if len(ratesResponse.Rates) != 2 {
			t.Fatalf("unexpected rates count: %d", len(ratesResponse.Rates))
		}

		quoteCtx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("idempotency-key", "grpc-quote-1"))
		quoteResponse, err := client.CreateQuote(quoteCtx, &fxv1.CreateQuoteRequest{
			BaseAmount: &fxv1.MoneyInput{
				Currency:   "KRW",
				MinorUnits: 100000,
			},
			QuoteCurrency: "USD",
		})
		if err != nil {
			t.Fatalf("create quote: %v", err)
		}
		if quoteResponse.Rate != "0.00074" {
			t.Fatalf("unexpected quote rate: %s", quoteResponse.Rate)
		}

		duplicateQuoteResponse, err := client.CreateQuote(quoteCtx, &fxv1.CreateQuoteRequest{
			BaseAmount: &fxv1.MoneyInput{
				Currency:   "KRW",
				MinorUnits: 100000,
			},
			QuoteCurrency: "USD",
		})
		if err != nil {
			t.Fatalf("create duplicate quote: %v", err)
		}
		if duplicateQuoteResponse.Id != quoteResponse.Id {
			t.Fatalf("expected idempotent quote response, got %s vs %s", duplicateQuoteResponse.Id, quoteResponse.Id)
		}

		conversionCtx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("idempotency-key", "grpc-conversion-1"))
		conversionResponse, err := client.CreateConversion(conversionCtx, &fxv1.CreateConversionRequest{
			QuoteId: quoteResponse.Id,
		})
		if err != nil {
			t.Fatalf("create conversion: %v", err)
		}
		if conversionResponse.Status != "AWAITING_PAYMENT" {
			t.Fatalf("unexpected conversion status: %s", conversionResponse.Status)
		}

		loadedConversion, err := client.GetConversion(context.Background(), &fxv1.GetConversionRequest{
			ConversionId: conversionResponse.Id,
		})
		if err != nil {
			t.Fatalf("get conversion: %v", err)
		}
		if loadedConversion.Id != conversionResponse.Id {
			t.Fatalf("unexpected conversion id: %s", loadedConversion.Id)
		}
	})

	t.Run("missing idempotency metadata returns invalid argument", func(t *testing.T) {
		instance.Reset(t)

		_, err := client.CreateQuote(context.Background(), &fxv1.CreateQuoteRequest{
			BaseAmount: &fxv1.MoneyInput{
				Currency:   "KRW",
				MinorUnits: 100000,
			},
			QuoteCurrency: "USD",
		})
		if err == nil {
			t.Fatal("expected idempotency metadata validation error")
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("unexpected grpc status: %s", status.Code(err))
		}
	})
}

func newClient(t *testing.T, instance *testutil.PostgresInstance, clock *fixedClock) (fxv1.FXServiceClient, func()) {
	t.Helper()

	server := buildServer(t, instance, clock)
	listener := bufconn.Listen(1024 * 1024)

	go func() {
		if err := server.Serve(listener); err != nil {
			_ = err
		}
	}()

	connection, err := grpcpkg.NewClient("passthrough:///bufnet",
		grpcpkg.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpcpkg.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("create grpc client connection: %v", err)
	}

	cleanup := func() {
		_ = connection.Close()
		server.Stop()
		_ = listener.Close()
	}

	return fxv1.NewFXServiceClient(connection), cleanup
}

func buildServer(t *testing.T, instance *testutil.PostgresInstance, clock *fixedClock) *grpcpkg.Server {
	t.Helper()

	store := platformpostgres.NewStore(instance.Gorm, zap.NewNop())
	telemetry := observability.NewTelemetry()
	rateProvider := &fakeRateProvider{
		rates: map[shared.Currency]decimal.Decimal{
			shared.CurrencyUSD: decimal.RequireFromString("0.00074"),
			shared.CurrencyJPY: decimal.RequireFromString("0.1101"),
			shared.CurrencyEUR: decimal.RequireFromString("0.00068"),
		},
	}

	return grpcadapter.NewServer(grpcadapter.ServerDeps{
		GetReferenceRates: usecase.NewGetReferenceRatesUsecase(store, rateProvider, telemetry),
		CreateQuote:       usecase.NewCreateQuoteUsecase(store, store, rateProvider, clock, telemetry, 15*time.Minute, 50, shared.MustMoney(shared.CurrencyKRW, 500)),
		AcceptQuote:       usecase.NewAcceptQuoteUsecase(store, store, store, clock, telemetry),
		GetConversion:     usecase.NewGetConversionUsecase(store, telemetry),
	})
}

type fixedClock struct {
	now time.Time
}

func (c *fixedClock) Now() time.Time {
	return c.now
}

type fakeRateProvider struct {
	rates map[shared.Currency]decimal.Decimal
}

func (p *fakeRateProvider) GetReferenceRates(_ context.Context, base shared.Currency, quotes []shared.Currency) ([]shared.ExchangeRate, error) {
	now := time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC)
	result := make([]shared.ExchangeRate, 0, len(quotes))
	for _, quoteCurrency := range quotes {
		rate, ok := p.rates[quoteCurrency]
		if !ok {
			continue
		}
		result = append(result, shared.ExchangeRate{
			Base:       base,
			Quote:      quoteCurrency,
			Provider:   shared.ProviderFrankfurter,
			Rate:       rate,
			ObservedAt: now,
			FetchedAt:  now,
		})
	}

	return result, nil
}
