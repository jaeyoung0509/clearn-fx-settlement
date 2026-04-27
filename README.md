# FX Settlement Backend

A Go backend project for a simplified FX quote, conversion, webhook, and settlement workflow.

This project demonstrates how I structure a backend system with:

- Domain-Driven Design
- Ports & Adapters / Hexagonal Architecture
- Clean Architecture
- Idempotent write APIs
- Webhook deduplication
- Outbox-style event publishing
- PostgreSQL persistence
- Prometheus metrics and Grafana dashboard

The main goal is to keep business logic independent from transport and infrastructure details.

---

## Core flow

```text
get reference rates
  -> create quote
  -> create conversion
  -> receive payment webhook
  -> receive transfer webhook
  -> update conversion state
  -> enqueue outbox event
```

The default supported corridors are:

- KRW/USD
- KRW/JPY
- KRW/EUR

Money is stored as integer minor units. Floating point values are not used for monetary amounts.

---

## Architecture

```text
HTTP / gRPC / net-rpc
        |
        v
 inbound adapters
        |
        v
     usecases
        |
        v
      ports
        |
        v
 outbound adapters
        |
        +--> PostgreSQL
        +--> Frankfurter FX API
        +--> HMAC webhook verifier
        +--> outbox publisher
        +--> Prometheus telemetry
```

The usecase layer is the application core.

HTTP, gRPC, and net/rpc adapters all call the same usecases. This shows that the business logic is not coupled to a specific transport framework.

---

## Package structure

```text
cmd/
  api/              HTTP server
  grpc/             gRPC server
  rpc/              net/rpc server
  migrate/          database migration command
  demo-traffic/     sample traffic generator

internal/
  domain/
    shared/         Money, Currency, ExchangeRate, IdempotencyKey
    quote/          Quote aggregate
    conversion/     Conversion aggregate and state transitions
    webhook/        Inbox message model
    outbox/         Outbox event model

  usecase/          Application usecases
  port/             Interfaces for repositories, providers, publishers, telemetry

  adapter/
    inbound/
      http/         Gin handlers and middleware
      grpc/         Protobuf/gRPC adapter
      rpc/          net/rpc adapter

    outbound/
      postgres/     GORM persistence, transactions, migrations
      frankfurter/  FX reference rate provider
      webhooksigning/
      observability/
```

---

## Main usecases

- `GetReferenceRates`
- `CreateQuote`
- `AcceptQuote`
- `GetConversion`
- `HandlePaymentWebhook`
- `HandleTransferWebhook`
- `PublishOutbox`

---

## Design decisions

### Domain-first structure

Domain models and usecases do not depend on Gin, gRPC, GORM, Prometheus, or any external framework.

This keeps the core logic testable and reusable.

### Explicit idempotency

Write operations require idempotency keys.

- HTTP: `Idempotency-Key` header
- gRPC: `idempotency-key` metadata
- net/rpc: `IdempotencyKey` request field

This prevents duplicated quote or conversion creation when clients retry requests.

### Webhook deduplication

Webhook events are deduplicated by:

```text
provider + externalEventId
```

This models how payment-provider webhooks should be handled when retries happen.

### Outbox seam

Domain events are persisted through an outbox-style model.

The current publisher is simple, but the boundary is ready for Kafka, SNS/SQS, or another broker.

### Observability

Each transport exposes Prometheus metrics so runtime behaviour can be compared across HTTP, gRPC, and net/rpc.

---

## Prerequisites

- Go 1.22+
- Docker and Docker Compose
- Optional: `grpcurl`
- Optional: `jq`

---

## Local ports

| Purpose | Address |
|---|---:|
| HTTP API | `:8000` |
| gRPC API | `:9000` |
| net/rpc API | `:9100` |
| HTTP metrics | `:9101` |
| gRPC metrics | `:9102` |
| RPC metrics | `:9103` |
| PostgreSQL | `:5432` |
| Prometheus | `:9090` |
| Grafana | `:3000` |

---

## Quick start

