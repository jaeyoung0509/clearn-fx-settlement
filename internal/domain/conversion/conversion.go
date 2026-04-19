package conversion

import (
	"time"

	"github.com/shopspring/decimal"

	"fx-settlement-lab/go-backend/internal/domain"
	"fx-settlement-lab/go-backend/internal/domain/quote"
	"fx-settlement-lab/go-backend/internal/domain/shared"
)

type Status string

const (
	StatusRequested       Status = "REQUESTED"
	StatusAwaitingPayment Status = "AWAITING_PAYMENT"
	StatusProcessing      Status = "PROCESSING"
	StatusCompleted       Status = "COMPLETED"
	StatusFailed          Status = "FAILED"
	StatusCancelled       Status = "CANCELLED"
)

type Conversion struct {
	ID                 string
	QuoteID            string
	IdempotencyKey     shared.IdempotencyKey
	BaseAmount         shared.Money
	QuoteAmount        shared.Money
	FeeAmount          shared.Money
	TotalDebitAmount   shared.Money
	Rate               decimal.Decimal
	RateProvider       string
	PaymentProvider    string
	TransferProvider   string
	Status             Status
	ExternalPaymentID  *string
	ExternalTransferID *string
	FailureReason      *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func FromQuote(
	id string,
	idempotencyKey shared.IdempotencyKey,
	sourceQuote quote.Quote,
	paymentProvider string,
	transferProvider string,
	now time.Time,
) (Conversion, error) {
	if id == "" {
		return Conversion{}, domain.Validation("Conversion ID is required", nil)
	}
	if !shared.IsSupportedPaymentProvider(paymentProvider) {
		return Conversion{}, domain.Validation("Unsupported payment provider", map[string]any{
			"provider": paymentProvider,
		})
	}
	if !shared.IsSupportedTransferProvider(transferProvider) {
		return Conversion{}, domain.Validation("Unsupported transfer provider", map[string]any{
			"provider": transferProvider,
		})
	}
	if err := sourceQuote.CanAccept(now); err != nil {
		return Conversion{}, err
	}

	return Conversion{
		ID:               id,
		QuoteID:          sourceQuote.ID,
		IdempotencyKey:   idempotencyKey,
		BaseAmount:       sourceQuote.BaseAmount,
		QuoteAmount:      sourceQuote.QuoteAmount,
		FeeAmount:        sourceQuote.FeeAmount,
		TotalDebitAmount: sourceQuote.TotalDebitAmount,
		Rate:             sourceQuote.Rate,
		RateProvider:     sourceQuote.RateProvider,
		PaymentProvider:  paymentProvider,
		TransferProvider: transferProvider,
		Status:           StatusAwaitingPayment,
		CreatedAt:        now.UTC(),
		UpdatedAt:        now.UTC(),
	}, nil
}

func (c Conversion) AdvanceForPayment(externalReference string, now time.Time) (Conversion, error) {
	switch c.Status {
	case StatusAwaitingPayment, StatusRequested:
	default:
		return Conversion{}, domain.Conflict("Conversion cannot move to processing from current status", map[string]any{
			"conversionId": c.ID,
			"status":       c.Status,
		})
	}

	c.Status = StatusProcessing
	c.UpdatedAt = now.UTC()
	if externalReference != "" {
		c.ExternalPaymentID = &externalReference
	}

	return c, nil
}

func (c Conversion) CompleteTransfer(externalReference string, now time.Time) (Conversion, error) {
	if c.Status != StatusProcessing {
		return Conversion{}, domain.Conflict("Conversion cannot complete from current status", map[string]any{
			"conversionId": c.ID,
			"status":       c.Status,
		})
	}

	c.Status = StatusCompleted
	c.UpdatedAt = now.UTC()
	if externalReference != "" {
		c.ExternalTransferID = &externalReference
	}

	return c, nil
}

func (c Conversion) FailPayment(reason string, externalReference string, now time.Time) Conversion {
	c.Status = StatusFailed
	c.UpdatedAt = now.UTC()
	if reason != "" {
		c.FailureReason = &reason
	}
	if externalReference != "" {
		if c.ExternalPaymentID == nil {
			c.ExternalPaymentID = &externalReference
		}
	}

	return c
}

func (c Conversion) FailTransfer(reason string, externalReference string, now time.Time) Conversion {
	c.Status = StatusFailed
	c.UpdatedAt = now.UTC()
	if reason != "" {
		c.FailureReason = &reason
	}
	if externalReference != "" {
		if c.ExternalTransferID == nil {
			c.ExternalTransferID = &externalReference
		}
	}

	return c
}
