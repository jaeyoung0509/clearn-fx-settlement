#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

mkdir -p .tmp

if [[ ! -f .env ]]; then
	cp .env.example .env
fi

wait_for_postgres() {
	local deadline
	deadline=$((SECONDS + 60))

	until docker inspect --format '{{.State.Health.Status}}' fx-settlement-postgres 2>/dev/null | grep -q '^healthy$'; do
		if (( SECONDS >= deadline )); then
			printf 'postgres did not become healthy in time\n' >&2
			return 1
		fi
		sleep 1
	done
}

cleanup() {
	local exit_code="$1"
	set +e
	for pid_file in .tmp/api.pid .tmp/grpc.pid .tmp/rpc.pid; do
		if [[ -f "$pid_file" ]]; then
			pid="$(cat "$pid_file")"
			kill "$pid" 2>/dev/null || true
			wait "$pid" 2>/dev/null || true
			rm -f "$pid_file"
		fi
	done

	if [[ "$exit_code" -ne 0 ]]; then
		for log_file in .tmp/api.log .tmp/grpc.log .tmp/rpc.log; do
			if [[ -f "$log_file" ]]; then
				printf '\n==> %s\n' "$log_file"
				tail -n 40 "$log_file" || true
			fi
		done
	fi
}

trap 'cleanup $?' EXIT

docker compose up -d postgres prometheus grafana
wait_for_postgres
go run ./cmd/migrate up

go run ./cmd/api > .tmp/api.log 2>&1 &
echo "$!" > .tmp/api.pid

go run ./cmd/grpc > .tmp/grpc.log 2>&1 &
echo "$!" > .tmp/grpc.pid

go run ./cmd/rpc > .tmp/rpc.log 2>&1 &
echo "$!" > .tmp/rpc.pid

go run ./cmd/demo-traffic

printf '\n== HTTP metrics ==\n'
curl -fsS http://localhost:9101/metrics | grep 'fx_inbound_requests_total' || true

printf '\n== gRPC metrics ==\n'
curl -fsS http://localhost:9102/metrics | grep 'fx_inbound_requests_total' || true

printf '\n== RPC metrics ==\n'
curl -fsS http://localhost:9103/metrics | grep 'fx_inbound_requests_total' || true

printf '\nPrometheus: http://localhost:9090\n'
printf 'Grafana: http://localhost:3000\n'
printf 'Server logs: .tmp/api.log, .tmp/grpc.log, .tmp/rpc.log\n'