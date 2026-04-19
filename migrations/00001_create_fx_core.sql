-- +goose Up
CREATE TABLE IF NOT EXISTS reference_rates (
    base_currency CHAR(3) NOT NULL,
    quote_currency CHAR(3) NOT NULL,
    provider VARCHAR(32) NOT NULL,
    rate NUMERIC(20, 10) NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (provider, base_currency, quote_currency)
);

CREATE TABLE IF NOT EXISTS quotes (
    id CHAR(26) PRIMARY KEY,
    idempotency_key VARCHAR(128) NOT NULL UNIQUE,
    base_currency CHAR(3) NOT NULL,
    base_minor_units BIGINT NOT NULL,
    quote_currency CHAR(3) NOT NULL,
    quote_minor_units BIGINT NOT NULL,
    fee_minor_units BIGINT NOT NULL,
    total_debit_minor_units BIGINT NOT NULL,
    rate NUMERIC(20, 10) NOT NULL,
    rate_provider VARCHAR(32) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_quotes_expires_at ON quotes (expires_at);
CREATE INDEX IF NOT EXISTS idx_quotes_created_at ON quotes (created_at DESC);

CREATE TABLE IF NOT EXISTS conversions (
    id CHAR(26) PRIMARY KEY,
    quote_id CHAR(26) NOT NULL UNIQUE REFERENCES quotes(id),
    idempotency_key VARCHAR(128) NOT NULL UNIQUE,
    base_currency CHAR(3) NOT NULL,
    base_minor_units BIGINT NOT NULL,
    quote_currency CHAR(3) NOT NULL,
    quote_minor_units BIGINT NOT NULL,
    fee_minor_units BIGINT NOT NULL,
    total_debit_minor_units BIGINT NOT NULL,
    rate NUMERIC(20, 10) NOT NULL,
    rate_provider VARCHAR(32) NOT NULL,
    payment_provider VARCHAR(32) NOT NULL,
    transfer_provider VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL,
    external_payment_id VARCHAR(128) NULL,
    external_transfer_id VARCHAR(128) NULL,
    failure_reason TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_conversions_status_updated_at ON conversions (status, updated_at DESC);

CREATE TABLE IF NOT EXISTS webhook_inbox (
    id CHAR(26) PRIMARY KEY,
    provider VARCHAR(32) NOT NULL,
    topic VARCHAR(64) NOT NULL,
    external_event_id VARCHAR(128) NOT NULL,
    conversion_id CHAR(26) NOT NULL REFERENCES conversions(id),
    external_ref VARCHAR(128) NULL,
    payload JSONB NOT NULL,
    received_at TIMESTAMPTZ NOT NULL,
    processed_at TIMESTAMPTZ NULL,
    UNIQUE (provider, external_event_id)
);

CREATE INDEX IF NOT EXISTS idx_webhook_inbox_conversion_id ON webhook_inbox (conversion_id);

CREATE TABLE IF NOT EXISTS outbox_events (
    id CHAR(26) PRIMARY KEY,
    aggregate_type VARCHAR(64) NOT NULL,
    aggregate_id CHAR(26) NOT NULL,
    event_type VARCHAR(64) NOT NULL,
    payload JSONB NOT NULL,
    publish_attempts INT NOT NULL DEFAULT 0,
    last_error TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    published_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_outbox_events_pending ON outbox_events (published_at, created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_outbox_events_pending;
DROP TABLE IF EXISTS outbox_events;
DROP INDEX IF EXISTS idx_webhook_inbox_conversion_id;
DROP TABLE IF EXISTS webhook_inbox;
DROP INDEX IF EXISTS idx_conversions_status_updated_at;
DROP TABLE IF EXISTS conversions;
DROP INDEX IF EXISTS idx_quotes_created_at;
DROP INDEX IF EXISTS idx_quotes_expires_at;
DROP TABLE IF EXISTS quotes;
DROP TABLE IF EXISTS reference_rates;
