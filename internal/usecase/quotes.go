package usecase

import (
	"context"
	"time"

	"fx-settlement-lab/go-backend/internal/domain/conversion"
	"fx-settlement-lab/go-backend/internal/domain/quote"
	"fx-settlement-lab/go-backend/internal/domain/shared"
	"fx-settlement-lab/go-backend/internal/port"
)

type CreateQuoteUsecase struct {
	repository port.QuoteRepository
	rateRepo   port.ReferenceRateRepository
	provider   port.RateProvider
	clock      port.Clock
	telemetry  port.Telemetry
	quoteTTL   time.Duration
	feeBPS     int64
	minFee     shared.Money
}

func NewCreateQuoteUsecase(
	repository port.QuoteRepository,
	rateRepo port.ReferenceRateRepository,
	provider port.RateProvider,
	clock port.Clock,
	telemetry port.Telemetry,
	quoteTTL time.Duration,
	feeBPS int64,
	minFee shared.Money,
) *CreateQuoteUsecase {
	return &CreateQuoteUsecase{
		repository: repository,
		rateRepo:   rateRepo,
		provider:   provider,
		clock:      clock,
		telemetry:  telemetry,
		quoteTTL:   quoteTTL,
		feeBPS:     feeBPS,
		minFee:     minFee,
	}
}

func (u *CreateQuoteUsecase) Execute(
	ctx context.Context,
	idempotencyKey shared.IdempotencyKey,
	baseAmount shared.Money,
	quoteCurrency shared.Currency,
) (quote.Quote, error) {
	ctx, finish := u.telemetry.StartSpan(ctx, "usecase.create_quote")
	defer func() { finish(nil) }()

	if existing, found, err := u.repository.GetQuoteByIdempotencyKey(ctx, idempotencyKey); err != nil {
		finish(err)
		return quote.Quote{}, err
	} else if found {
		return existing, nil
	}

	rates, err := ensureReferenceRates(ctx, u.rateRepo, u.provider, baseAmount.Currency, []shared.Currency{quoteCurrency})
	if err != nil {
		finish(err)
		return quote.Quote{}, err
	}

	created, err := quote.New(
		newULID(),
		idempotencyKey,
		baseAmount,
		quoteCurrency,
		rates[0],
		u.feeBPS,
		u.minFee,
		u.clock.Now(),
		u.quoteTTL,
	)
	if err != nil {
		finish(err)
		return quote.Quote{}, err
	}

	saved, err := u.repository.CreateQuote(ctx, created)
	if err != nil {
		finish(err)
		return quote.Quote{}, err
	}

	return saved, nil
}

type AcceptQuoteUsecase struct {
	quotes      port.QuoteRepository
	conversions port.ConversionRepository
	unitOfWork  port.UnitOfWork
	clock       port.Clock
	telemetry   port.Telemetry
}

func NewAcceptQuoteUsecase(
	quotes port.QuoteRepository,
	conversions port.ConversionRepository,
	unitOfWork port.UnitOfWork,
	clock port.Clock,
	telemetry port.Telemetry,
) *AcceptQuoteUsecase {
	return &AcceptQuoteUsecase{
		quotes:      quotes,
		conversions: conversions,
		unitOfWork:  unitOfWork,
		clock:       clock,
		telemetry:   telemetry,
	}
}

func (u *AcceptQuoteUsecase) Execute(
	ctx context.Context,
	idempotencyKey shared.IdempotencyKey,
	quoteID string,
	paymentProvider string,
	transferProvider string,
) (conversion.Conversion, error) {
	ctx, finish := u.telemetry.StartSpan(ctx, "usecase.accept_quote")
	defer func() { finish(nil) }()

	if existing, found, err := u.conversions.GetConversionByIdempotencyKey(ctx, idempotencyKey); err != nil {
		finish(err)
		return conversion.Conversion{}, err
	} else if found {
		return existing, nil
	}

	var created conversion.Conversion
	now := u.clock.Now()

	err := u.unitOfWork.WithinTransaction(ctx, func(txCtx context.Context, repos port.TransactionRepositories) error {
		sourceQuote, err := repos.Quotes.GetQuoteByID(txCtx, quoteID)
		if err != nil {
			return err
		}
		if err := sourceQuote.CanAccept(now); err != nil {
			return err
		}

		created, err = conversion.FromQuote(
			newULID(),
			idempotencyKey,
			sourceQuote,
			paymentProvider,
			transferProvider,
			now,
		)
		if err != nil {
			return err
		}

		if _, err := repos.Conversions.CreateConversion(txCtx, created); err != nil {
			return err
		}
		if err := repos.Quotes.MarkQuoteAccepted(txCtx, quoteID, now); err != nil {
			return err
		}

		return repos.Outbox.Enqueue(txCtx, newOutboxEvent(
			"conversion",
			created.ID,
			"conversion.created",
			map[string]any{
				"conversionId": created.ID,
				"quoteId":      created.QuoteID,
				"status":       created.Status,
			},
			now,
		))
	})
	if err != nil {
		finish(err)
		return conversion.Conversion{}, err
	}

	return created, nil
}
