#!/bin/sh
set -eu

fail() { printf 'FAIL: %s\n' "$1" >&2; exit 1; }
pass() { printf 'OK: %s\n' "$1"; }

[ -n "${AGENT_DB_PASSWORD:-}" ] || fail "AGENT_DB_PASSWORD is missing"
[ -n "${AGENT_ENCRYPTION_KEY:-}" ] || fail "AGENT_ENCRYPTION_KEY is missing"
[ "${PRIVATE_AGENT_LIVE_DEFAULT:-false}" = "false" ] || fail "PRIVATE_AGENT_LIVE_DEFAULT must be false"

port="${AGENT_PORT:-18080}"
if command -v ss >/dev/null 2>&1 && ss -ltn | awk '{print $4}' | grep -Eq "[:.]${port}$"; then
  fail "port ${port} is already occupied"
fi
pass "port ${port} is free"

free_mb=$(df -Pm . | awk 'NR==2 {print $4}')
[ "$free_mb" -ge 2048 ] || fail "less than 2 GB disk is free"
pass "disk has ${free_mb} MB free"

command -v docker >/dev/null 2>&1 || fail "docker is not installed"
docker compose -f docker-compose.agent.yml config >/dev/null || fail "agent compose configuration is invalid"
pass "compose configuration is valid"

printf 'Preflight passed. No containers were started.\n'

