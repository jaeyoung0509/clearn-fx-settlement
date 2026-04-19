package grpcadapter

import (
	"context"
	"time"

	"github.com/oklog/ulid/v2"
	grpcpkg "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"fx-settlement-lab/go-backend/internal/domain"
	"fx-settlement-lab/go-backend/internal/domain/conversion"
	"fx-settlement-lab/go-backend/internal/domain/quote"
	"fx-settlement-lab/go-backend/internal/domain/shared"
	"fx-settlement-lab/go-backend/internal/port"
	"fx-settlement-lab/go-backend/internal/usecase"
	fxv1 "fx-settlement-lab/go-backend/proto/fx/v1"
)

const idempotencyMetadataKey = "idempotency-key"

type ServerDeps struct {
	GetReferenceRates *usecase.GetReferenceRatesUsecase
	CreateQuote       *usecase.CreateQuoteUsecase
	AcceptQuote       *usecase.AcceptQuoteUsecase
	GetConversion     *usecase.GetConversionUsecase
	Telemetry         port.Telemetry
}

type FXService struct {
	fxv1.UnimplementedFXServiceServer
	getReferenceRates *usecase.GetReferenceRatesUsecase
	createQuote       *usecase.CreateQuoteUsecase
	acceptQuote       *usecase.AcceptQuoteUsecase
	getConversion     *usecase.GetConversionUsecase
}

var _ fxv1.FXServiceServer = (*FXService)(nil)

func NewServer(deps ServerDeps) *grpcpkg.Server {
	options := make([]grpcpkg.ServerOption, 0, 1)
	if deps.Telemetry != nil {
		options = append(options, grpcpkg.UnaryInterceptor(unaryMetricsInterceptor(deps.Telemetry)))
	}

	server := grpcpkg.NewServer(options...)
	fxv1.RegisterFXServiceServer(server, &FXService{
		getReferenceRates: deps.GetReferenceRates,
		createQuote:       deps.CreateQuote,
		acceptQuote:       deps.AcceptQuote,
		getConversion:     deps.GetConversion,
	})
	reflection.Register(server)

	return server
}

func unaryMetricsInterceptor(telemetry port.Telemetry) grpcpkg.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpcpkg.UnaryServerInfo, handler grpcpkg.UnaryHandler) (any, error) {
		started := time.Now()
		response, err := handler(ctx, req)
		telemetry.RecordInboundRequest(ctx, "grpc", info.FullMethod, classifyGRPCOutcome(status.Code(err)), time.Since(started))

		return response, err
	}
}

func classifyGRPCOutcome(code codes.Code) string {
	switch code {
	case codes.OK:
		return "success"
	case codes.InvalidArgument, codes.NotFound, codes.FailedPrecondition, codes.Unauthenticated:
		return "client_error"
	default:
		return "server_error"
	}
}

func (s *FXService) GetRates(ctx context.Context, req *fxv1.GetRatesRequest) (*fxv1.GetRatesResponse, error) {
	baseCurrency, err := shared.ParseCurrency(req.GetBase())
	if err != nil {
		return nil, mapError(err)
	}
	quoteCurrencies, err := parseCurrencies(req.GetQuotes())
	if err != nil {
		return nil, mapError(err)
	}

	rates, err := s.getReferenceRates.Execute(ctx, baseCurrency, quoteCurrencies)
	if err != nil {
		return nil, mapError(err)
	}

	response := &fxv1.GetRatesResponse{
		BaseCurrency: baseCurrency.String(),
		Provider:     shared.ProviderFrankfurter,
		Rates:        make([]*fxv1.ReferenceRate, 0, len(rates)),
	}
	for _, rate := range rates {
		response.Rates = append(response.Rates, toReferenceRateMessage(rate))
	}
	if len(rates) > 0 {
		response.Provider = rates[0].Provider
	}

	return response, nil
}

