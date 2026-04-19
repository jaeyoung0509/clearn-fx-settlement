package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"fx-settlement-lab/go-backend/internal/domain"
	"fx-settlement-lab/go-backend/internal/domain/conversion"
	"fx-settlement-lab/go-backend/internal/domain/outbox"
	"fx-settlement-lab/go-backend/internal/domain/quote"
	"fx-settlement-lab/go-backend/internal/domain/shared"
	"fx-settlement-lab/go-backend/internal/domain/webhook"
	"fx-settlement-lab/go-backend/internal/port"
)

type Store struct {
	db     *gorm.DB
	logger *zap.Logger
}

var (
	_ port.ReferenceRateRepository = (*Store)(nil)
	_ port.QuoteRepository         = (*Store)(nil)
	_ port.ConversionRepository    = (*Store)(nil)
	_ port.WebhookInboxRepository  = (*Store)(nil)
	_ port.OutboxRepository        = (*Store)(nil)
	_ port.UnitOfWork              = (*Store)(nil)
)

type ReferenceRateModel struct {
	BaseCurrency  string    `gorm:"column:base_currency;type:char(3);not null"`
	QuoteCurrency string    `gorm:"column:quote_currency;type:char(3);not null"`
	Provider      string    `gorm:"column:provider;type:varchar(32);not null"`
	Rate          string    `gorm:"column:rate;type:numeric(20,10);not null"`
	ObservedAt    time.Time `gorm:"column:observed_at;not null"`
	FetchedAt     time.Time `gorm:"column:fetched_at;not null"`
	UpdatedAt     time.Time `gorm:"column:updated_at;not null"`
}

func (ReferenceRateModel) TableName() string { return "reference_rates" }

type QuoteModel struct {
	ID              string     `gorm:"column:id;type:char(26);primaryKey"`
	IdempotencyKey  string     `gorm:"column:idempotency_key;type:varchar(128);uniqueIndex;not null"`
	BaseCurrency    string     `gorm:"column:base_currency;type:char(3);not null"`
	BaseMinorUnits  int64      `gorm:"column:base_minor_units;not null"`
	QuoteCurrency   string     `gorm:"column:quote_currency;type:char(3);not null"`
	QuoteMinorUnits int64      `gorm:"column:quote_minor_units;not null"`
	FeeMinorUnits   int64      `gorm:"column:fee_minor_units;not null"`
	TotalDebitMinor int64      `gorm:"column:total_debit_minor_units;not null"`
	Rate            string     `gorm:"column:rate;type:numeric(20,10);not null"`
	RateProvider    string     `gorm:"column:rate_provider;type:varchar(32);not null"`
	ExpiresAt       time.Time  `gorm:"column:expires_at;not null"`
	AcceptedAt      *time.Time `gorm:"column:accepted_at"`
	CreatedAt       time.Time  `gorm:"column:created_at;not null"`
}

func (QuoteModel) TableName() string { return "quotes" }

type ConversionModel struct {
	ID                 string    `gorm:"column:id;type:char(26);primaryKey"`
	QuoteID            string    `gorm:"column:quote_id;type:char(26);uniqueIndex;not null"`
	IdempotencyKey     string    `gorm:"column:idempotency_key;type:varchar(128);uniqueIndex;not null"`
	BaseCurrency       string    `gorm:"column:base_currency;type:char(3);not null"`
	BaseMinorUnits     int64     `gorm:"column:base_minor_units;not null"`
	QuoteCurrency      string    `gorm:"column:quote_currency;type:char(3);not null"`
	QuoteMinorUnits    int64     `gorm:"column:quote_minor_units;not null"`
	FeeMinorUnits      int64     `gorm:"column:fee_minor_units;not null"`
	TotalDebitMinor    int64     `gorm:"column:total_debit_minor_units;not null"`
	Rate               string    `gorm:"column:rate;type:numeric(20,10);not null"`
	RateProvider       string    `gorm:"column:rate_provider;type:varchar(32);not null"`
	PaymentProvider    string    `gorm:"column:payment_provider;type:varchar(32);not null"`
	TransferProvider   string    `gorm:"column:transfer_provider;type:varchar(32);not null"`
	Status             string    `gorm:"column:status;type:varchar(32);not null"`
	ExternalPaymentID  *string   `gorm:"column:external_payment_id"`
	ExternalTransferID *string   `gorm:"column:external_transfer_id"`
	FailureReason      *string   `gorm:"column:failure_reason"`
	CreatedAt          time.Time `gorm:"column:created_at;not null"`
	UpdatedAt          time.Time `gorm:"column:updated_at;not null"`
}

