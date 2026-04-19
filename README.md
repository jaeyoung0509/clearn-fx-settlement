# FX Settlement Go Backend

Gin + GORM + PostgreSQL 기반의 핀테크 강의용 FX 정산/환전 백엔드입니다.

## Architecture
- `internal/domain/shared`
  - `Money`, `Currency`, `ExchangeRate`, `IdempotencyKey`, `ProviderEvent`
- `internal/domain/quote`
  - 견적 생성, 만료, 수락 가능 여부
- `internal/domain/conversion`
  - 환전 요청 상태 전이
- `internal/domain/webhook`
  - inbox message 모델
- `internal/domain/outbox`
  - 발행 대기 이벤트 모델
- `internal/usecase`
  - `GetReferenceRates`, `SyncReferenceRates`, `CreateQuote`, `AcceptQuote`, `GetConversion`, `HandlePaymentWebhook`, `HandleTransferWebhook`, `PublishOutbox`
- `internal/port`
  - repository / provider / publisher / clock / telemetry / unit of work 계약
- `internal/adapter/inbound/http`
  - Gin handler, request validation, response mapping
- `internal/adapter/inbound/grpc`
  - protobuf/gRPC service mapping, metadata extraction, status code mapping
- `internal/adapter/inbound/rpc`
  - stdlib net/rpc service mapping, request DTO 변환
- `internal/adapter/outbound/postgres`
  - GORM store + transaction boundary + inbox/outbox persistence
- `internal/adapter/outbound/frankfurter`
  - 무료 reference rate provider
- `internal/adapter/outbound/{toss,stripe,plaid,wise}`
  - 후속 실습용 provider seam
- `internal/adapter/outbound/webhooksigning`
  - 로컬/강의용 HMAC webhook 검증
- `internal/adapter/outbound/observability`
  - Prometheus exporter + request/domain telemetry

현재는 `모놀리식 + 분리 경계` 구조입니다. 나중에 `quote`, `conversion`, `webhook`, `ledger`, `notification`으로 분리할 수 있게 이름과 경계를 먼저 맞춰뒀습니다.

## Public API
- `GET /api/v1/rates?base=KRW&quotes=USD,JPY,EUR`
- `POST /api/v1/quotes`
- `POST /api/v1/conversions`
- `GET /api/v1/conversions/:conversionId`
- `POST /api/v1/webhooks/payments`
- `POST /api/v1/webhooks/transfers`
- `GET /health`
- `GET /ready`

## gRPC API
- `fx.v1.FXService/GetRates`
- `fx.v1.FXService/CreateQuote`
- `fx.v1.FXService/CreateConversion`
- `fx.v1.FXService/GetConversion`

## RPC API
- `FXRPCService.GetRates`
- `FXRPCService.CreateQuote`
- `FXRPCService.CreateConversion`
- `FXRPCService.GetConversion`

## Metrics Endpoints
- HTTP metrics: `GET http://localhost:9101/metrics`
- gRPC metrics: `GET http://localhost:9102/metrics`
- RPC metrics: `GET http://localhost:9103/metrics`

공통 메트릭은 `fx_inbound_requests_total`, `fx_inbound_request_duration_seconds`로 노출되고, `transport`, `operation`, `outcome` 라벨을 가집니다.
도메인 메트릭은 `fx_webhook_duplicate_total`, `fx_webhook_accepted_total`, `fx_outbox_published_total`, `fx_outbox_publish_failed_total`를 그대로 유지합니다.

모든 성공 응답은 `{ success, eventTime, data }`, 에러 응답은 `{ eventTime, error: { code, message, details, requestId } }`를 유지합니다.
gRPC는 같은 usecase를 재사용하고, `Idempotency-Key` 대신 gRPC metadata의 `idempotency-key`를 사용합니다.
RPC는 같은 usecase를 재사용하고, `IdempotencyKey`를 요청 DTO 필드로 받습니다.

