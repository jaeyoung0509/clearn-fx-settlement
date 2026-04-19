package httpadapter

import (
	"bytes"
	"encoding/json"
	"io"
	stdhttp "net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oklog/ulid/v2"

	"fx-settlement-lab/go-backend/internal/adapter/outbound/webhooksigning"
	"fx-settlement-lab/go-backend/internal/domain/shared"
	"fx-settlement-lab/go-backend/internal/usecase"
)

const webhookSignatureHeader = "X-Webhook-Signature"

type FXHandlerDeps struct {
	GetReferenceRates       *usecase.GetReferenceRatesUsecase
	CreateQuote             *usecase.CreateQuoteUsecase
	AcceptQuote             *usecase.AcceptQuoteUsecase
	GetConversion           *usecase.GetConversionUsecase
	HandlePaymentWebhook    *usecase.HandlePaymentWebhookUsecase
	HandleTransferWebhook   *usecase.HandleTransferWebhookUsecase
	PaymentWebhookVerifier  *webhooksigning.HMACVerifier
	TransferWebhookVerifier *webhooksigning.HMACVerifier
}

type FXHandler struct {
	getReferenceRates       *usecase.GetReferenceRatesUsecase
	createQuote             *usecase.CreateQuoteUsecase
	acceptQuote             *usecase.AcceptQuoteUsecase
	getConversion           *usecase.GetConversionUsecase
	handlePaymentWebhook    *usecase.HandlePaymentWebhookUsecase
	handleTransferWebhook   *usecase.HandleTransferWebhookUsecase
	paymentWebhookVerifier  *webhooksigning.HMACVerifier
	transferWebhookVerifier *webhooksigning.HMACVerifier
}

type getRatesQuery struct {
	Base   string `form:"base" binding:"required"`
	Quotes string `form:"quotes" binding:"required"`
}

type moneyRequest struct {
	Currency   string `json:"currency" binding:"required"`
	MinorUnits int64  `json:"minorUnits" binding:"required"`
}

type createQuoteRequest struct {
	BaseAmount    moneyRequest `json:"baseAmount" binding:"required"`
	QuoteCurrency string       `json:"quoteCurrency" binding:"required"`
}

type createConversionRequest struct {
	QuoteID          string `json:"quoteId" binding:"required"`
	PaymentProvider  string `json:"paymentProvider" binding:"omitempty,oneof=toss stripe demo"`
	TransferProvider string `json:"transferProvider" binding:"omitempty,oneof=wise plaid demo"`
}

type webhookRequest struct {
	Provider          string     `json:"provider" binding:"required"`
	ExternalEventID   string     `json:"externalEventId" binding:"required"`
	ConversionID      string     `json:"conversionId" binding:"required"`
	EventType         string     `json:"eventType" binding:"required"`
	ExternalReference *string    `json:"externalReference"`
	OccurredAt        *time.Time `json:"occurredAt"`
}

func NewFXHandler(deps FXHandlerDeps) *FXHandler {
	return &FXHandler{
		getReferenceRates:       deps.GetReferenceRates,
		createQuote:             deps.CreateQuote,
		acceptQuote:             deps.AcceptQuote,
		getConversion:           deps.GetConversion,
		handlePaymentWebhook:    deps.HandlePaymentWebhook,
		handleTransferWebhook:   deps.HandleTransferWebhook,
		paymentWebhookVerifier:  deps.PaymentWebhookVerifier,
		transferWebhookVerifier: deps.TransferWebhookVerifier,
	}
}

func (h *FXHandler) RegisterRoutes(group *gin.RouterGroup) {
	group.GET("/rates", h.GetRates)
	group.POST("/quotes", h.CreateQuote)
	group.POST("/conversions", h.CreateConversion)
	group.GET("/conversions/:conversionId", h.GetConversion)
	group.POST("/webhooks/payments", h.HandlePaymentWebhook)
	group.POST("/webhooks/transfers", h.HandleTransferWebhook)
}

func (h *FXHandler) GetRates(c *gin.Context) {
	var query getRatesQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		_ = c.Error(requestValidationError(err))
		return
	}

	baseCurrency, err := shared.ParseCurrency(query.Base)
	if err != nil {
		_ = c.Error(err)
		return
	}
	quoteCurrencies, err := parseCurrenciesCSV(query.Quotes)
	if err != nil {
		_ = c.Error(err)
		return
	}

	rates, err := h.getReferenceRates.Execute(c.Request.Context(), baseCurrency, quoteCurrencies)
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(stdhttp.StatusOK, newSuccessResponse(toRatesResponse(baseCurrency, rates)))
}

