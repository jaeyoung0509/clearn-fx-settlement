package usecase

import (
	"context"

	"fx-settlement-lab/go-backend/internal/domain"
	"fx-settlement-lab/go-backend/internal/domain/conversion"
	"fx-settlement-lab/go-backend/internal/domain/shared"
	"fx-settlement-lab/go-backend/internal/domain/webhook"
	"fx-settlement-lab/go-backend/internal/port"
)

type HandlePaymentWebhookUsecase struct {
	conversions port.ConversionRepository
	unitOfWork  port.UnitOfWork
	clock       port.Clock
	telemetry   port.Telemetry
}

func NewHandlePaymentWebhookUsecase(
	conversions port.ConversionRepository,
	unitOfWork port.UnitOfWork,
	clock port.Clock,
	telemetry port.Telemetry,
) *HandlePaymentWebhookUsecase {
	return &HandlePaymentWebhookUsecase{
		conversions: conversions,
		unitOfWork:  unitOfWork,
		clock:       clock,
		telemetry:   telemetry,
	}
}

func (u *HandlePaymentWebhookUsecase) Execute(ctx context.Context, event shared.ProviderEvent) (conversion.Conversion, bool, error) {
	ctx, finish := u.telemetry.StartSpan(ctx, "usecase.handle_payment_webhook")
	defer func() { finish(nil) }()

	var updated conversion.Conversion
	var duplicate bool
	now := u.clock.Now()

	err := u.unitOfWork.WithinTransaction(ctx, func(txCtx context.Context, repos port.TransactionRepositories) error {
		processedAt := now.UTC()
		stored, err := repos.WebhookInbox.StoreIfAbsent(txCtx, webhook.InboxMessage{
			ID:              newULID(),
			Provider:        event.Provider,
			Topic:           event.Topic,
			ExternalEventID: event.ExternalEventID,
			ConversionID:    event.ConversionID,
			ExternalRef:     event.ExternalReference,
			Payload:         event.Payload,
			ReceivedAt:      now.UTC(),
			ProcessedAt:     &processedAt,
		})
		if err != nil {
			return err
		}
		if !stored {
			duplicate = true
			updated, err = repos.Conversions.GetConversionByID(txCtx, event.ConversionID)
			return err
		}

		current, err := repos.Conversions.GetConversionByID(txCtx, event.ConversionID)
		if err != nil {
			return err
		}

		switch event.Topic {
		case "payment.succeeded":
			reference := ""
			if event.ExternalReference != nil {
				reference = *event.ExternalReference
			}
			updated, err = current.AdvanceForPayment(reference, now)
			if err != nil {
				return err
			}
		case "payment.failed":
			reference := ""
			if event.ExternalReference != nil {
				reference = *event.ExternalReference
			}
			updated = current.FailPayment("payment.failed", reference, now)
		default:
			return domain.Validation("Unsupported payment webhook topic", map[string]any{
				"topic": event.Topic,
			})
		}

		if _, err := repos.Conversions.UpdateConversion(txCtx, updated); err != nil {
			return err
		}

		return repos.Outbox.Enqueue(txCtx, newOutboxEvent(
			"conversion",
			updated.ID,
			"conversion."+string(updated.Status),
			map[string]any{
				"conversionId": updated.ID,
				"status":       updated.Status,
				"provider":     event.Provider,
			},
			now,
		))
	})
	if err != nil {
		finish(err)
		return conversion.Conversion{}, false, err
	}

	if duplicate {
		u.telemetry.RecordWebhookDuplicate(ctx, event.Provider)
	} else {
		u.telemetry.RecordWebhookAccepted(ctx, event.Provider)
	}

	return updated, duplicate, nil
}

type HandleTransferWebhookUsecase struct {
	conversions port.ConversionRepository
	unitOfWork  port.UnitOfWork
	clock       port.Clock
	telemetry   port.Telemetry
}

func NewHandleTransferWebhookUsecase(
	conversions port.ConversionRepository,
	unitOfWork port.UnitOfWork,
	clock port.Clock,
	telemetry port.Telemetry,
) *HandleTransferWebhookUsecase {
	return &HandleTransferWebhookUsecase{
		conversions: conversions,
		unitOfWork:  unitOfWork,
		clock:       clock,
		telemetry:   telemetry,
	}
}

func (u *HandleTransferWebhookUsecase) Execute(ctx context.Context, event shared.ProviderEvent) (conversion.Conversion, bool, error) {
	ctx, finish := u.telemetry.StartSpan(ctx, "usecase.handle_transfer_webhook")
	defer func() { finish(nil) }()

	var updated conversion.Conversion
	var duplicate bool
	now := u.clock.Now()

	err := u.unitOfWork.WithinTransaction(ctx, func(txCtx context.Context, repos port.TransactionRepositories) error {
		processedAt := now.UTC()
		stored, err := repos.WebhookInbox.StoreIfAbsent(txCtx, webhook.InboxMessage{
			ID:              newULID(),
			Provider:        event.Provider,
			Topic:           event.Topic,
			ExternalEventID: event.ExternalEventID,
			ConversionID:    event.ConversionID,
			ExternalRef:     event.ExternalReference,
			Payload:         event.Payload,
			ReceivedAt:      now.UTC(),
			ProcessedAt:     &processedAt,
		})
		if err != nil {
			return err
		}
		if !stored {
			duplicate = true
			updated, err = repos.Conversions.GetConversionByID(txCtx, event.ConversionID)
			return err
		}

		current, err := repos.Conversions.GetConversionByID(txCtx, event.ConversionID)
		if err != nil {
			return err
		}

		switch event.Topic {
		case "transfer.completed":
			reference := ""
			if event.ExternalReference != nil {
				reference = *event.ExternalReference
			}
			updated, err = current.CompleteTransfer(reference, now)
			if err != nil {
				return err
			}
		case "transfer.failed":
			reference := ""
			if event.ExternalReference != nil {
				reference = *event.ExternalReference
			}
			updated = current.FailTransfer("transfer.failed", reference, now)
		default:
			return domain.Validation("Unsupported transfer webhook topic", map[string]any{
				"topic": event.Topic,
			})
		}

		if _, err := repos.Conversions.UpdateConversion(txCtx, updated); err != nil {
			return err
		}

		return repos.Outbox.Enqueue(txCtx, newOutboxEvent(
			"conversion",
			updated.ID,
			"conversion."+string(updated.Status),
			map[string]any{
				"conversionId": updated.ID,
				"status":       updated.Status,
				"provider":     event.Provider,
			},
			now,
		))
	})
	if err != nil {
		finish(err)
		return conversion.Conversion{}, false, err
	}

	if duplicate {
		u.telemetry.RecordWebhookDuplicate(ctx, event.Provider)
	} else {
		u.telemetry.RecordWebhookAccepted(ctx, event.Provider)
	}

	return updated, duplicate, nil
}