func (ConversionModel) TableName() string { return "conversions" }

type WebhookInboxModel struct {
	ID              string     `gorm:"column:id;type:char(26);primaryKey"`
	Provider        string     `gorm:"column:provider;type:varchar(32);not null"`
	Topic           string     `gorm:"column:topic;type:varchar(64);not null"`
	ExternalEventID string     `gorm:"column:external_event_id;type:varchar(128);not null"`
	ConversionID    string     `gorm:"column:conversion_id;type:char(26);not null"`
	ExternalRef     *string    `gorm:"column:external_ref"`
	Payload         string     `gorm:"column:payload;type:jsonb;not null"`
	ReceivedAt      time.Time  `gorm:"column:received_at;not null"`
	ProcessedAt     *time.Time `gorm:"column:processed_at"`
}

func (WebhookInboxModel) TableName() string { return "webhook_inbox" }

type OutboxEventModel struct {
	ID              string     `gorm:"column:id;type:char(26);primaryKey"`
	AggregateType   string     `gorm:"column:aggregate_type;type:varchar(64);not null"`
	AggregateID     string     `gorm:"column:aggregate_id;type:char(26);not null"`
	EventType       string     `gorm:"column:event_type;type:varchar(64);not null"`
	Payload         string     `gorm:"column:payload;type:jsonb;not null"`
	PublishAttempts int        `gorm:"column:publish_attempts;not null"`
	LastError       *string    `gorm:"column:last_error"`
	CreatedAt       time.Time  `gorm:"column:created_at;not null"`
	PublishedAt     *time.Time `gorm:"column:published_at"`
}

func (OutboxEventModel) TableName() string { return "outbox_events" }

func NewStore(db *gorm.DB, logger *zap.Logger) *Store {
	return &Store{db: db, logger: logger}
}

func (s *Store) clone(db *gorm.DB) *Store {
	return &Store{db: db, logger: s.logger}
}

func (s *Store) WithinTransaction(ctx context.Context, fn func(ctx context.Context, repos port.TransactionRepositories) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		store := s.clone(tx)
		return fn(ctx, port.TransactionRepositories{
			Rates:        store,
			Quotes:       store,
			Conversions:  store,
			WebhookInbox: store,
			Outbox:       store,
		})
	})
}

func (s *Store) UpsertRates(ctx context.Context, rates []shared.ExchangeRate) error {
	models := make([]ReferenceRateModel, 0, len(rates))
	for _, rate := range rates {
		if err := rate.Validate(); err != nil {
			return err
		}
		models = append(models, ReferenceRateModel{
			BaseCurrency:  rate.Base.String(),
			QuoteCurrency: rate.Quote.String(),
			Provider:      rate.Provider,
			Rate:          rate.Rate.String(),
			ObservedAt:    rate.ObservedAt.UTC(),
			FetchedAt:     rate.FetchedAt.UTC(),
			UpdatedAt:     time.Now().UTC(),
		})
	}
	if len(models) == 0 {
		return nil
	}

	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "provider"},
			{Name: "base_currency"},
			{Name: "quote_currency"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"rate", "observed_at", "fetched_at", "updated_at"}),
	}).Create(&models).Error; err != nil {
		return s.internalError("Failed to upsert reference rates", err)
	}

	return nil
}

func (s *Store) ListLatestRates(ctx context.Context, base shared.Currency, quotes []shared.Currency) ([]shared.ExchangeRate, error) {
	if len(quotes) == 0 {
		return nil, nil
	}

	quoteValues := make([]string, 0, len(quotes))
	for _, quoteCurrency := range quotes {
		quoteValues = append(quoteValues, quoteCurrency.String())
	}

	var models []ReferenceRateModel
	if err := s.db.WithContext(ctx).
		Model(&ReferenceRateModel{}).
		Where("provider = ? AND base_currency = ? AND quote_currency IN ?", shared.ProviderFrankfurter, base.String(), quoteValues).
		Order("quote_currency ASC").
		Find(&models).Error; err != nil {
		return nil, s.internalError("Failed to list reference rates", err)
	}

	rates := make([]shared.ExchangeRate, 0, len(models))
	for _, model := range models {
		rate, err := model.toDomain()
		if err != nil {
			return nil, err
		}
		rates = append(rates, rate)
	}

	return rates, nil
}

func (s *Store) GetQuoteByID(ctx context.Context, id string) (quote.Quote, error) {
	var model QuoteModel
	if err := s.db.WithContext(ctx).First(&model, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return quote.Quote{}, domain.NotFound("Quote not found", map[string]any{"quoteId": id})
		}
		return quote.Quote{}, s.internalError("Failed to load quote", err)
	}

	return model.toDomain()
}

