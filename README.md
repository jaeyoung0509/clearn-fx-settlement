*** Begin File
# FX Settlement Lab — Go Backend

This repository is a lecture-style, hands-on fintech backend for FX settlement and currency conversion, implemented in Go using DDD, Ports & Adapters (Hexagonal), and Clean Architecture. The project is designed so that each commit represents a small lecture or step: check out a commit, read the files, run the tests, and learn.

What this repo teaches
- Domain-first development: build the domain model first, add framework adapters later.
- Usecases as the application core: usecases orchestrate domain logic and remain stable across transports.
- Multi-transport reuse: HTTP, gRPC, and net/rpc all reuse the same usecases.
- Outbox / Inbox seam: design for event-driven flows (easy to migrate to Kafka later).
- Observability: instrument transports and domain logic with Prometheus and visualize with Grafana.

Philosophy
- Design the domain and contracts before wiring adapters.
- Keep usecases framework-agnostic and testable.
- Treat adapters as replaceable seams.
- Observability is a verification tool, not just decoration.

Architecture (at a glance)

```
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
        +--> webhook verifier (HMAC)
        +--> event publisher / outbox
        +--> Prometheus telemetry
```

Package map (key folders)
- `internal/domain/shared` — `Money`, `Currency`, `ExchangeRate`, `IdempotencyKey`, `ProviderEvent`
- `internal/domain/quote` — quote aggregate and validation
- `internal/domain/conversion` — conversion state transitions
- `internal/domain/webhook` — inbox message model
- `internal/domain/outbox` — outbox event model
- `internal/usecase` — `GetReferenceRates`, `CreateQuote`, `AcceptQuote`, `GetConversion`, `HandlePaymentWebhook`, `HandleTransferWebhook`, `PublishOutbox`
- `internal/port` — repository/provider/publisher/clock/telemetry/unit-of-work contracts
- `internal/adapter/inbound/http` — Gin handlers, validation, response mapping, middleware
- `internal/adapter/inbound/grpc` — protobuf/gRPC service adapter, metadata handling
- `internal/adapter/inbound/rpc` — stdlib net/rpc adapter
- `internal/adapter/outbound/postgres` — GORM store, transactions, migrations, inbox/outbox persistence
- `internal/adapter/outbound/frankfurter` — reference rate provider adapter
- `internal/adapter/outbound/webhooksigning` — HMAC webhook verification for lecture demos
- `internal/adapter/outbound/observability` — Prometheus exporters and telemetry helpers
- `cmd/` — entrypoints: `api`, `grpc`, `rpc`, `migrate`

Prerequisites
- Go 1.22+
- Docker and Docker Compose (for local DB and monitoring stack)
- macOS or Linux shell
- Optional but helpful: `grpcurl` for gRPC examples, `jq` for JSON inspection

Local ports (defaults)
| Purpose | Address | Notes |
|---|---:|---|
| HTTP API | `:8000` | REST endpoints (Gin) |
| gRPC API | `:9000` | proto-defined API |
| net/rpc API | `:9100` | legacy RPC adapter |
| HTTP metrics | `:9101` | Prometheus endpoint for HTTP process |
| gRPC metrics | `:9102` | Prometheus endpoint for gRPC process |
| RPC metrics | `:9103` | Prometheus endpoint for RPC process |
| PostgreSQL | `:5432` | application DB |
| Prometheus | `:9090` | metrics server |
| Grafana | `:3000` | dashboard UI |

Study workflow (recommended)
1. Read this README to get the big picture.
2. Inspect the git history: each commit is a lecture.

```bash
git log --reverse --oneline
git checkout <commit-hash>   # jump to a lecture
git switch main              # return to the main branch
git diff <older> <newer>     # view what changed between lectures
```

