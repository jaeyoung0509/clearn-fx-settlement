package usecase

import (
	"context"

	"fx-settlement-lab/go-backend/internal/domain/conversion"
	"fx-settlement-lab/go-backend/internal/port"
)

type GetConversionUsecase struct {
	repository port.ConversionRepository
	telemetry  port.Telemetry
}

func NewGetConversionUsecase(repository port.ConversionRepository, telemetry port.Telemetry) *GetConversionUsecase {
	return &GetConversionUsecase{
		repository: repository,
		telemetry:  telemetry,
	}
}

func (u *GetConversionUsecase) Execute(ctx context.Context, conversionID string) (conversion.Conversion, error) {
	ctx, finish := u.telemetry.StartSpan(ctx, "usecase.get_conversion")
	defer func() { finish(nil) }()

	value, err := u.repository.GetConversionByID(ctx, conversionID)
	if err != nil {
		finish(err)
		return conversion.Conversion{}, err
	}

	return value, nil
}
