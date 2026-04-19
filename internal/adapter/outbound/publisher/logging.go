package publisher

import (
	"context"

	"go.uber.org/zap"

	"fx-settlement-lab/go-backend/internal/domain/outbox"
	"fx-settlement-lab/go-backend/internal/port"
)

type LoggingPublisher struct {
	logger *zap.Logger
}

var _ port.EventPublisher = (*LoggingPublisher)(nil)

func NewLoggingPublisher(logger *zap.Logger) *LoggingPublisher {
	return &LoggingPublisher{logger: logger}
}

func (p *LoggingPublisher) Publish(_ context.Context, event outbox.Event) error {
	p.logger.Info("publishing outbox event",
		zap.String("eventId", event.ID),
		zap.String("aggregateType", event.AggregateType),
		zap.String("aggregateId", event.AggregateID),
		zap.String("eventType", event.EventType),
	)

	return nil
}
