package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"fx-settlement-lab/go-backend/internal/domain"
)

type errorBody struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	RequestID *string        `json:"requestId,omitempty"`
}

type errorEnvelope struct {
	EventTime time.Time `json:"eventTime"`
	Error     errorBody `json:"error"`
}

func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) == 0 || c.Writer.Written() {
			return
		}

		appErr := domain.AsAppError(c.Errors.Last().Err)
		requestID := RequestIDFromContext(c.Request.Context())
		statusCode := httpStatusFor(appErr.Code)

		var requestIDPointer *string
		if requestID != "" {
			requestIDPointer = &requestID
		}

		c.AbortWithStatusJSON(statusCode, errorEnvelope{
			EventTime: time.Now().UTC(),
			Error: errorBody{
				Code:      string(appErr.Code),
				Message:   appErr.Message,
				Details:   appErr.Details,
				RequestID: requestIDPointer,
			},
		})
	}
}

func httpStatusFor(code domain.ErrorCode) int {
	switch code {
	case domain.ErrorCodeNotFound:
		return http.StatusNotFound
	case domain.ErrorCodeValidation:
		return http.StatusUnprocessableEntity
	case domain.ErrorCodeConflict:
		return http.StatusConflict
	case domain.ErrorCodeUnauthorized:
		return http.StatusUnauthorized
	default:
		return http.StatusInternalServerError
	}
}
