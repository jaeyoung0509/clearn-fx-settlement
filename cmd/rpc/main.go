package main

import (
	"context"
	"errors"
	"net"
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

	listener, err := net.Listen("tcp", cfg.RPCAddress())
	if err != nil {
		return err
	}
	defer func() { _ = listener.Close() }()
	metricsServer := &http.Server{
		Addr:              cfg.RPCMetricsAddress(),
		Handler:           runtime.MetricsHandler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	serverErrors := make(chan error, 2)
	go func() {
		runtime.Logger.Info("starting rpc server", zap.String("addr", cfg.RPCAddress()))
		for {
			connection, acceptErr := listener.Accept()
			if acceptErr != nil {
				if errors.Is(acceptErr, net.ErrClosed) {
					serverErrors <- nil
					return
				}
				serverErrors <- acceptErr
				return
			}

			go runtime.RPCServer.ServeConn(connection)
		}
	}()
	go func() {
		runtime.Logger.Info("starting metrics server", zap.String("addr", cfg.RPCMetricsAddress()))
		serverErrors <- metricsServer.ListenAndServe()
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		if err != nil {
			return err
		}
	case <-signals:
		runtime.Logger.Info("shutdown signal received")
		_ = listener.Close()
		if err := <-serverErrors; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), runtime.ShutdownTimeout)
	defer cancel()

	if err := metricsServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}
