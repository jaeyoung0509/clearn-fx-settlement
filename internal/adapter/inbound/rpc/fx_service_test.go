package rpcadapter_test

import (
	"context"
	"net"
	stdrpc "net/rpc"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	rpcadapter "fx-settlement-lab/go-backend/internal/adapter/inbound/rpc"
	"fx-settlement-lab/go-backend/internal/adapter/outbound/observability"
	platformpostgres "fx-settlement-lab/go-backend/internal/adapter/outbound/postgres"
	"fx-settlement-lab/go-backend/internal/domain/shared"
	"fx-settlement-lab/go-backend/internal/testutil"
	"fx-settlement-lab/go-backend/internal/usecase"
)

func TestFXRPCService(t *testing.T) {
	instance := testutil.StartPostgres(t)
	clock := &fixedClock{now: time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC)}
	client, cleanup := newClient(t, instance, clock)
	defer cleanup()

	t.Run("rates quote and conversion lifecycle", func(t *testing.T) {
		instance.Reset(t)

		var ratesReply rpcadapter.GetRatesReply
		if err := client.Call("FXRPCService.GetRates", rpcadapter.GetRatesArgs{
			Base:   "KRW",
			Quotes: []string{"USD", "JPY"},
		}, &ratesReply); err != nil {
			t.Fatalf("get rates: %v", err)
		}
		if len(ratesReply.Rates) != 2 {
			t.Fatalf("unexpected rates count: %d", len(ratesReply.Rates))
		}

		var quoteReply rpcadapter.QuoteReply
		if err := client.Call("FXRPCService.CreateQuote", rpcadapter.CreateQuoteArgs{
			IdempotencyKey: "rpc-quote-1",
			BaseAmount: rpcadapter.MoneyInput{
				Currency:   "KRW",
				MinorUnits: 100000,
			},
			QuoteCurrency: "USD",
		}, &quoteReply); err != nil {
			t.Fatalf("create quote: %v", err)
		}
		if quoteReply.Rate != "0.00074" {
			t.Fatalf("unexpected quote rate: %s", quoteReply.Rate)
		}

		var duplicateQuoteReply rpcadapter.QuoteReply
		if err := client.Call("FXRPCService.CreateQuote", rpcadapter.CreateQuoteArgs{
			IdempotencyKey: "rpc-quote-1",
			BaseAmount: rpcadapter.MoneyInput{
				Currency:   "KRW",
				MinorUnits: 100000,
			},
			QuoteCurrency: "USD",
		}, &duplicateQuoteReply); err != nil {
			t.Fatalf("create duplicate quote: %v", err)
		}
		if duplicateQuoteReply.ID != quoteReply.ID {
			t.Fatalf("expected idempotent quote response, got %s vs %s", duplicateQuoteReply.ID, quoteReply.ID)
		}

		var conversionReply rpcadapter.ConversionReply
		if err := client.Call("FXRPCService.CreateConversion", rpcadapter.CreateConversionArgs{
			IdempotencyKey: "rpc-conversion-1",
			QuoteID:        quoteReply.ID,
		}, &conversionReply); err != nil {
			t.Fatalf("create conversion: %v", err)
		}
		if conversionReply.Status != "AWAITING_PAYMENT" {
			t.Fatalf("unexpected conversion status: %s", conversionReply.Status)
		}

		var loadedConversion rpcadapter.ConversionReply
		if err := client.Call("FXRPCService.GetConversion", rpcadapter.GetConversionArgs{
			ConversionID: conversionReply.ID,
		}, &loadedConversion); err != nil {
			t.Fatalf("get conversion: %v", err)
		}
		if loadedConversion.ID != conversionReply.ID {
			t.Fatalf("unexpected conversion id: %s", loadedConversion.ID)
		}
	})

	t.Run("missing idempotency key returns error", func(t *testing.T) {
		instance.Reset(t)

		var quoteReply rpcadapter.QuoteReply
		err := client.Call("FXRPCService.CreateQuote", rpcadapter.CreateQuoteArgs{
			BaseAmount: rpcadapter.MoneyInput{
				Currency:   "KRW",
				MinorUnits: 100000,
			},
			QuoteCurrency: "USD",
		}, &quoteReply)
		if err == nil {
			t.Fatal("expected idempotency validation error")
		}
		if !strings.Contains(err.Error(), "Idempotency-Key header is required") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func newClient(t *testing.T, instance *testutil.PostgresInstance, clock *fixedClock) (*stdrpc.Client, func()) {
	t.Helper()

	server := buildServer(t, instance, clock)
	serverConn, clientConn := net.Pipe()
	go server.ServeConn(serverConn)

	client := stdrpc.NewClient(clientConn)
	cleanup := func() {
		_ = client.Close()
		_ = serverConn.Close()
		_ = clientConn.Close()
	}

	return client, cleanup
}

func buildServer(t *testing.T, instance *testutil.PostgresInstance, clock *fixedClock) *stdrpc.Server {
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

	server, err := rpcadapter.NewServer(rpcadapter.ServerDeps{
		GetReferenceRates: usecase.NewGetReferenceRatesUsecase(store, rateProvider, telemetry),
		CreateQuote:       usecase.NewCreateQuoteUsecase(store, store, rateProvider, clock, telemetry, 15*time.Minute, 50, shared.MustMoney(shared.CurrencyKRW, 500)),
		AcceptQuote:       usecase.NewAcceptQuoteUsecase(store, store, store, clock, telemetry),
		GetConversion:     usecase.NewGetConversionUsecase(store, telemetry),
	})
	if err != nil {
		t.Fatalf("build rpc server: %v", err)
	}

	return server
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
