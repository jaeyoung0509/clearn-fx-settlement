package observability

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"fx-settlement-lab/go-backend/internal/port"
)

type Telemetry struct {
	tracer                 trace.Tracer
	webhookDuplicateCount  metric.Int64Counter
	webhookAcceptedCount   metric.Int64Counter
	outboxPublishedCount   metric.Int64Counter
	outboxPublishFailCount metric.Int64Counter
}

var _ port.Telemetry = (*Telemetry)(nil)

func NewTelemetry() *Telemetry {
	meter := otel.Meter("fx-settlement-lab/go-backend/fx")
	webhookDuplicateCount, _ := meter.Int64Counter("fx_webhook_duplicate_total")
	webhookAcceptedCount, _ := meter.Int64Counter("fx_webhook_accepted_total")
	outboxPublishedCount, _ := meter.Int64Counter("fx_outbox_published_total")
	outboxPublishFailCount, _ := meter.Int64Counter("fx_outbox_publish_failed_total")

	return &Telemetry{
		tracer:                 otel.Tracer("fx-settlement-lab/go-backend/fx"),
		webhookDuplicateCount:  webhookDuplicateCount,
		webhookAcceptedCount:   webhookAcceptedCount,
		outboxPublishedCount:   outboxPublishedCount,
		outboxPublishFailCount: outboxPublishFailCount,
	}
}

func (t *Telemetry) StartSpan(ctx context.Context, name string) (context.Context, func(err error)) {
	ctx, span := t.tracer.Start(ctx, name)

	return ctx, func(err error) {
		if err != nil {
			span.RecordError(err)
		}
		span.End()
	}
}

func (t *Telemetry) RecordWebhookDuplicate(ctx context.Context, provider string) {
	t.webhookDuplicateCount.Add(ctx, 1, metric.WithAttributes(attribute.String("provider", provider)))
}

func (t *Telemetry) RecordWebhookAccepted(ctx context.Context, provider string) {
	t.webhookAcceptedCount.Add(ctx, 1, metric.WithAttributes(attribute.String("provider", provider)))
}

func (t *Telemetry) RecordOutboxPublished(ctx context.Context, eventType string) {
	t.outboxPublishedCount.Add(ctx, 1, metric.WithAttributes(attribute.String("event_type", eventType)))
}

func (t *Telemetry) RecordOutboxPublishFailure(ctx context.Context, eventType string) {
	t.outboxPublishFailCount.Add(ctx, 1, metric.WithAttributes(attribute.String("event_type", eventType)))
}