func (s *FXService) CreateQuote(ctx context.Context, req *fxv1.CreateQuoteRequest) (*fxv1.Quote, error) {
	idempotencyKey, err := requiredIdempotencyKey(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	if req.GetBaseAmount() == nil {
		return nil, mapError(domain.Validation("Base amount is required", nil))
	}

	baseCurrency, err := shared.ParseCurrency(req.GetBaseAmount().GetCurrency())
	if err != nil {
		return nil, mapError(err)
	}
	baseAmount, err := shared.NewMoney(baseCurrency, req.GetBaseAmount().GetMinorUnits())
	if err != nil {
		return nil, mapError(err)
	}
	quoteCurrency, err := shared.ParseCurrency(req.GetQuoteCurrency())
	if err != nil {
		return nil, mapError(err)
	}

	value, err := s.createQuote.Execute(ctx, idempotencyKey, baseAmount, quoteCurrency)
	if err != nil {
		return nil, mapError(err)
	}

	return toQuoteMessage(value), nil
}

func (s *FXService) CreateConversion(ctx context.Context, req *fxv1.CreateConversionRequest) (*fxv1.Conversion, error) {
	idempotencyKey, err := requiredIdempotencyKey(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	if err := validateULID("quoteId", req.GetQuoteId()); err != nil {
		return nil, mapError(err)
	}

	paymentProvider := req.GetPaymentProvider()
	if paymentProvider == "" {
		paymentProvider = shared.ProviderToss
	}
	transferProvider := req.GetTransferProvider()
	if transferProvider == "" {
		transferProvider = shared.ProviderWise
	}

	value, err := s.acceptQuote.Execute(ctx, idempotencyKey, req.GetQuoteId(), paymentProvider, transferProvider)
	if err != nil {
		return nil, mapError(err)
	}

	return toConversionMessage(value), nil
}

func (s *FXService) GetConversion(ctx context.Context, req *fxv1.GetConversionRequest) (*fxv1.Conversion, error) {
	if err := validateULID("conversionId", req.GetConversionId()); err != nil {
		return nil, mapError(err)
	}

	value, err := s.getConversion.Execute(ctx, req.GetConversionId())
	if err != nil {
		return nil, mapError(err)
	}

	return toConversionMessage(value), nil
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

func requiredIdempotencyKey(ctx context.Context) (shared.IdempotencyKey, error) {
	metadataValues, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return shared.ParseIdempotencyKey("")
	}

	values := metadataValues.Get(idempotencyMetadataKey)
	if len(values) == 0 {
		return shared.ParseIdempotencyKey("")
	}

	return shared.ParseIdempotencyKey(values[0])
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

func mapError(err error) error {
	appErr := domain.AsAppError(err)

	grpcCode := codes.Internal
	switch appErr.Code {
	case domain.ErrorCodeNotFound:
		grpcCode = codes.NotFound
	case domain.ErrorCodeValidation:
		grpcCode = codes.InvalidArgument
	case domain.ErrorCodeConflict:
		grpcCode = codes.FailedPrecondition
	case domain.ErrorCodeUnauthorized:
		grpcCode = codes.Unauthenticated
	}

	return status.Error(grpcCode, appErr.Message)
}

func toMoneyMessage(value shared.Money) *fxv1.Money {
	return &fxv1.Money{
		Currency:   value.Currency.String(),
		MinorUnits: value.MinorUnits,
		Scale:      value.Scale(),
		Amount:     value.AmountString(),
	}
}

func toReferenceRateMessage(value shared.ExchangeRate) *fxv1.ReferenceRate {
	return &fxv1.ReferenceRate{
		BaseCurrency:  value.Base.String(),
		QuoteCurrency: value.Quote.String(),
		Rate:          value.Rate.String(),
		Provider:      value.Provider,
		ObservedAt:    timestamppb.New(value.ObservedAt.UTC()),
		FetchedAt:     timestamppb.New(value.FetchedAt.UTC()),
	}
}

func toQuoteMessage(value quote.Quote) *fxv1.Quote {
	message := &fxv1.Quote{
		Id:               value.ID,
		IdempotencyKey:   value.IdempotencyKey.String(),
		BaseAmount:       toMoneyMessage(value.BaseAmount),
		QuoteAmount:      toMoneyMessage(value.QuoteAmount),
		FeeAmount:        toMoneyMessage(value.FeeAmount),
		TotalDebitAmount: toMoneyMessage(value.TotalDebitAmount),
		Rate:             value.Rate.String(),
		RateProvider:     value.RateProvider,
		ExpiresAt:        timestamppb.New(value.ExpiresAt.UTC()),
		CreatedAt:        timestamppb.New(value.CreatedAt.UTC()),
	}
	if value.AcceptedAt != nil {
		message.AcceptedAt = timestamppb.New(value.AcceptedAt.UTC())
	}

	return message
}

func toConversionMessage(value conversion.Conversion) *fxv1.Conversion {
	message := &fxv1.Conversion{
		Id:               value.ID,
		QuoteId:          value.QuoteID,
		IdempotencyKey:   value.IdempotencyKey.String(),
		BaseAmount:       toMoneyMessage(value.BaseAmount),
		QuoteAmount:      toMoneyMessage(value.QuoteAmount),
		FeeAmount:        toMoneyMessage(value.FeeAmount),
		TotalDebitAmount: toMoneyMessage(value.TotalDebitAmount),
		Rate:             value.Rate.String(),
		RateProvider:     value.RateProvider,
		PaymentProvider:  value.PaymentProvider,
		TransferProvider: value.TransferProvider,
		Status:           string(value.Status),
		CreatedAt:        timestamppb.New(value.CreatedAt.UTC()),
		UpdatedAt:        timestamppb.New(value.UpdatedAt.UTC()),
	}
	if value.ExternalPaymentID != nil {
		message.ExternalPaymentId = *value.ExternalPaymentID
	}
	if value.ExternalTransferID != nil {
		message.ExternalTransferId = *value.ExternalTransferID
	}
	if value.FailureReason != nil {
		message.FailureReason = *value.FailureReason
	}

	return message
}
