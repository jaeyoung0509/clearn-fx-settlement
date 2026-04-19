package usecase

import (
	"context"
	"time"

	"fx-settlement-lab/go-backend/internal/port"
)

type PublishOutboxResult struct {
	Processed int `json:"processed"`
	Published int `json:"published"`
	Failed    int `json:"failed"`
}

type PublishOutboxUsecase struct {
	repository port.OutboxRepository
	publisher  port.EventPublisher
	telemetry  port.Telemetry
}

func NewPublishOutboxUsecase(repository port.OutboxRepository, publisher port.EventPublisher, telemetry port.Telemetry) *PublishOutboxUsecase {
	return &PublishOutboxUsecase{
		repository: repository,
		publisher:  publisher,
		telemetry:  telemetry,
	}
}

func (u *PublishOutboxUsecase) Execute(ctx context.Context, limit int) (PublishOutboxResult, error) {
	ctx, finish := u.telemetry.StartSpan(ctx, "usecase.publish_outbox")
	defer func() { finish(nil) }()

	events, err := u.repository.ListPending(ctx, limit)
	if err != nil {
		finish(err)
		return PublishOutboxResult{}, err
	}

	result := PublishOutboxResult{Processed: len(events)}

	for _, event := range events {
		if err := u.publisher.Publish(ctx, event); err != nil {
			u.telemetry.RecordOutboxPublishFailure(ctx, event.EventType)
			if markErr := u.repository.MarkPublishFailed(ctx, event.ID, err.Error()); markErr != nil {
				finish(markErr)
				return PublishOutboxResult{}, markErr
			}
			result.Failed++
			continue
		}

		u.telemetry.RecordOutboxPublished(ctx, event.EventType)
		if err := u.repository.MarkPublished(ctx, event.ID, time.Now().UTC()); err != nil {
			finish(err)
			return PublishOutboxResult{}, err
		}
		result.Published++
	}

	return result, nil
}
