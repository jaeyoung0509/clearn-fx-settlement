package observability

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPrometheusStackExposesInboundMetrics(t *testing.T) {
	stack, err := NewPrometheusStack()
	if err != nil {
		t.Fatalf("new prometheus stack: %v", err)
	}

	stack.Telemetry.RecordInboundRequest(context.Background(), "http", "GET /health", "success", 25*time.Millisecond)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/metrics", nil)
	stack.Handler.ServeHTTP(recorder, request)

	body := recorder.Body.String()
	if !strings.Contains(body, "fx_inbound_requests_total") {
		t.Fatalf("expected inbound request counter in metrics output: %s", body)
	}
	if !strings.Contains(body, "operation=\"GET /health\"") || !strings.Contains(body, "transport=\"http\"") {
		t.Fatalf("expected request labels in metrics output: %s", body)
	}
	if !strings.Contains(body, "fx_inbound_request_duration_seconds") {
		t.Fatalf("expected inbound request histogram in metrics output: %s", body)
	}
}
