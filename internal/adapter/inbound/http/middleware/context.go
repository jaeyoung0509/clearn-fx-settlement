package middleware

import (
	"context"

	"go.uber.org/zap"
)

type contextKey string

const (
	requestIDKey contextKey = "request_id"
	loggerKey    contextKey = "logger"
)

func withRequestContext(ctx context.Context, requestID string, logger *zap.Logger) context.Context {
	ctx = context.WithValue(ctx, requestIDKey, requestID)
	ctx = context.WithValue(ctx, loggerKey, logger)
	return ctx
}

func RequestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDKey).(string)
	return requestID
}

func LoggerFromContext(ctx context.Context) *zap.Logger {
	logger, _ := ctx.Value(loggerKey).(*zap.Logger)
	return logger
}