Lecture roadmap (high-level)
| Lecture | Commit | Topic |
|---:|---|---|
| 00 | `896311d` | Foundation & configuration (config, logger, testable start) |
| 01 | `5bad74c` | Core domain & port contracts (Money, Quote, Conversion) |
| 02 | `768bf4b` | Usecase layer (application orchestration) |
| 03 | `d7321d8` | Outbound adapter seams (providers, publisher, observability) |
| 04 | `825316b` | PostgreSQL persistence, migrations, inbox/outbox |
| 05 | `601710b` | HTTP inbound adapter (Gin, validation, error envelope) |
| 06 | `c973c93` | Runtime & local environment (composition root, docker) |
| 07 | `e7232c6` | Reusable utilities (keyset pagination) |
| 08 | `fc41ba6` | gRPC adapter (proto contract) |
| 09 | `b83af70` | Standalone gRPC runtime |
| 10 | `5e9b3c4` | net/rpc adapter |
| 11 | `4ffbb84` | Standalone net/rpc runtime |
| 12 | `9d06692` | Prometheus instrumentation (transport + domain) |
| 13 | `a596271` | Local Prometheus & Grafana stack |
| 14 | `5e2a593` | Make-based observability demo (one-line reproducible run) |

Lecture details (short)
- Lecture 00 — Foundation: set up `internal/config`, logger, and a testable app entrypoint.
- Lecture 01 — Domain: model `Money`, `IdempotencyKey`, `Quote`, and `Conversion` aggregates.
- Lecture 02 — Usecases: implement `CreateQuote`, `AcceptQuote`, `GetConversion`, and rate sync.
- Lecture 03 — Outbound adapters: implement the Frankfurter provider seam, webhook HMAC seam, and logging publisher.
- Lecture 04 — Persistence: add GORM-based store, migrations, and inbox/outbox tables.
- Lecture 05 — HTTP inbound: Gin handlers, validation rules, middleware, and error envelope.
- Lecture 06 — Runtime: composition root wiring, `.env` handling, Docker Compose and migrations.
- Lecture 07 — Utilities: add a reusable keyset pagination utility for listing endpoints.
- Lecture 08/09 — gRPC: proto-first contract and a standalone gRPC process.
- Lecture 10/11 — net/rpc: legacy RPC adapter and standalone process for demonstration.
- Lecture 12 — Observability: add Prometheus metrics for throughput, latency, and outcomes.
- Lecture 13 — Monitoring: provision Prometheus + Grafana and a dashboard for quick comparison.
- Lecture 14 — Demo: provide `Makefile` + `scripts/observability_demo.sh` + `cmd/demo-traffic` to reproduce metrics from all transports.

Quick start (minimal)

```bash
cp .env.example .env
docker compose up -d postgres prometheus grafana
go run ./cmd/migrate up
go run ./cmd/api
go run ./cmd/grpc
go run ./cmd/rpc
```

Make targets (convenience)

```bash
make infra-up         # start docker-based infra
make migrate-up       # apply DB migrations
make run-api          # run HTTP server (foreground)
make run-grpc         # run gRPC server
make run-rpc          # run net/rpc server
```

Observability demo (one-liner)

```bash
make observability-demo
```

What `make observability-demo` does
- Ensures `.env` exists (copies from `.env.example` if missing)
- Starts Postgres, Prometheus, and Grafana via Docker Compose
- Waits for Postgres to become healthy
- Runs migrations
- Starts the HTTP, gRPC, and net/rpc servers in the background
- Sends sample success and error traffic to each transport
- Prints `fx_inbound_requests_total` from each metrics endpoint

If you only want sample traffic or metrics snapshots:

```bash
make observability-traffic
make metrics-snapshot
make prometheus-query
```

Public API (HTTP)
- `GET /api/v1/rates?base=KRW&quotes=USD,JPY,EUR`
- `POST /api/v1/quotes`
- `POST /api/v1/conversions`
- `GET /api/v1/conversions/:conversionId`
- `POST /api/v1/webhooks/payments`
- `POST /api/v1/webhooks/transfers`
- `GET /health`
- `GET /ready`

