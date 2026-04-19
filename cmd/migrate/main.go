package main

import (
	"fmt"
	"os"

	platformpostgres "fx-settlement-lab/go-backend/internal/adapter/outbound/postgres"
	"fx-settlement-lab/go-backend/internal/config"
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

	command := "up"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	db, err := platformpostgres.OpenSQLDB(cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	if err := platformpostgres.RunGoose(db, "migrations", command); err != nil {
		return fmt.Errorf("run migration command %q: %w", command, err)
	}

	return nil
}
