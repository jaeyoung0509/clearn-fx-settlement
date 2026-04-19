package httpadapter

import (
	"time"

	"fx-settlement-lab/go-backend/internal/domain/conversion"
	"fx-settlement-lab/go-backend/internal/domain/quote"
	"fx-settlement-lab/go-backend/internal/domain/shared"
)

type successResponse[T any] struct {
	Success   bool      `json:"success"`
	EventTime time.Time `json:"eventTime"`
	Data      T         `json:"data"`
}

type moneyResponse struct {
	Currency   string `json:"currency"`
	MinorUnits int64  `json:"minorUnits"`
	Scale      int32  `json:"scale"`
	Amount     string `json:"amount"`
}

type referenceRateResponse struct {
	BaseCurrency  string    `json:"baseCurrency"`
	QuoteCurrency string    `json:"quoteCurrency"`
	Rate          string    `json:"rate"`
	Provider      string    `json:"provider"`
	ObservedAt    time.Time `json:"observedAt"`
	FetchedAt     time.Time `json:"fetchedAt"`
}

type ratesResponse struct {
	BaseCurrency string                  `json:"baseCurrency"`
	Provider     string                  `json:"provider"`
	Rates        []referenceRateResponse `json:"rates"`
}

type quoteResponse struct {
	ID               string        `json:"id"`
	IdempotencyKey   string        `json:"idempotencyKey"`
	BaseAmount       moneyResponse `json:"baseAmount"`
	QuoteAmount      moneyResponse `json:"quoteAmount"`
	FeeAmount        moneyResponse `json:"feeAmount"`
	TotalDebitAmount moneyResponse `json:"totalDebitAmount"`
	Rate             string        `json:"rate"`
	RateProvider     string        `json:"rateProvider"`
	ExpiresAt        time.Time     `json:"expiresAt"`
	AcceptedAt       *time.Time    `json:"acceptedAt"`
	CreatedAt        time.Time     `json:"createdAt"`
}

type conversionResponse struct {
	ID                 string        `json:"id"`
	QuoteID            string        `json:"quoteId"`
	IdempotencyKey     string        `json:"idempotencyKey"`
	BaseAmount         moneyResponse `json:"baseAmount"`
	QuoteAmount        moneyResponse `json:"quoteAmount"`
	FeeAmount          moneyResponse `json:"feeAmount"`
	TotalDebitAmount   moneyResponse `json:"totalDebitAmount"`
	Rate               string        `json:"rate"`
	RateProvider       string        `json:"rateProvider"`
	PaymentProvider    string        `json:"paymentProvider"`
	TransferProvider   string        `json:"transferProvider"`
	Status             string        `json:"status"`
	ExternalPaymentID  *string       `json:"externalPaymentId,omitempty"`
	ExternalTransferID *string       `json:"externalTransferId,omitempty"`
	FailureReason      *string       `json:"failureReason,omitempty"`
	CreatedAt          time.Time     `json:"createdAt"`
	UpdatedAt          time.Time     `json:"updatedAt"`
}

type webhookAckResponse struct {
	Accepted   bool               `json:"accepted"`
	Duplicate  bool               `json:"duplicate"`
	Conversion conversionResponse `json:"conversion"`
}

func newSuccessResponse[T any](data T) successResponse[T] {
	return successResponse[T]{
		Success:   true,
		EventTime: time.Now().UTC(),
		Data:      data,
	}
}

func toMoneyResponse(money shared.Money) moneyResponse {
	return moneyResponse{
		Currency:   money.Currency.String(),
		MinorUnits: money.MinorUnits,
		Scale:      money.Scale(),
		Amount:     money.AmountString(),
	}
}

func toRatesResponse(base shared.Currency, rates []shared.ExchangeRate) ratesResponse {
	response := ratesResponse{
		BaseCurrency: base.String(),
		Provider:     shared.ProviderFrankfurter,
		Rates:        make([]referenceRateResponse, 0, len(rates)),
	}

	for _, rate := range rates {
		response.Rates = append(response.Rates, referenceRateResponse{
			BaseCurrency:  rate.Base.String(),
			QuoteCurrency: rate.Quote.String(),
			Rate:          rate.Rate.String(),
			Provider:      rate.Provider,
			ObservedAt:    rate.ObservedAt.UTC(),
			FetchedAt:     rate.FetchedAt.UTC(),
		})
	}

	if len(rates) > 0 {
		response.Provider = rates[0].Provider
	}

	return response
}

func toQuoteResponse(value quote.Quote) quoteResponse {
	return quoteResponse{
		ID:               value.ID,
		IdempotencyKey:   value.IdempotencyKey.String(),
		BaseAmount:       toMoneyResponse(value.BaseAmount),
		QuoteAmount:      toMoneyResponse(value.QuoteAmount),
		FeeAmount:        toMoneyResponse(value.FeeAmount),
		TotalDebitAmount: toMoneyResponse(value.TotalDebitAmount),
		Rate:             value.Rate.String(),
		RateProvider:     value.RateProvider,
		ExpiresAt:        value.ExpiresAt.UTC(),
		AcceptedAt:       value.AcceptedAt,
		CreatedAt:        value.CreatedAt.UTC(),
	}
}

func toConversionResponse(value conversion.Conversion) conversionResponse {
	return conversionResponse{
		ID:                 value.ID,
		QuoteID:            value.QuoteID,
		IdempotencyKey:     value.IdempotencyKey.String(),
		BaseAmount:         toMoneyResponse(value.BaseAmount),
		QuoteAmount:        toMoneyResponse(value.QuoteAmount),
		FeeAmount:          toMoneyResponse(value.FeeAmount),
		TotalDebitAmount:   toMoneyResponse(value.TotalDebitAmount),
		Rate:               value.Rate.String(),
		RateProvider:       value.RateProvider,
		PaymentProvider:    value.PaymentProvider,
		TransferProvider:   value.TransferProvider,
		Status:             string(value.Status),
		ExternalPaymentID:  value.ExternalPaymentID,
		ExternalTransferID: value.ExternalTransferID,
		FailureReason:      value.FailureReason,
		CreatedAt:          value.CreatedAt.UTC(),
		UpdatedAt:          value.UpdatedAt.UTC(),
	}
}
