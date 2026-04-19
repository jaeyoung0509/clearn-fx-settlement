package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"fx-settlement-lab/go-backend/internal/domain"
)

func TestErrorHandlerMapsDomainErrors(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	testCases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "validation",
			err:        domain.Validation("Request validation failed", nil),
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   string(domain.ErrorCodeValidation),
		},
		{
			name:       "not found",
			err:        domain.NotFound("Item not found", nil),
			wantStatus: http.StatusNotFound,
			wantCode:   string(domain.ErrorCodeNotFound),
		},
		{
			name:       "internal",
			err:        domain.Internal("Unexpected server error", nil),
			wantStatus: http.StatusInternalServerError,
			wantCode:   string(domain.ErrorCodeInternal),
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			engine := gin.New()
			engine.Use(RequestContext(zap.NewNop()), ErrorHandler())
			engine.GET("/", func(c *gin.Context) {
				_ = c.Error(testCase.err)
			})

			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Header.Set("X-Request-ID", "test-request-id")
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)

			if recorder.Code != testCase.wantStatus {
				t.Fatalf("unexpected status: got %d want %d", recorder.Code, testCase.wantStatus)
			}

			var payload map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode response: %v", err)
			}

			errorBody := payload["error"].(map[string]any)
			if errorBody["code"] != testCase.wantCode {
				t.Fatalf("unexpected error code: %v", errorBody["code"])
			}
			if errorBody["requestId"] != "test-request-id" {
				t.Fatalf("unexpected requestId: %v", errorBody["requestId"])
			}
		})
	}
}
