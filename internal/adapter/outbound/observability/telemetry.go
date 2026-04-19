package observability

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"fx-settlement-lab/go-backend/internal/port"
)

type PrometheusStack struct {
	Telemetry *Telemetry
	Handler   http.Handler
}

type Telemetry struct {
	tracer                 trace.Tracer
	inboundRequestCount    *prometheus.CounterVec
	inboundRequestDuration *prometheus.HistogramVec
	webhookDuplicateCount  *prometheus.CounterVec
	webhookAcceptedCount   *prometheus.CounterVec
	outboxPublishedCount   *prometheus.CounterVec
	outboxPublishFailCount *prometheus.CounterVec
}

var _ port.Telemetry = (*Telemetry)(nil)

func NewTelemetry() *Telemetry {
	telemetry, err := newTelemetry(prometheus.NewRegistry())
	if err != nil {
		panic(err)
	}

	return telemetry
}

func NewPrometheusStack() (*PrometheusStack, error) {
	registry := prometheus.NewRegistry()
	if err := registerCollectors(
		registry,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	); err != nil {
		return nil, err
	}

	telemetry, err := newTelemetry(registry)
	if err != nil {
		return nil, err
	}

	return &PrometheusStack{
		Telemetry: telemetry,
		Handler:   promhttp.HandlerFor(registry, promhttp.HandlerOpts{EnableOpenMetrics: true}),
	}, nil
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

func (t *Telemetry) RecordInboundRequest(_ context.Context, transport string, operation string, outcome string, duration time.Duration) {
	t.inboundRequestCount.WithLabelValues(transport, operation, outcome).Inc()
	t.inboundRequestDuration.WithLabelValues(transport, operation, outcome).Observe(duration.Seconds())
}

func (t *Telemetry) RecordWebhookDuplicate(_ context.Context, provider string) {
	t.webhookDuplicateCount.WithLabelValues(provider).Inc()
}

func (t *Telemetry) RecordWebhookAccepted(_ context.Context, provider string) {
	t.webhookAcceptedCount.WithLabelValues(provider).Inc()
}

func (t *Telemetry) RecordOutboxPublished(_ context.Context, eventType string) {
	t.outboxPublishedCount.WithLabelValues(eventType).Inc()
}

func (t *Telemetry) RecordOutboxPublishFailure(_ context.Context, eventType string) {
	t.outboxPublishFailCount.WithLabelValues(eventType).Inc()
}

func newTelemetry(registerer prometheus.Registerer) (*Telemetry, error) {
	telemetry := &Telemetry{
		tracer: otel.Tracer("fx-settlement-lab/go-backend/fx",
			trace.WithInstrumentationAttributes(attribute.String("component", "observability")),
		),
		inboundRequestCount: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fx_inbound_requests_total",
			Help: "Total inbound requests handled by transport, operation, and outcome.",
		}, []string{"transport", "operation", "outcome"}),
		inboundRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "fx_inbound_request_duration_seconds",
			Help:    "Inbound request duration in seconds by transport, operation, and outcome.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		}, []string{"transport", "operation", "outcome"}),
		webhookDuplicateCount: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fx_webhook_duplicate_total",
			Help: "Total number of duplicate webhook deliveries ignored.",
		}, []string{"provider"}),
		webhookAcceptedCount: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fx_webhook_accepted_total",
			Help: "Total number of webhook deliveries accepted.",
		}, []string{"provider"}),
		outboxPublishedCount: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fx_outbox_published_total",
			Help: "Total number of outbox events published.",
		}, []string{"event_type"}),
		outboxPublishFailCount: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fx_outbox_publish_failed_total",
			Help: "Total number of failed outbox publish attempts.",
		}, []string{"event_type"}),
	}

	if err := registerCollectors(
		registerer,
		telemetry.inboundRequestCount,
		telemetry.inboundRequestDuration,
		telemetry.webhookDuplicateCount,
		telemetry.webhookAcceptedCount,
		telemetry.outboxPublishedCount,
		telemetry.outboxPublishFailCount,
	); err != nil {
		return nil, err
	}

	return telemetry, nil
}

func registerCollectors(registerer prometheus.Registerer, collectors ...prometheus.Collector) error {
	for _, collector := range collectors {
		if err := registerer.Register(collector); err != nil {
			return err
		}
	}

	return nil
}
