#!/usr/bin/env bash
# Non-destructive smoke checks; only short-lived limiter/probe keys are written.
set -euo pipefail
cd "$(dirname "$0")/.."
compose=(docker compose -f docker/docker-compose.yml)
"${compose[@]}" ps
[[ $("${compose[@]}" exec -T redis redis-cli PING) == PONG ]]
services=(auth user post chat notification)
for index in "${!services[@]}"; do
  service="${services[$index]}-service"
  port=$((8001 + index))
  "${compose[@]}" exec -T "$service" /app/server --check-redis
  "${compose[@]}" exec -T "$service" wget -q -O - "http://127.0.0.1:$port/health"
  printf '\n'
  # Unknown routes exercise the installed global limiter without business writes.
  code=$(curl -sS -o /dev/null -w '%{http_code}' "http://127.0.0.1:$port/redis-verification")
  [[ "$code" == 404 || "$code" == 429 ]]
  keys=$("${compose[@]}" exec -T redis redis-cli --scan --pattern "ratelimit:$service:*")
  [[ -n "$keys" ]]
  while IFS= read -r key; do
    ttl=$("${compose[@]}" exec -T redis redis-cli TTL "$key")
    [[ "$ttl" -ge 0 ]]
    printf '%s TTL=%s\n' "$key" "$ttl"
  done <<< "$keys"
  # A separate process verifies fail-fast without interrupting the running server.
  if "${compose[@]}" exec -T -e REDIS_ADDR=127.0.0.1:1 "$service" /app/server --check-redis; then
    printf 'FAIL: %s accepted unreachable Redis\n' "$service" >&2
    exit 1
  fi
  printf '%s PASS: PING, SET/GET/TTL, health, namespace, failure\n' "$service"
done
