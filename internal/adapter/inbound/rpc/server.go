package rpcadapter

import (
	"net/rpc"
	"time"

	"github.com/oklog/ulid/v2"

	"fx-settlement-lab/go-backend/internal/domain"
	"fx-settlement-lab/go-backend/internal/domain/conversion"
	"fx-settlement-lab/go-backend/internal/domain/quote"
	"fx-settlement-lab/go-backend/internal/domain/shared"
	"fx-settlement-lab/go-backend/internal/usecase"
)

const serviceName = "FXRPCService"

type ServerDeps struct {
	GetReferenceRates *usecase.GetReferenceRatesUsecase
	CreateQuote       *usecase.CreateQuoteUsecase
	AcceptQuote       *usecase.AcceptQuoteUsecase
	GetConversion     *usecase.GetConversionUsecase
}

type Server struct {
	getReferenceRates *usecase.GetReferenceRatesUsecase
	createQuote       *usecase.CreateQuoteUsecase
	acceptQuote       *usecase.AcceptQuoteUsecase
	getConversion     *usecase.GetConversionUsecase
}

type MoneyInput struct {
	Currency   string
	MinorUnits int64
}

type Money struct {
	Currency   string
	MinorUnits int64
	Scale      int32
	Amount     string
}

type ReferenceRate struct {
	BaseCurrency  string
	QuoteCurrency string
	Rate          string
	Provider      string
	ObservedAt    time.Time
	FetchedAt     time.Time
}

type GetRatesArgs struct {
	Base   string
	Quotes []string
}

type GetRatesReply struct {
	BaseCurrency string
	Provider     string
	Rates        []ReferenceRate
}

type CreateQuoteArgs struct {
	IdempotencyKey string
	BaseAmount     MoneyInput
	QuoteCurrency  string
}

type QuoteReply struct {
	ID               string
	IdempotencyKey   string
	BaseAmount       Money
	QuoteAmount      Money
	FeeAmount        Money
	TotalDebitAmount Money
	Rate             string
	RateProvider     string
	ExpiresAt        time.Time
	AcceptedAt       *time.Time
	CreatedAt        time.Time
}

type CreateConversionArgs struct {
	IdempotencyKey   string
	QuoteID          string
	PaymentProvider  string
	TransferProvider string
}

type GetConversionArgs struct {
	ConversionID string
}