func (s *Store) GetQuoteByIdempotencyKey(ctx context.Context, key shared.IdempotencyKey) (quote.Quote, bool, error) {
	var model QuoteModel
	if err := s.db.WithContext(ctx).First(&model, "idempotency_key = ?", key.String()).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return quote.Quote{}, false, nil
		}
		return quote.Quote{}, false, s.internalError("Failed to load quote by idempotency key", err)
	}

	value, err := model.toDomain()
	return value, true, err
}

func (s *Store) CreateQuote(ctx context.Context, value quote.Quote) (quote.Quote, error) {
	model := quoteModelFromDomain(value)
	if err := s.db.WithContext(ctx).Create(&model).Error; err != nil {
		if isUniqueViolation(err) {
			return quote.Quote{}, domain.Conflict("Quote idempotency key already exists", map[string]any{
				"idempotencyKey": value.IdempotencyKey.String(),
			})
		}
		return quote.Quote{}, s.internalError("Failed to create quote", err)
	}

	return model.toDomain()
}

func (s *Store) MarkQuoteAccepted(ctx context.Context, quoteID string, acceptedAt time.Time) error {
	result := s.db.WithContext(ctx).
		Model(&QuoteModel{}).
		Where("id = ? AND accepted_at IS NULL AND expires_at > ?", quoteID, acceptedAt.UTC()).
		Updates(map[string]any{"accepted_at": acceptedAt.UTC()})
	if result.Error != nil {
		return s.internalError("Failed to mark quote accepted", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.Conflict("Quote already accepted or expired", map[string]any{
			"quoteId": quoteID,
		})
	}

	return nil
}

func (s *Store) GetConversionByID(ctx context.Context, id string) (conversion.Conversion, error) {
	var model ConversionModel
	if err := s.db.WithContext(ctx).First(&model, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return conversion.Conversion{}, domain.NotFound("Conversion not found", map[string]any{"conversionId": id})
		}
		return conversion.Conversion{}, s.internalError("Failed to load conversion", err)
	}

	return model.toDomain()
}

func (s *Store) GetConversionByIdempotencyKey(ctx context.Context, key shared.IdempotencyKey) (conversion.Conversion, bool, error) {
	var model ConversionModel
	if err := s.db.WithContext(ctx).First(&model, "idempotency_key = ?", key.String()).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return conversion.Conversion{}, false, nil
		}
		return conversion.Conversion{}, false, s.internalError("Failed to load conversion by idempotency key", err)
	}

	value, err := model.toDomain()
	return value, true, err
}

func (s *Store) CreateConversion(ctx context.Context, value conversion.Conversion) (conversion.Conversion, error) {
	model := conversionModelFromDomain(value)
	if err := s.db.WithContext(ctx).Create(&model).Error; err != nil {
		if isUniqueViolation(err) {
			return conversion.Conversion{}, domain.Conflict("Conversion already exists for quote or idempotency key", map[string]any{
				"quoteId":        value.QuoteID,
				"idempotencyKey": value.IdempotencyKey.String(),
			})
		}
		return conversion.Conversion{}, s.internalError("Failed to create conversion", err)
	}

	return model.toDomain()
}

func (s *Store) UpdateConversion(ctx context.Context, value conversion.Conversion) (conversion.Conversion, error) {
	model := conversionModelFromDomain(value)
	result := s.db.WithContext(ctx).Save(&model)
	if result.Error != nil {
		return conversion.Conversion{}, s.internalError("Failed to update conversion", result.Error)
	}
	if result.RowsAffected == 0 {
		return conversion.Conversion{}, domain.NotFound("Conversion not found", map[string]any{"conversionId": value.ID})
	}

	return model.toDomain()
}

func (s *Store) StoreIfAbsent(ctx context.Context, message webhook.InboxMessage) (bool, error) {
	if err := message.Validate(); err != nil {
		return false, err
	}

	model := webhookModelFromDomain(message)
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "provider"},
			{Name: "external_event_id"},
		},
		DoNothing: true,
	}).Create(&model)
	if result.Error != nil {
		return false, s.internalError("Failed to store webhook inbox message", result.Error)
	}

	return result.RowsAffected > 0, nil
}

func (s *Store) Enqueue(ctx context.Context, event outbox.Event) error {
	model := outboxModelFromDomain(event)
	if err := s.db.WithContext(ctx).Create(&model).Error; err != nil {
		return s.internalError("Failed to enqueue outbox event", err)
	}

	return nil
}