## Domain Notes
- 기준 통화는 `KRW`
- 기본 corridor는 `KRW/USD`, `KRW/JPY`, `KRW/EUR`
- 금액은 `minor unit integer` 기반으로 처리하고 `float`는 사용하지 않습니다
- `POST /quotes`, `POST /conversions`는 `Idempotency-Key` 헤더가 필요합니다
- webhook dedupe 기준은 `provider + externalEventId`
- 첫 단계에서는 `견적 -> 환전 요청 -> payment webhook -> transfer webhook -> 상태 변경 -> outbox 적재` 흐름에 집중합니다

## Local Run
```bash
cp .env.example .env
docker compose up -d postgres prometheus grafana
go run ./cmd/migrate up
go run ./cmd/api
go run ./cmd/grpc
go run ./cmd/rpc
```

## Make Targets
```bash
make infra-up
make migrate-up
make run-api
make run-grpc
make run-rpc
```

관측성 데모를 한 번에 확인하려면 아래 한 줄이면 됩니다.

```bash
make observability-demo
```

이 타깃은 다음 순서로 실행됩니다.
- `.env`가 없으면 `.env.example`을 복사
- postgres, prometheus, grafana 기동
- migration 적용
- HTTP, gRPC, net/rpc 서버를 백그라운드로 실행
- 세 transport에 샘플 성공/실패 요청을 전송
- 각 metrics endpoint에서 `fx_inbound_requests_total`을 출력

샘플 트래픽만 따로 보내고 싶으면 아래 타깃을 사용하면 됩니다.

```bash
make observability-traffic
make metrics-snapshot
make prometheus-query
```

Grafana는 `http://localhost:3000`에서 `admin/admin`으로 바로 들어갈 수 있고, Prometheus datasource와 대시보드를 자동으로 provisioning 합니다.
Prometheus는 `http://localhost:9090`에서 각 프로세스의 metrics endpoint를 scrape 합니다.

## Example Requests
```bash
curl 'http://localhost:8000/api/v1/rates?base=KRW&quotes=USD,JPY,EUR'
```

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

```bash
curl -X POST 'http://localhost:8000/api/v1/conversions' \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: conversion-001' \
  -d '{
    "quoteId": "01JS0000000000000000000000"
  }'
```

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

## Monitoring Walkthrough
```bash
curl http://localhost:9101/metrics | grep fx_inbound_requests_total
curl http://localhost:9102/metrics | grep fx_inbound_requests_total
curl http://localhost:9103/metrics | grep fx_inbound_requests_total
```

Grafana 대시보드에서는 다음을 바로 비교할 수 있습니다.
- transport/job별 요청 처리량
- transport/job별 p95 latency
- transport별 client_error / server_error 비율
- webhook 중복/수락 카운트와 outbox publish 결과

## Tests
```bash
go test ./...
```

### Integration Testing Notes
- 통합테스트는 `testcontainers-go`로 PostgreSQL 컨테이너를 띄웁니다.
- Docker가 없거나 데몬에 접근할 수 없는 환경에서는 통합테스트를 자동으로 skip 합니다.
- top-level test 당 컨테이너 1개만 생성하고 subtest마다 `Reset(t)`으로 상태를 비웁니다.
- cleanup은 `t.Cleanup`으로 등록해서 DB/pool이 먼저 닫히고 컨테이너가 마지막에 내려가도록 설계했습니다.
- Docker가 있는 로컬과 GitHub Actions를 동일한 전제로 가져갈 수 있습니다.

## Sandbox Direction
- 기본 환율 provider: [Frankfurter](https://frankfurter.dev/docs/)
- 한국 결제 webhook 데모: Toss Payments 테스트 키
- 글로벌 결제 확장: Stripe sandbox/test mode
- 은행/계좌 이벤트 확장: Plaid sandbox
- 송금/정산 확장: Wise sandbox

현재 구현은 Frankfurter를 실제로 붙이고, payment/transfer webhook은 강의용 HMAC 검증기와 inbox/outbox 흐름을 먼저 고정한 상태입니다.
