package usecase

import (
	"context"

	"fx-settlement-lab/go-backend/internal/domain/shared"
	"fx-settlement-lab/go-backend/internal/port"
)

type SyncReferenceRatesUsecase struct {
	repository port.ReferenceRateRepository
	provider   port.RateProvider
	telemetry  port.Telemetry
}

func NewSyncReferenceRatesUsecase(repository port.ReferenceRateRepository, provider port.RateProvider, telemetry port.Telemetry) *SyncReferenceRatesUsecase {
	return &SyncReferenceRatesUsecase{
		repository: repository,
		provider:   provider,
		telemetry:  telemetry,
	}
}

func (u *SyncReferenceRatesUsecase) Execute(ctx context.Context, base shared.Currency, quotes []shared.Currency) ([]shared.ExchangeRate, error) {
	ctx, finish := u.telemetry.StartSpan(ctx, "usecase.sync_reference_rates")
	defer func() { finish(nil) }()

	rates, err := u.provider.GetReferenceRates(ctx, base, quotes)
	if err != nil {
		finish(err)
		return nil, err
	}
	if err := u.repository.UpsertRates(ctx, rates); err != nil {
		finish(err)
		return nil, err
	}

	return sortRates(rates, quotes), nil
}

type GetReferenceRatesUsecase struct {
	repository port.ReferenceRateRepository
	provider   port.RateProvider
	telemetry  port.Telemetry
}

func NewGetReferenceRatesUsecase(repository port.ReferenceRateRepository, provider port.RateProvider, telemetry port.Telemetry) *GetReferenceRatesUsecase {
	return &GetReferenceRatesUsecase{
		repository: repository,
		provider:   provider,
		telemetry:  telemetry,
	}
}

func (u *GetReferenceRatesUsecase) Execute(ctx context.Context, base shared.Currency, quotes []shared.Currency) ([]shared.ExchangeRate, error) {
	ctx, finish := u.telemetry.StartSpan(ctx, "usecase.get_reference_rates")
	defer func() { finish(nil) }()

	rates, err := ensureReferenceRates(ctx, u.repository, u.provider, base, quotes)
	if err != nil {
		finish(err)
		return nil, err
	}

	return rates, nil
}