```bash
cp .env.example .env
docker compose up -d postgres prometheus grafana
go run ./cmd/migrate up
go run ./cmd/api
```

In separate terminals:

```bash
go run ./cmd/grpc
go run ./cmd/rpc
```

---

## Make targets

```bash
make infra-up
make migrate-up
make run-api
make run-grpc
make run-rpc
```

---

## Run tests

```bash
go test ./...
```

Integration tests use Testcontainers with PostgreSQL.

If Docker is unavailable, integration tests are skipped automatically.

---

## HTTP API

### Get rates

```bash
curl 'http://localhost:8000/api/v1/rates?base=KRW&quotes=USD,JPY,EUR'
```

### Create quote

```bash
curl -X POST 'http://localhost:8000/api/v1/quotes' \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: quote-001' \
  -d '{
    "baseAmount": {
      "currency": "KRW",
      "minorUnits": 100000
    },
    "quoteCurrency": "USD"
  }'
```

### Create conversion

```bash
curl -X POST 'http://localhost:8000/api/v1/conversions' \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: conversion-001' \
  -d '{
    "quoteId": "01JS0000000000000000000000"
  }'
```

### Other endpoints

```text
GET  /api/v1/conversions/:conversionId
POST /api/v1/webhooks/payments
POST /api/v1/webhooks/transfers
GET  /health
GET  /ready
```

---

## gRPC API

Service:

```text
fx.v1.FXService
```

Methods:

```text
GetRates
CreateQuote
CreateConversion
GetConversion
```

Example:

```bash
grpcurl -plaintext \
  -H 'idempotency-key: grpc-quote-001' \
  -d '{
    "baseAmount": {
      "currency": "KRW",
      "minorUnits": 100000
    },
    "quoteCurrency": "USD"
  }' \
  localhost:9000 fx.v1.FXService/CreateQuote
```

---

## net/rpc API

Methods:

```text
FXRPCService.GetRates
FXRPCService.CreateQuote
FXRPCService.CreateConversion
FXRPCService.GetConversion
```

The net/rpc adapter is included to show that legacy transports can reuse the same application core.

---

## Observability

Metrics endpoints:

```text
http://localhost:9101/metrics
http://localhost:9102/metrics
http://localhost:9103/metrics
```

Useful metrics:

```text
fx_inbound_requests_total{transport,operation,outcome}
fx_inbound_request_duration_seconds{transport,operation}
fx_webhook_duplicate_total
fx_webhook_accepted_total
fx_outbox_published_total
fx_outbox_publish_failed_total
```

Quick check:

```bash
curl http://localhost:9101/metrics | grep fx_inbound_requests_total
```

Grafana:

```text
http://localhost:3000
```

Default login:

```text
admin / admin
```

---

## Demo

```bash
make observability-demo
```

This command:

- starts local infrastructure
- runs migrations
- starts HTTP, gRPC, and net/rpc servers
- sends sample success and error traffic
- prints metrics from each transport

---

## Testing strategy

The project uses:

- domain tests for validation and state transitions
- usecase tests with fake ports
- adapter tests for request mapping
- PostgreSQL integration tests with Testcontainers
- observability demo traffic for runtime verification

The aim is to test business rules without requiring infrastructure unless the test specifically targets an adapter.

---

## External integrations

Implemented:

- Frankfurter API for reference FX rates
- PostgreSQL for persistence
- Prometheus for metrics
- Grafana for dashboarding

Possible extensions:

- Stripe test mode for payment flows
- Wise sandbox for cross-border settlement
- Kafka or SNS/SQS for outbox publishing
- OpenTelemetry for distributed tracing

---

## Limitations

This is a take-home sized backend, not a full production FX platform.

Current limitations:

- no real ledger implementation
- no production authentication or authorization
- no external message broker yet
- simplified webhook payloads
- limited FX corridors
- no distributed tracing

The code is structured so these can be added without rewriting the domain or usecase layers.

---

## Summary

This project focuses on correctness, boundaries, and operational visibility.

The core business logic lives in domain models and usecases.  
Transports and infrastructure are implemented as replaceable adapters.
