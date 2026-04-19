package app

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/multierr"
	"go.uber.org/zap"

	httpadapter "fx-settlement-lab/go-backend/internal/adapter/inbound/http"
	"fx-settlement-lab/go-backend/internal/adapter/outbound/frankfurter"
	"fx-settlement-lab/go-backend/internal/adapter/outbound/logger"
	"fx-settlement-lab/go-backend/internal/adapter/outbound/observability"
	platformpostgres "fx-settlement-lab/go-backend/internal/adapter/outbound/postgres"
	"fx-settlement-lab/go-backend/internal/adapter/outbound/publisher"
	"fx-settlement-lab/go-backend/internal/adapter/outbound/webhooksigning"
	"fx-settlement-lab/go-backend/internal/config"
	"fx-settlement-lab/go-backend/internal/domain/shared"
	"fx-settlement-lab/go-backend/internal/port"
	"fx-settlement-lab/go-backend/internal/usecase"
)

type Runtime struct {
	Logger          *zap.Logger
	Router          http.Handler
	Pool            *pgxpool.Pool
	SQLDB           *sql.DB
	ShutdownTimeout time.Duration
}

type providerSet struct {
	RateProvider            port.RateProvider
	EventPublisher          port.EventPublisher
	PaymentWebhookVerifier  *webhooksigning.HMACVerifier
	TransferWebhookVerifier *webhooksigning.HMACVerifier
}

type usecaseSet struct {
	GetReferenceRates     *usecase.GetReferenceRatesUsecase
	CreateQuote           *usecase.CreateQuoteUsecase
	AcceptQuote           *usecase.AcceptQuoteUsecase
	GetConversion         *usecase.GetConversionUsecase
	HandlePaymentWebhook  *usecase.HandlePaymentWebhookUsecase
	HandleTransferWebhook *usecase.HandleTransferWebhookUsecase
}

func Bootstrap(ctx context.Context, cfg config.Config) (*Runtime, error) {
	appLogger, err := logger.New(cfg.LogLevel)
	if err != nil {
		return nil, err
	}

	pool, err := platformpostgres.NewPGXPool(ctx, cfg)
	if err != nil {
		_ = appLogger.Sync()
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		_ = appLogger.Sync()
		return nil, err
	}

	db, err := platformpostgres.NewGormDB(cfg)
	if err != nil {
		pool.Close()
		_ = appLogger.Sync()
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		pool.Close()
		_ = appLogger.Sync()
		return nil, err
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		pool.Close()
		_ = appLogger.Sync()
		return nil, err
	}

	store := platformpostgres.NewStore(db, appLogger)
	telemetry := observability.NewTelemetry()
	clock := systemClock{}
	providers := newProviders(cfg, appLogger)
	usecases := newUsecases(cfg, store, providers, telemetry, clock)

	router := httpadapter.NewRouter(httpadapter.RouterDeps{
		Logger:                  appLogger,
		GetReferenceRates:       usecases.GetReferenceRates,
		CreateQuote:             usecases.CreateQuote,
		AcceptQuote:             usecases.AcceptQuote,
		GetConversion:           usecases.GetConversion,
		HandlePaymentWebhook:    usecases.HandlePaymentWebhook,
		HandleTransferWebhook:   usecases.HandleTransferWebhook,
		PaymentWebhookVerifier:  providers.PaymentWebhookVerifier,
		TransferWebhookVerifier: providers.TransferWebhookVerifier,
		ReadyChecker:            pool,
		CORSAllowedOrigins:      cfg.CORSAllowedOrigins,
	})

	return &Runtime{
		Logger:          appLogger,
		Router:          router,
		Pool:            pool,
		SQLDB:           sqlDB,
		ShutdownTimeout: cfg.ShutdownTimeout,
	}, nil
}

func (r *Runtime) Close() error {
	var err error
	if r.SQLDB != nil {
		err = multierr.Append(err, r.SQLDB.Close())
	}
	if r.Pool != nil {
		r.Pool.Close()
	}
	if r.Logger != nil {
		err = multierr.Append(err, r.Logger.Sync())
	}

	return err
}

func newProviders(cfg config.Config, appLogger *zap.Logger) providerSet {
	return providerSet{
		RateProvider: frankfurter.NewClient(cfg.FrankfurterBaseURL, cfg.FrankfurterSource, &http.Client{
			Timeout: cfg.HTTPClientTimeout,
		}),
		EventPublisher:          publisher.NewLoggingPublisher(appLogger),
		PaymentWebhookVerifier:  webhooksigning.NewHMACVerifier(cfg.PaymentWebhookSecret),
		TransferWebhookVerifier: webhooksigning.NewHMACVerifier(cfg.TransferWebhookSecret),
	}
}

func newUsecases(
	cfg config.Config,
	store *platformpostgres.Store,
	providers providerSet,
	telemetry port.Telemetry,
	clock port.Clock,
) usecaseSet {
	minFee := shared.MustMoney(shared.CurrencyKRW, cfg.FXMinFeeMinor)

	return usecaseSet{
		GetReferenceRates:     usecase.NewGetReferenceRatesUsecase(store, providers.RateProvider, telemetry),
		CreateQuote:           usecase.NewCreateQuoteUsecase(store, store, providers.RateProvider, clock, telemetry, cfg.QuoteTTL, cfg.FXFeeBPS, minFee),
		AcceptQuote:           usecase.NewAcceptQuoteUsecase(store, store, store, clock, telemetry),
		GetConversion:         usecase.NewGetConversionUsecase(store, telemetry),
		HandlePaymentWebhook:  usecase.NewHandlePaymentWebhookUsecase(store, store, clock, telemetry),
		HandleTransferWebhook: usecase.NewHandleTransferWebhookUsecase(store, store, clock, telemetry),
	}
}

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now().UTC()
}
