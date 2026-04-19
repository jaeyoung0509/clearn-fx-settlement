package httpadapter_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	httpadapter "fx-settlement-lab/go-backend/internal/adapter/inbound/http"
	"fx-settlement-lab/go-backend/internal/adapter/outbound/observability"
	platformpostgres "fx-settlement-lab/go-backend/internal/adapter/outbound/postgres"
	"fx-settlement-lab/go-backend/internal/adapter/outbound/webhooksigning"
	"fx-settlement-lab/go-backend/internal/domain/shared"
	"fx-settlement-lab/go-backend/internal/testutil"
	"fx-settlement-lab/go-backend/internal/usecase"
)

func TestFXAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)

	instance := testutil.StartPostgres(t)
	clock := &fixedClock{now: time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC)}
	router := buildRouter(t, instance, clock)

	doRequest := func(method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
		t.Helper()

		var payloadBytes []byte
		if body != nil {
			var err error
			payloadBytes, err = json.Marshal(body)
			if err != nil {
				t.Fatalf("marshal request: %v", err)
			}
		}

		request := httptest.NewRequest(method, path, bytes.NewReader(payloadBytes))
		if body != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		for key, value := range headers {
			request.Header.Set(key, value)
		}

		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		return recorder
	}

	t.Run("rates quote conversion and webhook lifecycle", func(t *testing.T) {
		instance.Reset(t)

		ratesResponse := doRequest(http.MethodGet, "/api/v1/rates?base=KRW&quotes=USD,JPY", nil, nil)
		if ratesResponse.Code != http.StatusOK {
			t.Fatalf("rates status: %d body=%s", ratesResponse.Code, ratesResponse.Body.String())
		}
		ratesPayload := decodeMap(t, ratesResponse.Body.Bytes())
		if len(ratesPayload["data"].(map[string]any)["rates"].([]any)) != 2 {
			t.Fatalf("unexpected rates payload: %#v", ratesPayload)
		}

		quoteResponse := doRequest(http.MethodPost, "/api/v1/quotes", map[string]any{
			"baseAmount": map[string]any{
				"currency":   "KRW",
				"minorUnits": 100000,
			},
			"quoteCurrency": "USD",
		}, map[string]string{"Idempotency-Key": "quote-key"})
		if quoteResponse.Code != http.StatusOK {
			t.Fatalf("quote status: %d body=%s", quoteResponse.Code, quoteResponse.Body.String())
		}
		quotePayload := decodeMap(t, quoteResponse.Body.Bytes())["data"].(map[string]any)
		quoteID := quotePayload["id"].(string)
		if quotePayload["rate"] != "0.00074" {
			t.Fatalf("unexpected quote payload: %#v", quotePayload)
		}

		duplicateQuoteResponse := doRequest(http.MethodPost, "/api/v1/quotes", map[string]any{
			"baseAmount": map[string]any{
				"currency":   "KRW",
				"minorUnits": 100000,
			},
			"quoteCurrency": "USD",
		}, map[string]string{"Idempotency-Key": "quote-key"})
		if duplicateQuoteResponse.Code != http.StatusOK {
			t.Fatalf("duplicate quote status: %d body=%s", duplicateQuoteResponse.Code, duplicateQuoteResponse.Body.String())
		}
		duplicateQuotePayload := decodeMap(t, duplicateQuoteResponse.Body.Bytes())["data"].(map[string]any)
		if duplicateQuotePayload["id"] != quoteID {
			t.Fatalf("expected idempotent quote response, got %#v", duplicateQuotePayload)
		}

		conversionResponse := doRequest(http.MethodPost, "/api/v1/conversions", map[string]any{
			"quoteId": quoteID,
		}, map[string]string{"Idempotency-Key": "conversion-key"})
		if conversionResponse.Code != http.StatusOK {
			t.Fatalf("conversion status: %d body=%s", conversionResponse.Code, conversionResponse.Body.String())
		}
		conversionPayload := decodeMap(t, conversionResponse.Body.Bytes())["data"].(map[string]any)
		conversionID := conversionPayload["id"].(string)
		if conversionPayload["status"] != "AWAITING_PAYMENT" {
			t.Fatalf("unexpected conversion payload: %#v", conversionPayload)
		}

		clock.now = clock.now.Add(time.Minute)
		paymentPayload := map[string]any{
			"provider":          "toss",
			"externalEventId":   "evt-payment-1",
			"conversionId":      conversionID,
			"eventType":         "payment.succeeded",
			"externalReference": "pay_123",
			"occurredAt":        clock.now,
		}
		paymentResponse := doSignedWebhookRequest(t, router, "/api/v1/webhooks/payments", paymentPayload, "payment-secret")
		if paymentResponse.Code != http.StatusOK {
			t.Fatalf("payment webhook status: %d body=%s", paymentResponse.Code, paymentResponse.Body.String())
		}
		paymentResult := decodeMap(t, paymentResponse.Body.Bytes())["data"].(map[string]any)
		if paymentResult["duplicate"] != false {
			t.Fatalf("unexpected payment webhook payload: %#v", paymentResult)
		}
		if paymentResult["conversion"].(map[string]any)["status"] != "PROCESSING" {
			t.Fatalf("unexpected conversion after payment webhook: %#v", paymentResult)
		}

		duplicatePaymentResponse := doSignedWebhookRequest(t, router, "/api/v1/webhooks/payments", paymentPayload, "payment-secret")
		if duplicatePaymentResponse.Code != http.StatusOK {
			t.Fatalf("duplicate payment webhook status: %d body=%s", duplicatePaymentResponse.Code, duplicatePaymentResponse.Body.String())
		}
		duplicatePaymentResult := decodeMap(t, duplicatePaymentResponse.Body.Bytes())["data"].(map[string]any)
		if duplicatePaymentResult["duplicate"] != true {
			t.Fatalf("expected duplicate webhook acknowledgement: %#v", duplicatePaymentResult)
		}

		clock.now = clock.now.Add(time.Minute)
		transferPayload := map[string]any{
			"provider":          "wise",
			"externalEventId":   "evt-transfer-1",
			"conversionId":      conversionID,
			"eventType":         "transfer.completed",
			"externalReference": "tr_123",
			"occurredAt":        clock.now,
		}
		transferResponse := doSignedWebhookRequest(t, router, "/api/v1/webhooks/transfers", transferPayload, "transfer-secret")
		if transferResponse.Code != http.StatusOK {
			t.Fatalf("transfer webhook status: %d body=%s", transferResponse.Code, transferResponse.Body.String())
		}
		transferResult := decodeMap(t, transferResponse.Body.Bytes())["data"].(map[string]any)
		if transferResult["conversion"].(map[string]any)["status"] != "COMPLETED" {
			t.Fatalf("unexpected conversion after transfer webhook: %#v", transferResult)
		}

		getConversionResponse := doRequest(http.MethodGet, "/api/v1/conversions/"+conversionID, nil, nil)
		if getConversionResponse.Code != http.StatusOK {
			t.Fatalf("get conversion status: %d body=%s", getConversionResponse.Code, getConversionResponse.Body.String())
		}
		getConversionPayload := decodeMap(t, getConversionResponse.Body.Bytes())["data"].(map[string]any)
		if getConversionPayload["status"] != "COMPLETED" {
			t.Fatalf("unexpected get conversion payload: %#v", getConversionPayload)
		}
	})

	t.Run("error envelope and readiness", func(t *testing.T) {
		instance.Reset(t)

		notFoundResponse := doRequest(http.MethodGet, "/api/v1/conversions/01ARZ3NDEKTSV4RRFFQ69G5FAV", nil, map[string]string{
			"X-Request-ID": "custom-request-id",
		})
		if notFoundResponse.Code != http.StatusNotFound {
			t.Fatalf("unexpected not found status: %d body=%s", notFoundResponse.Code, notFoundResponse.Body.String())
		}
		notFoundPayload := decodeMap(t, notFoundResponse.Body.Bytes())
		if notFoundPayload["error"].(map[string]any)["requestId"] != "custom-request-id" {
			t.Fatalf("unexpected requestId propagation: %#v", notFoundPayload)
		}

		healthResponse := doRequest(http.MethodGet, "/health", nil, nil)
		if healthResponse.Code != http.StatusOK {
			t.Fatalf("health status: %d", healthResponse.Code)
		}

		readyResponse := doRequest(http.MethodGet, "/ready", nil, nil)
		if readyResponse.Code != http.StatusOK {
			t.Fatalf("ready status: %d body=%s", readyResponse.Code, readyResponse.Body.String())
		}
	})
}

