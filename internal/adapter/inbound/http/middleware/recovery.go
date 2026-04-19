package middleware

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"fx-settlement-lab/go-backend/internal/domain"
)

func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger := LoggerFromContext(c.Request.Context())
				if logger != nil {
					logger.Error("panic recovered",
						zap.Any("panic", recovered),
						zap.Stack("stack"),
					)
				}

				_ = c.Error(domain.Internal("", nil).WithCause(fmt.Errorf("panic: %v", recovered)))
				c.Abort()
			}
		}()

		c.Next()
	}
}
