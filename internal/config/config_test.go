package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromDotEnv(t *testing.T) {
	t.Helper()

	tempDir := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalWD)
	})

	writeEnvFile(t, filepath.Join(tempDir, ".env"), []byte(
		"HTTP_PORT=9000\n"+
			"GRPC_PORT=9100\n"+
			"DATABASE_URL=postgres://example:example@localhost:5432/app?sslmode=disable\n"+
			"LOG_LEVEL=debug\n"+
			"CORS_ALLOWED_ORIGINS=http://localhost:5173,http://127.0.0.1:5173\n"+
			"PGX_MAX_CONNS=20\n"+
			"PGX_MIN_CONNS=5\n"+
			"SHUTDOWN_TIMEOUT=30s\n"+
			"QUOTE_TTL=20m\n"+
			"FX_FEE_BPS=75\n"+
			"FX_MIN_FEE_MINOR=700\n",
	))

	restoreEnv := snapshotEnv(
		"HTTP_PORT",
		"GRPC_PORT",
		"DATABASE_URL",
		"LOG_LEVEL",
		"CORS_ALLOWED_ORIGINS",
		"PGX_MAX_CONNS",
		"PGX_MIN_CONNS",
		"SHUTDOWN_TIMEOUT",
		"QUOTE_TTL",
		"FX_FEE_BPS",
		"FX_MIN_FEE_MINOR",
	)
	t.Cleanup(restoreEnv)

	_ = os.Unsetenv("HTTP_PORT")
	_ = os.Unsetenv("GRPC_PORT")
	_ = os.Unsetenv("DATABASE_URL")
	_ = os.Unsetenv("LOG_LEVEL")
	_ = os.Unsetenv("CORS_ALLOWED_ORIGINS")
	_ = os.Unsetenv("PGX_MAX_CONNS")
	_ = os.Unsetenv("PGX_MIN_CONNS")
	_ = os.Unsetenv("SHUTDOWN_TIMEOUT")
	_ = os.Unsetenv("QUOTE_TTL")
	_ = os.Unsetenv("FX_FEE_BPS")
	_ = os.Unsetenv("FX_MIN_FEE_MINOR")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.HTTPPort != "9000" {
		t.Fatalf("unexpected HTTP port: %s", cfg.HTTPPort)
	}
	if cfg.GRPCPort != "9100" {
		t.Fatalf("unexpected gRPC port: %s", cfg.GRPCPort)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("unexpected log level: %s", cfg.LogLevel)
	}
	if cfg.PGXMaxConns != 20 || cfg.PGXMinConns != 5 {
		t.Fatalf("unexpected pgx config: min=%d max=%d", cfg.PGXMinConns, cfg.PGXMaxConns)
	}
	if cfg.ShutdownTimeout.String() != "30s" {
		t.Fatalf("unexpected shutdown timeout: %s", cfg.ShutdownTimeout)
	}
	if cfg.QuoteTTL.String() != "20m0s" {
		t.Fatalf("unexpected quote ttl: %s", cfg.QuoteTTL)
	}
	if cfg.FXFeeBPS != 75 || cfg.FXMinFeeMinor != 700 {
		t.Fatalf("unexpected fx fee config: bps=%d min=%d", cfg.FXFeeBPS, cfg.FXMinFeeMinor)
	}
}

func TestLoadFromEnvironmentOnly(t *testing.T) {
	t.Helper()

	tempDir := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalWD)
	})

	t.Setenv("HTTP_PORT", "7000")
	t.Setenv("GRPC_PORT", "7100")
	t.Setenv("PGX_MAX_CONNS", "12")
	t.Setenv("PGX_MIN_CONNS", "3")
	t.Setenv("OUTBOX_PUBLISH_BATCH", "42")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.HTTPPort != "7000" {
		t.Fatalf("unexpected HTTP port: %s", cfg.HTTPPort)
	}
	if cfg.GRPCPort != "7100" {
		t.Fatalf("unexpected gRPC port: %s", cfg.GRPCPort)
	}
	if cfg.PGXMaxConns != 12 || cfg.PGXMinConns != 3 {
		t.Fatalf("unexpected pgx config: min=%d max=%d", cfg.PGXMinConns, cfg.PGXMaxConns)
	}
	if len(cfg.CORSAllowedOrigins) != 2 {
		t.Fatalf("unexpected cors origins: %#v", cfg.CORSAllowedOrigins)
	}
	if cfg.OutboxPublishBatch != 42 {
		t.Fatalf("unexpected outbox publish batch: %d", cfg.OutboxPublishBatch)
	}
}

func TestEnvironmentOverridesDotEnv(t *testing.T) {
	t.Helper()

	tempDir := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalWD)
	})

	writeEnvFile(t, filepath.Join(tempDir, ".env"), []byte(
		"HTTP_PORT=9000\n"+
			"GRPC_PORT=9100\n"+
			"PGX_MAX_CONNS=20\n"+
			"PGX_MIN_CONNS=5\n",
	))

	t.Setenv("HTTP_PORT", "9100")
	t.Setenv("GRPC_PORT", "9200")
	t.Setenv("PGX_MAX_CONNS", "25")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.HTTPPort != "9100" {
		t.Fatalf("expected env override for HTTP_PORT, got %s", cfg.HTTPPort)
	}
	if cfg.GRPCPort != "9200" {
		t.Fatalf("expected env override for GRPC_PORT, got %s", cfg.GRPCPort)
	}
	if cfg.PGXMaxConns != 25 {
		t.Fatalf("expected env override for PGX_MAX_CONNS, got %d", cfg.PGXMaxConns)
	}
	if cfg.PGXMinConns != 5 {
		t.Fatalf("expected dotenv value for PGX_MIN_CONNS, got %d", cfg.PGXMinConns)
	}
}

func writeEnvFile(t *testing.T, path string, content []byte) {
	t.Helper()

	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
}

func snapshotEnv(keys ...string) func() {
	type envValue struct {
		value   string
		present bool
	}

	snapshot := map[string]envValue{}
	for _, key := range keys {
		value, present := os.LookupEnv(key)
		snapshot[key] = envValue{
			value:   value,
			present: present,
		}
	}

	return func() {
		for key, state := range snapshot {
			if state.present {
				_ = os.Setenv(key, state.value)
				continue
			}
			_ = os.Unsetenv(key)
		}
	}
}
