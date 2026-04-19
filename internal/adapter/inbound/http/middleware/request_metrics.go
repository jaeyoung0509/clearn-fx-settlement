package middleware

import (
	"time"

	"github.com/gin-gonic/gin"

	"fx-settlement-lab/go-backend/internal/port"
)

func RequestMetrics(telemetry port.Telemetry) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()

		if telemetry == nil {
			return
		}

		route := c.FullPath()
		if route == "" {
			route = c.Request.URL.Path
		}

		telemetry.RecordInboundRequest(
			c.Request.Context(),
			"http",
			c.Request.Method+" "+route,
			classifyHTTPOutcome(c.Writer.Status()),
			time.Since(started),
		)
	}
}

func classifyHTTPOutcome(status int) string {
	switch {
	case status >= 500:
		return "server_error"
	case status >= 400:
		return "client_error"
	default:
		return "success"
	}
}