func (s *Store) ListPending(ctx context.Context, limit int) ([]outbox.Event, error) {
	if limit <= 0 {
		limit = 100
	}

	var models []OutboxEventModel
	if err := s.db.WithContext(ctx).
		Model(&OutboxEventModel{}).
		Where("published_at IS NULL").
		Order("created_at ASC").
		Limit(limit).
		Find(&models).Error; err != nil {
		return nil, s.internalError("Failed to list pending outbox events", err)
	}

	events := make([]outbox.Event, 0, len(models))
	for _, model := range models {
		event, err := model.toDomain()
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}

	return events, nil
}

func (s *Store) MarkPublished(ctx context.Context, eventID string, publishedAt time.Time) error {
	result := s.db.WithContext(ctx).
		Model(&OutboxEventModel{}).
		Where("id = ?", eventID).
		Updates(map[string]any{
			"published_at":     publishedAt.UTC(),
			"last_error":       nil,
			"publish_attempts": gorm.Expr("publish_attempts + 1"),
		})
	if result.Error != nil {
		return s.internalError("Failed to mark outbox event published", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.NotFound("Outbox event not found", map[string]any{"eventId": eventID})
	}

	return nil
}

func (s *Store) MarkPublishFailed(ctx context.Context, eventID string, lastError string) error {
	result := s.db.WithContext(ctx).
		Model(&OutboxEventModel{}).
		Where("id = ?", eventID).
		Updates(map[string]any{
			"last_error":       lastError,
			"publish_attempts": gorm.Expr("publish_attempts + 1"),
		})
	if result.Error != nil {
		return s.internalError("Failed to mark outbox publish failure", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.NotFound("Outbox event not found", map[string]any{"eventId": eventID})
	}

	return nil
}

func quoteModelFromDomain(value quote.Quote) QuoteModel {
	return QuoteModel{
		ID:              value.ID,
		IdempotencyKey:  value.IdempotencyKey.String(),
		BaseCurrency:    value.BaseAmount.Currency.String(),
		BaseMinorUnits:  value.BaseAmount.MinorUnits,
		QuoteCurrency:   value.QuoteAmount.Currency.String(),
		QuoteMinorUnits: value.QuoteAmount.MinorUnits,
		FeeMinorUnits:   value.FeeAmount.MinorUnits,
		TotalDebitMinor: value.TotalDebitAmount.MinorUnits,
		Rate:            value.Rate.String(),
		RateProvider:    value.RateProvider,
		ExpiresAt:       value.ExpiresAt.UTC(),
		AcceptedAt:      value.AcceptedAt,
		CreatedAt:       value.CreatedAt.UTC(),
	}
}

func (m QuoteModel) toDomain() (quote.Quote, error) {
	rate, err := decimal.NewFromString(m.Rate)
	if err != nil {
		return quote.Quote{}, domain.Internal("Stored quote rate is invalid", nil).WithCause(err)
	}

	baseCurrency := shared.MustCurrency(m.BaseCurrency)
	quoteCurrency := shared.MustCurrency(m.QuoteCurrency)

	return quote.Quote{
		ID:               m.ID,
		IdempotencyKey:   shared.IdempotencyKey(m.IdempotencyKey),
		BaseAmount:       shared.MustMoney(baseCurrency, m.BaseMinorUnits),
		QuoteAmount:      shared.MustMoney(quoteCurrency, m.QuoteMinorUnits),
		FeeAmount:        shared.MustMoney(baseCurrency, m.FeeMinorUnits),
		TotalDebitAmount: shared.MustMoney(baseCurrency, m.TotalDebitMinor),
		Rate:             rate,
		RateProvider:     m.RateProvider,
		ExpiresAt:        m.ExpiresAt.UTC(),
		AcceptedAt:       m.AcceptedAt,
		CreatedAt:        m.CreatedAt.UTC(),
	}, nil
}

func conversionModelFromDomain(value conversion.Conversion) ConversionModel {
	return ConversionModel{
		ID:                 value.ID,
		QuoteID:            value.QuoteID,
		IdempotencyKey:     value.IdempotencyKey.String(),
		BaseCurrency:       value.BaseAmount.Currency.String(),
		BaseMinorUnits:     value.BaseAmount.MinorUnits,
		QuoteCurrency:      value.QuoteAmount.Currency.String(),
		QuoteMinorUnits:    value.QuoteAmount.MinorUnits,
		FeeMinorUnits:      value.FeeAmount.MinorUnits,
		TotalDebitMinor:    value.TotalDebitAmount.MinorUnits,
		Rate:               value.Rate.String(),
		RateProvider:       value.RateProvider,
		PaymentProvider:    value.PaymentProvider,
		TransferProvider:   value.TransferProvider,
		Status:             string(value.Status),
		ExternalPaymentID:  value.ExternalPaymentID,
		ExternalTransferID: value.ExternalTransferID,
		FailureReason:      value.FailureReason,
		CreatedAt:          value.CreatedAt.UTC(),
		UpdatedAt:          value.UpdatedAt.UTC(),
	}
}

func (m ConversionModel) toDomain() (conversion.Conversion, error) {
	rate, err := decimal.NewFromString(m.Rate)
	if err != nil {
		return conversion.Conversion{}, domain.Internal("Stored conversion rate is invalid", nil).WithCause(err)
	}

	baseCurrency := shared.MustCurrency(m.BaseCurrency)
	quoteCurrency := shared.MustCurrency(m.QuoteCurrency)

	return conversion.Conversion{
		ID:                 m.ID,
		QuoteID:            m.QuoteID,
		IdempotencyKey:     shared.IdempotencyKey(m.IdempotencyKey),
		BaseAmount:         shared.MustMoney(baseCurrency, m.BaseMinorUnits),
		QuoteAmount:        shared.MustMoney(quoteCurrency, m.QuoteMinorUnits),
		FeeAmount:          shared.MustMoney(baseCurrency, m.FeeMinorUnits),
		TotalDebitAmount:   shared.MustMoney(baseCurrency, m.TotalDebitMinor),
		Rate:               rate,
		RateProvider:       m.RateProvider,
		PaymentProvider:    m.PaymentProvider,
		TransferProvider:   m.TransferProvider,
		Status:             conversion.Status(m.Status),
		ExternalPaymentID:  m.ExternalPaymentID,
		ExternalTransferID: m.ExternalTransferID,
		FailureReason:      m.FailureReason,
		CreatedAt:          m.CreatedAt.UTC(),
		UpdatedAt:          m.UpdatedAt.UTC(),
	}, nil
}

func webhookModelFromDomain(message webhook.InboxMessage) WebhookInboxModel {
	return WebhookInboxModel{
		ID:              message.ID,
		Provider:        message.Provider,
		Topic:           message.Topic,
		ExternalEventID: message.ExternalEventID,
		ConversionID:    message.ConversionID,
		ExternalRef:     message.ExternalRef,
		Payload:         string(message.Payload),
		ReceivedAt:      message.ReceivedAt.UTC(),
		ProcessedAt:     message.ProcessedAt,
	}
}

func outboxModelFromDomain(event outbox.Event) OutboxEventModel {
	return OutboxEventModel{
		ID:              event.ID,
		AggregateType:   event.AggregateType,
		AggregateID:     event.AggregateID,
		EventType:       event.EventType,
		Payload:         string(event.Payload),
		PublishAttempts: event.PublishAttempts,
		LastError:       event.LastError,
		CreatedAt:       event.CreatedAt.UTC(),
		PublishedAt:     event.PublishedAt,
	}
}

func (m OutboxEventModel) toDomain() (outbox.Event, error) {
	return outbox.Event{
		ID:              m.ID,
		AggregateType:   m.AggregateType,
		AggregateID:     m.AggregateID,
		EventType:       m.EventType,
		Payload:         json.RawMessage(m.Payload),
		PublishAttempts: m.PublishAttempts,
		LastError:       m.LastError,
		CreatedAt:       m.CreatedAt.UTC(),
		PublishedAt:     m.PublishedAt,
	}, nil
}

func (m ReferenceRateModel) toDomain() (shared.ExchangeRate, error) {
	rate, err := decimal.NewFromString(m.Rate)
	if err != nil {
		return shared.ExchangeRate{}, domain.Internal("Stored reference rate is invalid", nil).WithCause(err)
	}

	return shared.ExchangeRate{
		Base:       shared.MustCurrency(m.BaseCurrency),
		Quote:      shared.MustCurrency(m.QuoteCurrency),
		Provider:   m.Provider,
		Rate:       rate,
		ObservedAt: m.ObservedAt.UTC(),
		FetchedAt:  m.FetchedAt.UTC(),
	}, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (s *Store) internalError(message string, err error) error {
	s.logger.Error("postgres store failure", zap.String("message", message), zap.Error(err))
	return domain.Internal(message, map[string]any{
		"reason": "PostgresError",
	}).WithCause(err)
}

func (s *Store) String() string {
	return fmt.Sprintf("postgres.Store(%p)", s)
}