gRPC API (proto)
- `fx.v1.FXService/GetRates`
- `fx.v1.FXService/CreateQuote`
- `fx.v1.FXService/CreateConversion`
- `fx.v1.FXService/GetConversion`

net/rpc API
- `FXRPCService.GetRates`
- `FXRPCService.CreateQuote`
- `FXRPCService.CreateConversion`
- `FXRPCService.GetConversion`

Transport notes
- All transports reuse the same usecases.
- HTTP uses an `Idempotency-Key` header.
- gRPC uses metadata `idempotency-key`.
- net/rpc uses an `IdempotencyKey` field in the request DTO.

Example requests (HTTP)

```bash
curl 'http://localhost:8000/api/v1/rates?base=KRW&quotes=USD,JPY,EUR'
```

```bash
curl -X POST 'http://localhost:8000/api/v1/quotes' \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: quote-001' \
  -d '{
    "baseAmount": {"currency":"KRW","minorUnits":100000},
    "quoteCurrency": "USD"
  }'
```

```bash
curl -X POST 'http://localhost:8000/api/v1/conversions' \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: conversion-001' \
  -d '{"quoteId":"01JS0000000000000000000000"}'
```

gRPC example with `grpcurl`

```bash
grpcurl -plaintext -H 'idempotency-key: grpc-quote-001' -d '{
  "baseAmount": {"currency":"KRW","minorUnits":100000},
  "quoteCurrency": "USD"
}' localhost:9000 fx.v1.FXService/CreateQuote
```

Monitoring and metrics

Metrics endpoints (default)
- HTTP: `http://localhost:9101/metrics`
- gRPC: `http://localhost:9102/metrics`
- net/rpc: `http://localhost:9103/metrics`

Common metrics
- `fx_inbound_requests_total{transport,operation,outcome}`
- `fx_inbound_request_duration_seconds{transport,operation}`

Domain metrics
- `fx_webhook_duplicate_total`
- `fx_webhook_accepted_total`
- `fx_outbox_published_total`
- `fx_outbox_publish_failed_total`

Quick checks

```bash
curl http://localhost:9101/metrics | grep fx_inbound_requests_total
curl http://localhost:9102/metrics | grep fx_inbound_requests_total
curl http://localhost:9103/metrics | grep fx_inbound_requests_total
```

Grafana
- Dashboard available at `http://localhost:3000` (default `admin/admin`). Provisioning is done via the repo's `monitoring/grafana` folder.

Testing

```bash
go test ./...
```

Integration testing notes
- Integration tests use `testcontainers-go` to start a PostgreSQL container; tests automatically skip when Docker is unavailable.
- Each top-level test creates a single container; subtests reset state with `Reset(t)`.
- Cleanup is registered with `t.Cleanup` so DB connections close before the container stops.

Domain notes
- Base currency: `KRW`.
- Default corridors: `KRW/USD`, `KRW/JPY`, `KRW/EUR`.
- Money is stored in minor units (integer). Floating point is not used for amounts.
- `POST /quotes` and `POST /conversions` require idempotency keys.
- Webhook deduplication key: `provider + externalEventId`.
- Core flow for the hands-on labs: `quote -> conversion -> payment webhook -> transfer webhook -> state change -> outbox enqueue`.

Next steps after this repo (suggestions for further lectures)
- Replace DB-backed outbox with Kafka-based publisher and add an inbox consumer.
- Split the monolith by bounded contexts (quote, conversion, webhook, ledger, notification).
- Move from sync request/response to event-driven flows with eventual consistency.
- Expand observability from metrics to traces and log correlation.

Sandbox integrations & notes
- Reference rate provider: Frankfurter (https://frankfurter.dev/docs/)
- Local webhook demo: Toss Payments test keys
- Global payment expansion: Stripe sandbox/test mode
- Bank/account data: Plaid sandbox
- Cross-border/settlement: Wise sandbox

The repository currently connects to Frankfurter for reference rates and includes a lecture-grade HMAC webhook verifier and inbox/outbox flow for webhook handling.

