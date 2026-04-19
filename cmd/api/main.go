package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fx-settlement-lab/go-backend/internal/app"
	"fx-settlement-lab/go-backend/internal/config"
	"go.uber.org/zap"
)

func main() {
	if err := run(); err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	runtime, err := app.Bootstrap(context.Background(), cfg)
	if err != nil {
		return err
	}
	defer func() { _ = runtime.Close() }()

	server := &http.Server{
		Addr:              cfg.HTTPAddress(),
		Handler:           runtime.Router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		runtime.Logger.Info("starting http server", zap.String("addr", cfg.HTTPAddress()))
		serverErrors <- server.ListenAndServe()
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-signals:
		runtime.Logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), runtime.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}
