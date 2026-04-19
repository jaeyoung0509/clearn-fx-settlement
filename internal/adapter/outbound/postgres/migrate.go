package postgres

import (
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
)

func RunGoose(db *sql.DB, migrationsDir string, command string) error {
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}

	switch command {
	case "up":
		return goose.Up(db, migrationsDir)
	case "down":
		return goose.Down(db, migrationsDir)
	case "status":
		return goose.Status(db, migrationsDir)
	default:
		return fmt.Errorf("unsupported migration command: %s", command)
	}
}