func buildRouter(t *testing.T, instance *testutil.PostgresInstance, clock *fixedClock) *gin.Engine {
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

	return httpadapter.NewRouter(httpadapter.RouterDeps{
		Logger:                  zap.NewNop(),
		GetReferenceRates:       usecase.NewGetReferenceRatesUsecase(store, rateProvider, telemetry),
		CreateQuote:             usecase.NewCreateQuoteUsecase(store, store, rateProvider, clock, telemetry, 15*time.Minute, 50, shared.MustMoney(shared.CurrencyKRW, 500)),
		AcceptQuote:             usecase.NewAcceptQuoteUsecase(store, store, store, clock, telemetry),
		GetConversion:           usecase.NewGetConversionUsecase(store, telemetry),
		HandlePaymentWebhook:    usecase.NewHandlePaymentWebhookUsecase(store, store, clock, telemetry),
		HandleTransferWebhook:   usecase.NewHandleTransferWebhookUsecase(store, store, clock, telemetry),
		PaymentWebhookVerifier:  webhooksigning.NewHMACVerifier("payment-secret"),
		TransferWebhookVerifier: webhooksigning.NewHMACVerifier("transfer-secret"),
		ReadyChecker:            instance.Pool,
		CORSAllowedOrigins:      instance.Config.CORSAllowedOrigins,
	})
}

func decodeMap(t *testing.T, payload []byte) map[string]any {
	t.Helper()

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode payload: %v", err)
	}

	return decoded
}

func doSignedWebhookRequest(t *testing.T, router *gin.Engine, path string, body any, secret string) *httptest.ResponseRecorder {
	t.Helper()

	payloadBytes, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal webhook payload: %v", err)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payloadBytes)
	signature := hex.EncodeToString(mac.Sum(nil))

	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payloadBytes))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Webhook-Signature", signature)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
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
