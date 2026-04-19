SHELL := /bin/bash

.DEFAULT_GOAL := help

.PHONY: help bootstrap-env test infra-up infra-down wait-postgres migrate-up run-api run-grpc run-rpc observability-traffic metrics-snapshot prometheus-query observability-demo

help:
	@printf "%s\n" \
		"Available targets:" \
		"  make test                 - run the full Go test suite" \
		"  make infra-up             - start postgres, prometheus, and grafana" \
		"  make wait-postgres        - wait until local postgres is healthy" \
		"  make infra-down           - stop local docker compose services" \
		"  make migrate-up           - apply database migrations" \
		"  make run-api              - run HTTP server with metrics endpoint" \
		"  make run-grpc             - run gRPC server with metrics endpoint" \
		"  make run-rpc              - run net/rpc server with metrics endpoint" \
		"  make observability-traffic - send sample traffic across HTTP, gRPC, and RPC" \
		"  make metrics-snapshot     - print inbound request metrics from local endpoints" \
		"  make prometheus-query     - query Prometheus for transport/outcome counters" \
		"  make observability-demo   - boot infra, run servers, send sample traffic, print metrics"

bootstrap-env:
	@[ -f .env ] || cp .env.example .env

test:
	@go test ./...

infra-up: bootstrap-env
	@docker compose up -d postgres prometheus grafana
	@$(MAKE) wait-postgres

wait-postgres:
	@deadline=$$((SECONDS + 60)); \
	until docker inspect --format '{{.State.Health.Status}}' fx-settlement-postgres 2>/dev/null | grep -q '^healthy$$'; do \
		if (( SECONDS >= deadline )); then \
			echo 'postgres did not become healthy in time' >&2; \
			exit 1; \
		fi; \
		sleep 1; \
	done

infra-down:
	@docker compose down

migrate-up: bootstrap-env
	@go run ./cmd/migrate up

run-api: bootstrap-env
	@go run ./cmd/api

run-grpc: bootstrap-env
	@go run ./cmd/grpc

run-rpc: bootstrap-env
	@go run ./cmd/rpc

observability-traffic:
	@go run ./cmd/demo-traffic

metrics-snapshot:
	@printf '\n[http]\n'
	@curl -fsS http://localhost:9101/metrics | grep 'fx_inbound_requests_total' || true
	@printf '\n[grpc]\n'
	@curl -fsS http://localhost:9102/metrics | grep 'fx_inbound_requests_total' || true
	@printf '\n[rpc]\n'
	@curl -fsS http://localhost:9103/metrics | grep 'fx_inbound_requests_total' || true

prometheus-query:
	@curl -fsS --get --data-urlencode 'query=sum by (job,transport,outcome)(fx_inbound_requests_total)' http://localhost:9090/api/v1/query

observability-demo:
	@./scripts/observability_demo.sh