type ConversionReply struct {
	ID                 string
	QuoteID            string
	IdempotencyKey     string
	BaseAmount         Money
	QuoteAmount        Money
	FeeAmount          Money
	TotalDebitAmount   Money
	Rate               string
	RateProvider       string
	PaymentProvider    string
	TransferProvider   string
	Status             string
	ExternalPaymentID  *string
	ExternalTransferID *string
	FailureReason      *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func NewServer(deps ServerDeps) (*rpc.Server, error) {
	server := rpc.NewServer()
	err := server.RegisterName(serviceName, &Server{
		getReferenceRates: deps.GetReferenceRates,
		createQuote:       deps.CreateQuote,
		acceptQuote:       deps.AcceptQuote,
		getConversion:     deps.GetConversion,
	})
	if err != nil {
		return nil, err
	}

	return server, nil
}

func (s *Server) GetRates(args GetRatesArgs, reply *GetRatesReply) error {
	baseCurrency, err := shared.ParseCurrency(args.Base)
	if err != nil {
		return err
	}
	quoteCurrencies, err := parseCurrencies(args.Quotes)
	if err != nil {
		return err
	}

	rates, err := s.getReferenceRates.Execute(nilContext(), baseCurrency, quoteCurrencies)
	if err != nil {
		return err
	}

	reply.BaseCurrency = baseCurrency.String()
	reply.Provider = shared.ProviderFrankfurter
	reply.Rates = make([]ReferenceRate, 0, len(rates))
	for _, rate := range rates {
		reply.Rates = append(reply.Rates, toReferenceRateReply(rate))
	}
	if len(rates) > 0 {
		reply.Provider = rates[0].Provider
	}

	return nil
}

func (s *Server) CreateQuote(args CreateQuoteArgs, reply *QuoteReply) error {
	idempotencyKey, err := shared.ParseIdempotencyKey(args.IdempotencyKey)
	if err != nil {
		return err
	}

	baseCurrency, err := shared.ParseCurrency(args.BaseAmount.Currency)
	if err != nil {
		return err
	}
	baseAmount, err := shared.NewMoney(baseCurrency, args.BaseAmount.MinorUnits)
	if err != nil {
		return err
	}
	quoteCurrency, err := shared.ParseCurrency(args.QuoteCurrency)
	if err != nil {
		return err
	}

	value, err := s.createQuote.Execute(nilContext(), idempotencyKey, baseAmount, quoteCurrency)
	if err != nil {
		return err
	}

	*reply = toQuoteReply(value)
	return nil
}

func (s *Server) CreateConversion(args CreateConversionArgs, reply *ConversionReply) error {
	idempotencyKey, err := shared.ParseIdempotencyKey(args.IdempotencyKey)
	if err != nil {
		return err
	}
	if err := validateULID("quoteId", args.QuoteID); err != nil {
		return err
	}

	paymentProvider := args.PaymentProvider
	if paymentProvider == "" {
		paymentProvider = shared.ProviderToss
	}
	transferProvider := args.TransferProvider
	if transferProvider == "" {
		transferProvider = shared.ProviderWise
	}

	value, err := s.acceptQuote.Execute(nilContext(), idempotencyKey, args.QuoteID, paymentProvider, transferProvider)
	if err != nil {
		return err
	}

	*reply = toConversionReply(value)
	return nil
}

func (s *Server) GetConversion(args GetConversionArgs, reply *ConversionReply) error {
	if err := validateULID("conversionId", args.ConversionID); err != nil {
		return err
	}

	value, err := s.getConversion.Execute(nilContext(), args.ConversionID)
	if err != nil {
		return err
	}

	*reply = toConversionReply(value)
	return nil
}

func parseCurrencies(rawValues []string) ([]shared.Currency, error) {
	if len(rawValues) == 0 {
		return nil, domain.Validation("At least one quote currency is required", nil)
	}

	values := make([]shared.Currency, 0, len(rawValues))
	for _, rawValue := range rawValues {
		currency, err := shared.ParseCurrency(rawValue)
		if err != nil {
			return nil, err
		}
		values = append(values, currency)
	}

	return values, nil
}

func validateULID(field string, raw string) error {
	if _, err := ulid.ParseStrict(raw); err != nil {
		return domain.Validation("Request validation failed", map[string]any{
			"errors": []map[string]string{{
				"field": field,
				"tag":   "ulid",
			}},
		}).WithCause(err)
	}

	return nil
}

func toMoneyReply(value shared.Money) Money {
	return Money{
		Currency:   value.Currency.String(),
		MinorUnits: value.MinorUnits,
		Scale:      value.Scale(),
		Amount:     value.AmountString(),
	}
}

func toReferenceRateReply(value shared.ExchangeRate) ReferenceRate {
	return ReferenceRate{
		BaseCurrency:  value.Base.String(),
		QuoteCurrency: value.Quote.String(),
		Rate:          value.Rate.String(),
		Provider:      value.Provider,
		ObservedAt:    value.ObservedAt.UTC(),
		FetchedAt:     value.FetchedAt.UTC(),
	}
}

func toQuoteReply(value quote.Quote) QuoteReply {
	return QuoteReply{
		ID:               value.ID,
		IdempotencyKey:   value.IdempotencyKey.String(),
		BaseAmount:       toMoneyReply(value.BaseAmount),
		QuoteAmount:      toMoneyReply(value.QuoteAmount),
		FeeAmount:        toMoneyReply(value.FeeAmount),
		TotalDebitAmount: toMoneyReply(value.TotalDebitAmount),
		Rate:             value.Rate.String(),
		RateProvider:     value.RateProvider,
		ExpiresAt:        value.ExpiresAt.UTC(),
		AcceptedAt:       value.AcceptedAt,
		CreatedAt:        value.CreatedAt.UTC(),
	}
}

func toConversionReply(value conversion.Conversion) ConversionReply {
	return ConversionReply{
		ID:                 value.ID,
		QuoteID:            value.QuoteID,
		IdempotencyKey:     value.IdempotencyKey.String(),
		BaseAmount:         toMoneyReply(value.BaseAmount),
		QuoteAmount:        toMoneyReply(value.QuoteAmount),
		FeeAmount:          toMoneyReply(value.FeeAmount),
		TotalDebitAmount:   toMoneyReply(value.TotalDebitAmount),
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