func (h *FXHandler) CreateQuote(c *gin.Context) {
	idempotencyKey, err := requiredIdempotencyKey(c)
	if err != nil {
		_ = c.Error(err)
		return
	}

	var request createQuoteRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		_ = c.Error(requestValidationError(err))
		return
	}

	baseCurrency, err := shared.ParseCurrency(request.BaseAmount.Currency)
	if err != nil {
		_ = c.Error(err)
		return
	}
	baseAmount, err := shared.NewMoney(baseCurrency, request.BaseAmount.MinorUnits)
	if err != nil {
		_ = c.Error(err)
		return
	}
	quoteCurrency, err := shared.ParseCurrency(request.QuoteCurrency)
	if err != nil {
		_ = c.Error(err)
		return
	}

	result, err := h.createQuote.Execute(c.Request.Context(), idempotencyKey, baseAmount, quoteCurrency)
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(stdhttp.StatusOK, newSuccessResponse(toQuoteResponse(result)))
}

func (h *FXHandler) CreateConversion(c *gin.Context) {
	idempotencyKey, err := requiredIdempotencyKey(c)
	if err != nil {
		_ = c.Error(err)
		return
	}

	var request createConversionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		_ = c.Error(requestValidationError(err))
		return
	}
	if err := validateULID("quoteId", request.QuoteID); err != nil {
		_ = c.Error(err)
		return
	}

	paymentProvider := request.PaymentProvider
	if paymentProvider == "" {
		paymentProvider = shared.ProviderToss
	}
	transferProvider := request.TransferProvider
	if transferProvider == "" {
		transferProvider = shared.ProviderWise
	}

	result, err := h.acceptQuote.Execute(c.Request.Context(), idempotencyKey, request.QuoteID, paymentProvider, transferProvider)
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(stdhttp.StatusOK, newSuccessResponse(toConversionResponse(result)))
}

func (h *FXHandler) GetConversion(c *gin.Context) {
	conversionID := c.Param("conversionId")
	if err := validateULID("conversionId", conversionID); err != nil {
		_ = c.Error(err)
		return
	}

	result, err := h.getConversion.Execute(c.Request.Context(), conversionID)
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(stdhttp.StatusOK, newSuccessResponse(toConversionResponse(result)))
}

func (h *FXHandler) HandlePaymentWebhook(c *gin.Context) {
	rawBody, request, err := readWebhookRequest(c, h.paymentWebhookVerifier)
	if err != nil {
		_ = c.Error(err)
		return
	}

	occurredAt := time.Now().UTC()
	if request.OccurredAt != nil {
		occurredAt = request.OccurredAt.UTC()
	}

	result, duplicate, err := h.handlePaymentWebhook.Execute(c.Request.Context(), shared.ProviderEvent{
		Provider:          request.Provider,
		Topic:             request.EventType,
		ExternalEventID:   request.ExternalEventID,
		ConversionID:      request.ConversionID,
		ExternalReference: request.ExternalReference,
		OccurredAt:        occurredAt,
		Payload:           rawBody,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(stdhttp.StatusOK, newSuccessResponse(webhookAckResponse{
		Accepted:   true,
		Duplicate:  duplicate,
		Conversion: toConversionResponse(result),
	}))
}

func (h *FXHandler) HandleTransferWebhook(c *gin.Context) {
	rawBody, request, err := readWebhookRequest(c, h.transferWebhookVerifier)
	if err != nil {
		_ = c.Error(err)
		return
	}

	occurredAt := time.Now().UTC()
	if request.OccurredAt != nil {
		occurredAt = request.OccurredAt.UTC()
	}

	result, duplicate, err := h.handleTransferWebhook.Execute(c.Request.Context(), shared.ProviderEvent{
		Provider:          request.Provider,
		Topic:             request.EventType,
		ExternalEventID:   request.ExternalEventID,
		ConversionID:      request.ConversionID,
		ExternalReference: request.ExternalReference,
		OccurredAt:        occurredAt,
		Payload:           rawBody,
	})
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(stdhttp.StatusOK, newSuccessResponse(webhookAckResponse{
		Accepted:   true,
		Duplicate:  duplicate,
		Conversion: toConversionResponse(result),
	}))
}

func requiredIdempotencyKey(c *gin.Context) (shared.IdempotencyKey, error) {
	return shared.ParseIdempotencyKey(c.GetHeader("Idempotency-Key"))
}

func parseCurrenciesCSV(raw string) ([]shared.Currency, error) {
	parts := strings.Split(raw, ",")
	values := make([]shared.Currency, 0, len(parts))
	for _, part := range parts {
		currency, err := shared.ParseCurrency(part)
		if err != nil {
			return nil, err
		}
		values = append(values, currency)
	}

	return values, nil
}

func validateULID(field string, raw string) error {
	if _, err := ulid.ParseStrict(raw); err != nil {
		return requestValidationError(err)
	}

	return nil
}

func readWebhookRequest(c *gin.Context, verifier *webhooksigning.HMACVerifier) (json.RawMessage, webhookRequest, error) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, webhookRequest{}, err
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))

	if err := verifier.Verify(c.GetHeader(webhookSignatureHeader), body); err != nil {
		return nil, webhookRequest{}, err
	}

	var request webhookRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return nil, webhookRequest{}, requestValidationError(err)
	}
	if err := validateULID("conversionId", request.ConversionID); err != nil {
		return nil, webhookRequest{}, err
	}

	return json.RawMessage(body), request, nil
}
