package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const defaultEnvFilePath = ".env"

type Config struct {
	HTTPPort              string        `env:"HTTP_PORT" env-default:"8000" env-description:"HTTP listen port"`
	GRPCPort              string        `env:"GRPC_PORT" env-default:"9000" env-description:"gRPC listen port"`
	RPCPort               string        `env:"RPC_PORT" env-default:"9100" env-description:"RPC listen port"`
	HTTPMetricsPort       string        `env:"HTTP_METRICS_PORT" env-default:"9101" env-description:"HTTP metrics listen port"`
	GRPCMetricsPort       string        `env:"GRPC_METRICS_PORT" env-default:"9102" env-description:"gRPC metrics listen port"`
	RPCMetricsPort        string        `env:"RPC_METRICS_PORT" env-default:"9103" env-description:"RPC metrics listen port"`
	DatabaseURL           string        `env:"DATABASE_URL" env-default:"postgres://shopping:shopping@127.0.0.1:5432/fx_settlement?sslmode=disable" env-description:"PostgreSQL DSN"`
	LogLevel              string        `env:"LOG_LEVEL" env-default:"info" env-description:"Zap log level"`
	CORSAllowedOrigins    []string      `env:"CORS_ALLOWED_ORIGINS" env-default:"http://localhost:5173,http://127.0.0.1:5173" env-separator:"," env-description:"Allowed CORS origins"`
	PGXMaxConns           int32         `env:"PGX_MAX_CONNS" env-default:"10" env-description:"Maximum pgx pool connections"`
	PGXMinConns           int32         `env:"PGX_MIN_CONNS" env-default:"2" env-description:"Minimum pgx pool connections"`
	ShutdownTimeout       time.Duration `env:"SHUTDOWN_TIMEOUT" env-default:"10s" env-description:"Graceful shutdown timeout"`
	FrankfurterBaseURL    string        `env:"FRANKFURTER_BASE_URL" env-default:"https://api.frankfurter.dev" env-description:"Frankfurter API base URL"`
	FrankfurterSource     string        `env:"FRANKFURTER_PROVIDER" env-default:"ECB" env-description:"Frankfurter provider filter"`
	HTTPClientTimeout     time.Duration `env:"HTTP_CLIENT_TIMEOUT" env-default:"5s" env-description:"Shared outbound HTTP timeout"`
	QuoteTTL              time.Duration `env:"QUOTE_TTL" env-default:"15m" env-description:"Quote expiry duration"`
	FXFeeBPS              int64         `env:"FX_FEE_BPS" env-default:"50" env-description:"FX fee basis points"`
	FXMinFeeMinor         int64         `env:"FX_MIN_FEE_MINOR" env-default:"500" env-description:"Minimum FX fee in KRW minor units"`
	PaymentWebhookSecret  string        `env:"PAYMENT_WEBHOOK_SECRET" env-default:"payment-dev-secret" env-description:"Payment webhook HMAC secret"`
	TransferWebhookSecret string        `env:"TRANSFER_WEBHOOK_SECRET" env-default:"transfer-dev-secret" env-description:"Transfer webhook HMAC secret"`
	OutboxPublishBatch    int           `env:"OUTBOX_PUBLISH_BATCH" env-default:"100" env-description:"Outbox publish batch size"`
}

