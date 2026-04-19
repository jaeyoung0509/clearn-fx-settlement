package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const requestIDHeader = "X-Request-ID"

func RequestContext(baseLogger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader(requestIDHeader)
		if requestID == "" {
			requestID = uuid.NewString()
		}

		requestLogger := baseLogger.With(zap.String("requestId", requestID))
		c.Request = c.Request.WithContext(withRequestContext(c.Request.Context(), requestID, requestLogger))
		c.Header(requestIDHeader, requestID)
		c.Next()

		if !c.Writer.Written() {
			c.Status(http.StatusOK)
		}
	}
}
