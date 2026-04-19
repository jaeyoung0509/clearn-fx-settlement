package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()

		logger := LoggerFromContext(c.Request.Context())
		if logger == nil {
			return
		}

		logger.Info("request completed",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("statusCode", c.Writer.Status()),
			zap.Duration("latency", time.Since(started)),
			zap.String("clientIP", c.ClientIP()),
		)
	}
}