func Load() (Config, error) {
	v := viper.New()
	applyDefaults(v)

	if _, err := os.Stat(defaultEnvFilePath); err == nil {
		v.SetConfigFile(defaultEnvFilePath)
		v.SetConfigType("env")
		if err := v.ReadInConfig(); err != nil {
			return Config{}, fmt.Errorf("read config from %s: %w", defaultEnvFilePath, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("stat %s: %w", defaultEnvFilePath, err)
	}

	v.AutomaticEnv()
	bindEnv(v)

	cfg := Config{
		HTTPPort:              v.GetString("HTTP_PORT"),
		GRPCPort:              v.GetString("GRPC_PORT"),
		RPCPort:               v.GetString("RPC_PORT"),
		HTTPMetricsPort:       v.GetString("HTTP_METRICS_PORT"),
		GRPCMetricsPort:       v.GetString("GRPC_METRICS_PORT"),
		RPCMetricsPort:        v.GetString("RPC_METRICS_PORT"),
		DatabaseURL:           v.GetString("DATABASE_URL"),
		LogLevel:              v.GetString("LOG_LEVEL"),
		CORSAllowedOrigins:    csvSlice(v.GetString("CORS_ALLOWED_ORIGINS")),
		PGXMaxConns:           int32(v.GetInt("PGX_MAX_CONNS")),
		PGXMinConns:           int32(v.GetInt("PGX_MIN_CONNS")),
		ShutdownTimeout:       v.GetDuration("SHUTDOWN_TIMEOUT"),
		FrankfurterBaseURL:    v.GetString("FRANKFURTER_BASE_URL"),
		FrankfurterSource:     v.GetString("FRANKFURTER_PROVIDER"),
		HTTPClientTimeout:     v.GetDuration("HTTP_CLIENT_TIMEOUT"),
		QuoteTTL:              v.GetDuration("QUOTE_TTL"),
		FXFeeBPS:              v.GetInt64("FX_FEE_BPS"),
		FXMinFeeMinor:         v.GetInt64("FX_MIN_FEE_MINOR"),
		PaymentWebhookSecret:  v.GetString("PAYMENT_WEBHOOK_SECRET"),
		TransferWebhookSecret: v.GetString("TRANSFER_WEBHOOK_SECRET"),
		OutboxPublishBatch:    v.GetInt("OUTBOX_PUBLISH_BATCH"),
	}

	if cfg.PGXMinConns < 0 || cfg.PGXMaxConns < 1 || cfg.PGXMinConns > cfg.PGXMaxConns {
		return Config{}, fmt.Errorf(
			"invalid pgx connection settings: min=%d max=%d",
			cfg.PGXMinConns,
			cfg.PGXMaxConns,
		)
	}
	if cfg.QuoteTTL <= 0 {
		return Config{}, fmt.Errorf("invalid quote ttl: %s", cfg.QuoteTTL)
	}
	if cfg.HTTPClientTimeout <= 0 {
		return Config{}, fmt.Errorf("invalid http client timeout: %s", cfg.HTTPClientTimeout)
	}
	if cfg.FXFeeBPS < 0 || cfg.FXMinFeeMinor < 0 {
		return Config{}, fmt.Errorf("invalid fee configuration: bps=%d min_minor=%d", cfg.FXFeeBPS, cfg.FXMinFeeMinor)
	}
	if cfg.OutboxPublishBatch < 1 {
		return Config{}, fmt.Errorf("invalid outbox publish batch: %d", cfg.OutboxPublishBatch)
	}

	return cfg, nil
}

func (c Config) HTTPAddress() string {
	return ":" + c.HTTPPort
}

func (c Config) GRPCAddress() string {
	return ":" + c.GRPCPort
}

func (c Config) RPCAddress() string {
	return ":" + c.RPCPort
}

func (c Config) HTTPMetricsAddress() string {
	return ":" + c.HTTPMetricsPort
}

func (c Config) GRPCMetricsAddress() string {
	return ":" + c.GRPCMetricsPort
}

func (c Config) RPCMetricsAddress() string {
	return ":" + c.RPCMetricsPort
}

func applyDefaults(v *viper.Viper) {
	v.SetDefault("HTTP_PORT", "8000")
	v.SetDefault("GRPC_PORT", "9000")
	v.SetDefault("RPC_PORT", "9100")
	v.SetDefault("HTTP_METRICS_PORT", "9101")
	v.SetDefault("GRPC_METRICS_PORT", "9102")
	v.SetDefault("RPC_METRICS_PORT", "9103")
	v.SetDefault("DATABASE_URL", "postgres://shopping:shopping@127.0.0.1:5432/fx_settlement?sslmode=disable")
	v.SetDefault("LOG_LEVEL", "info")
	v.SetDefault("CORS_ALLOWED_ORIGINS", "http://localhost:5173,http://127.0.0.1:5173")
	v.SetDefault("PGX_MAX_CONNS", 10)
	v.SetDefault("PGX_MIN_CONNS", 2)
	v.SetDefault("SHUTDOWN_TIMEOUT", "10s")
	v.SetDefault("FRANKFURTER_BASE_URL", "https://api.frankfurter.dev")
	v.SetDefault("FRANKFURTER_PROVIDER", "ECB")
	v.SetDefault("HTTP_CLIENT_TIMEOUT", "5s")
	v.SetDefault("QUOTE_TTL", "15m")
	v.SetDefault("FX_FEE_BPS", 50)
	v.SetDefault("FX_MIN_FEE_MINOR", 500)
	v.SetDefault("PAYMENT_WEBHOOK_SECRET", "payment-dev-secret")
	v.SetDefault("TRANSFER_WEBHOOK_SECRET", "transfer-dev-secret")
	v.SetDefault("OUTBOX_PUBLISH_BATCH", 100)
}

func bindEnv(v *viper.Viper) {
	keys := []string{
		"HTTP_PORT",
		"GRPC_PORT",
		"RPC_PORT",
		"HTTP_METRICS_PORT",
		"GRPC_METRICS_PORT",
		"RPC_METRICS_PORT",
		"DATABASE_URL",
		"LOG_LEVEL",
		"CORS_ALLOWED_ORIGINS",
		"PGX_MAX_CONNS",
		"PGX_MIN_CONNS",
		"SHUTDOWN_TIMEOUT",
		"FRANKFURTER_BASE_URL",
		"FRANKFURTER_PROVIDER",
		"HTTP_CLIENT_TIMEOUT",
		"QUOTE_TTL",
		"FX_FEE_BPS",
		"FX_MIN_FEE_MINOR",
		"PAYMENT_WEBHOOK_SECRET",
		"TRANSFER_WEBHOOK_SECRET",
		"OUTBOX_PUBLISH_BATCH",
	}

	for _, key := range keys {
		if err := v.BindEnv(key); err != nil {
			panic(fmt.Sprintf("bind env %s: %v", key, err))
		}
	}
}

func csvSlice(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		values = append(values, trimmed)
	}

	return values
}
