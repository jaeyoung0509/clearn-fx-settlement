package testutil

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"gorm.io/gorm"

	platformpostgres "fx-settlement-lab/go-backend/internal/adapter/outbound/postgres"
	"fx-settlement-lab/go-backend/internal/config"
)

const (
	testDatabaseName  = "fx_settlement_test"
	testDatabaseUser  = "shopping"
	testDatabasePass  = "shopping"
	testPostgresImage = "postgres:16-alpine"
	containerTimeout  = 2 * time.Minute
)

// PostgresInstance owns the integration-test database lifecycle.
//
// Design notes:
// - Start one container per top-level test and reuse it across subtests for speed.
// - Reset database state per subtest instead of starting a container per subtest.
// - Register cleanup immediately after each resource is created so teardown order is deterministic.
type PostgresInstance struct {
	Container *tcpostgres.PostgresContainer
	Config    config.Config
	DSN       string
	Gorm      *gorm.DB
	Pool      *pgxpool.Pool
}

func StartPostgres(t testing.TB) *PostgresInstance {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), containerTimeout)
	t.Cleanup(cancel)

	container, err := startPostgresContainer(ctx)
	if err != nil {
		if isDockerUnavailable(err) {
			t.Skipf("skip integration test: docker is unavailable: %v", err)
		}
		t.Fatalf("start postgres container: %v", err)
	}

	// Cleanup is registered first so later DB/pool cleanup runs before container termination.
	testcontainers.CleanupContainer(t, container)

	dsn, err := container.ConnectionString(context.Background(), "sslmode=disable")
	if err != nil {
		t.Fatalf("resolve postgres connection string: %v", err)
	}

	ApplyMigrations(t, dsn)

	cfg := config.Config{
		DatabaseURL:        dsn,
		CORSAllowedOrigins: []string{"http://localhost:5173", "http://127.0.0.1:5173"},
		PGXMaxConns:        10,
		PGXMinConns:        2,
	}

	pool, err := platformpostgres.NewPGXPool(context.Background(), cfg)
	if err != nil {
		t.Fatalf("create pgx pool: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping pgx pool: %v", err)
	}

	db, err := platformpostgres.NewGormDB(cfg)
	if err != nil {
		t.Fatalf("create gorm db: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("fetch sql db: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	if err := sqlDB.PingContext(context.Background()); err != nil {
		t.Fatalf("ping sql db: %v", err)
	}

	return &PostgresInstance{
		Container: container,
		Config:    cfg,
		DSN:       dsn,
		Gorm:      db,
		Pool:      pool,
	}
}

func startPostgresContainer(ctx context.Context) (container *tcpostgres.PostgresContainer, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("testcontainers panic: %v", recovered)
		}
	}()

	return tcpostgres.Run(
		ctx,
		testPostgresImage,
		tcpostgres.WithDatabase(testDatabaseName),
		tcpostgres.WithUsername(testDatabaseUser),
		tcpostgres.WithPassword(testDatabasePass),
		tcpostgres.BasicWaitStrategies(),
	)
}

func isDockerUnavailable(err error) bool {
	message := strings.ToLower(err.Error())
	patterns := []string{
		"docker not found",
		"rootless docker not found",
		"cannot connect to the docker daemon",
		"docker daemon",
		"docker host",
		"docker socket",
		"permission denied",
	}

	for _, pattern := range patterns {
		if strings.Contains(message, pattern) {
			return true
		}
	}

	return false
}

func (p *PostgresInstance) Reset(t testing.TB) {
	t.Helper()

	if err := p.Gorm.Exec(
		"TRUNCATE TABLE webhook_inbox, outbox_events, conversions, quotes, reference_rates RESTART IDENTITY CASCADE",
	).Error; err != nil {
		t.Fatalf("truncate fx tables: %v", err)
	}
}

func ApplyMigrations(t testing.TB, databaseURL string) {
	t.Helper()

	db, err := platformpostgres.OpenSQLDB(databaseURL)
	if err != nil {
		t.Fatalf("open sql db: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := platformpostgres.RunGoose(db, filepath.Join(ModuleRoot(), "migrations"), "up"); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
}

func ModuleRoot() string {
	_, fileName, _, ok := runtime.Caller(0)
	if !ok {
		panic("resolve module root")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(fileName), "..", ".."))
}
