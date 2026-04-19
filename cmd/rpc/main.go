package main

import (
	"context"
	"errors"
	"net"
	"os"
	"os/signal"
	"syscall"

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

	serverErrors := make(chan error, 1)
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
		if err := <-serverErrors; err != nil {
			return err
		}
	}

	return nil
}
