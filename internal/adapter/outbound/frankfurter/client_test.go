package frankfurter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"fx-settlement-lab/go-backend/internal/domain"
	"fx-settlement-lab/go-backend/internal/domain/shared"
)

func TestClientGetReferenceRates(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("base"); got != "KRW" {
			t.Fatalf("unexpected base query: %s", got)
		}
		if got := r.URL.Query().Get("quotes"); got != "USD,JPY" {
			t.Fatalf("unexpected quotes query: %s", got)
		}
		if got := r.URL.Query().Get("providers"); got != "ECB" {
			t.Fatalf("unexpected provider query: %s", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"date":"2026-04-18","base":"KRW","quote":"USD","rate":0.00074},
			{"date":"2026-04-18","base":"KRW","quote":"JPY","rate":0.1101}
		]`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "ECB", server.Client())
	rates, err := client.GetReferenceRates(context.Background(), shared.CurrencyKRW, []shared.Currency{shared.CurrencyUSD, shared.CurrencyJPY})
	if err != nil {
		t.Fatalf("get reference rates: %v", err)
	}
	if len(rates) != 2 {
		t.Fatalf("unexpected number of rates: %d", len(rates))
	}
	if rates[0].Provider != shared.ProviderFrankfurter {
		t.Fatalf("unexpected provider: %#v", rates[0].Provider)
	}
	if rates[0].ObservedAt.Format(time.DateOnly) != "2026-04-18" {
		t.Fatalf("unexpected observed date: %s", rates[0].ObservedAt)
	}
}

func TestClientMapsValidationErrors(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"unsupported currency"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", server.Client())
	_, err := client.GetReferenceRates(context.Background(), shared.CurrencyKRW, []shared.Currency{shared.CurrencyUSD})
	if err == nil {
		t.Fatal("expected validation error")
	}

	appErr := domain.AsAppError(err)
	if appErr.Code != domain.ErrorCodeValidation {
		t.Fatalf("unexpected error code: %#v", appErr)
	}
}
