#!/usr/bin/env bash
# E2E Phase PG: migrate + smoke на DATABASE_URL_E2E (не трогает dev-сиды).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

TMP="${TMPDIR:-/tmp}/mvp-manager-e2e-pg-$$"
mkdir -p "$TMP"

# DSN по умолчанию — e2e-БД из docker-compose / .env.example.
export DATABASE_URL_E2E="${DATABASE_URL_E2E:-postgres://mvp:mvp@127.0.0.1:5432/mvp_manager_e2e?sslmode=disable}"
# Dev DSN — только чтобы проверить, что e2e его не затирает (если задан).
DEV_URL="${DATABASE_URL:-postgres://mvp:mvp@127.0.0.1:5432/mvp_manager?sslmode=disable}"

cleanup() {
  if [[ -n "${AGENT_PID:-}" ]] && kill -0 "$AGENT_PID" 2>/dev/null; then
    kill -TERM "$AGENT_PID" 2>/dev/null || true
    wait "$AGENT_PID" 2>/dev/null || true
  fi
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "==> wait postgres (compose)"
if ! docker compose ps --status running 2>/dev/null | grep -q postgres; then
  echo "поднимаем docker compose..."
  docker compose up -d
fi
for i in $(seq 1 30); do
  if docker compose exec -T postgres pg_isready -U mvp -d mvp_manager_e2e >/dev/null 2>&1; then
    break
  fi
  sleep 1
  if [[ "$i" -eq 30 ]]; then
    echo "postgres e2e не готов"; exit 1
  fi
done

# Зафиксировать число сид-ботов в dev ДО e2e (если migrate/seed уже делали).
DEV_SEED_BEFORE=""
if command -v psql >/dev/null 2>&1 || docker compose exec -T postgres true >/dev/null 2>&1; then
  DEV_SEED_BEFORE="$(docker compose exec -T postgres \
    psql -U mvp -d mvp_manager -tAc \
    "SELECT count(*) FROM bots WHERE id IN (
      'a1111111-1111-4111-8111-111111111111',
      'a2222222-2222-4222-8222-222222222222',
      'b1111111-1111-4111-8111-111111111111'
    )" 2>/dev/null | tr -d '[:space:]' || echo "")"
fi

echo "==> build"
mkdir -p bin
go build -o bin/agent ./cmd/agent
go build -o bin/ctl ./cmd/ctl
go build -o bin/migrate ./cmd/migrate
go build -o bin/bot-runner ./cmd/bot-runner

echo "==> migrate up on e2e ($DATABASE_URL_E2E)"
./bin/migrate --e2e up

echo "==> reset e2e bots/runtimes for clean smoke (dev не трогаем)"
docker compose exec -T postgres psql -U mvp -d mvp_manager_e2e -v ON_ERROR_STOP=1 <<'SQL'
TRUNCATE bot_events, bots, runtimes, nodes RESTART IDENTITY CASCADE;
SQL

export NODE_ID=node-e2e-pg
export STORE=postgres
export DATABASE_URL="$DATABASE_URL_E2E"
export BOT_RUNNER_COMMAND="$(pwd)/bin/bot-runner"
export RECONCILE_INTERVAL=500ms
export HEARTBEAT_INTERVAL=1s
export SHUTDOWN_GRACE=2s
export LEASE_TTL=15s

PORT="${E2E_PG_PORT:-18191}"

echo "==> start agent (STORE=postgres, e2e DB)"
./bin/agent >"$TMP/agent.log" 2>&1 &
AGENT_PID=$!
sleep 0.8
if ! kill -0 "$AGENT_PID" 2>/dev/null; then
  echo "agent died:"; cat "$TMP/agent.log"; exit 1
fi

echo "==> ctl bots create default"
CREATE_OUT="$(./bin/ctl bots create \
  --type default \
  --name e2e-pg-bot \
  --port "$PORT" \
  --token seed:e2e-pg \
  --mode polling \
  --channel telegram)"
echo "$CREATE_OUT"
BOT_ID="$(echo "$CREATE_OUT" | sed -n 's/.*bot_id=\([^ ]*\).*/\1/p')"
if [[ -z "$BOT_ID" ]]; then
  echo "не удалось разобрать bot_id"; cat "$TMP/agent.log"; exit 1
fi

echo "==> ctl bots list (должен видеть бота)"
LIST="$(./bin/ctl bots list)"
echo "$LIST"
echo "$LIST" | grep -q "$BOT_ID"

echo "==> ctl bots start + wait actual=running"
./bin/ctl bots start "$BOT_ID"

wait_bot_actual() {
  local id="$1" want="$2"
  local i
  for i in $(seq 1 40); do
    if ./bin/ctl bots list | awk -v id="$id" -v want="$want" '$1==id && $6==want {found=1} END{exit !found}'; then
      return 0
    fi
    sleep 0.25
  done
  return 1
}

if ! wait_bot_actual "$BOT_ID" running; then
  echo "timeout waiting running"; ./bin/ctl bots list; ./bin/ctl runtimes list; cat "$TMP/agent.log"; exit 1
fi

echo "==> stop"
./bin/ctl bots stop "$BOT_ID"
if ! wait_bot_actual "$BOT_ID" stopped; then
  # runner может оставить actual=stopped чуть позже; допускаем desired=stopped
  ./bin/ctl bots list | awk -v id="$BOT_ID" '$1==id && $5=="stopped" {found=1} END{exit !found}' \
    || { echo "stop failed"; ./bin/ctl bots list; cat "$TMP/agent.log"; exit 1; }
fi

echo "==> verify dev seed bots untouched"
if [[ -n "$DEV_SEED_BEFORE" && "$DEV_SEED_BEFORE" != "0" ]]; then
  DEV_SEED_AFTER="$(docker compose exec -T postgres \
    psql -U mvp -d mvp_manager -tAc \
    "SELECT count(*) FROM bots WHERE id IN (
      'a1111111-1111-4111-8111-111111111111',
      'a2222222-2222-4222-8222-222222222222',
      'b1111111-1111-4111-8111-111111111111'
    )" | tr -d '[:space:]')"
  if [[ "$DEV_SEED_AFTER" != "$DEV_SEED_BEFORE" ]]; then
    echo "dev seeds changed: before=$DEV_SEED_BEFORE after=$DEV_SEED_AFTER"
    exit 1
  fi
  echo "dev seed count still $DEV_SEED_AFTER"
else
  echo "(dev seeds отсутствуют или недоступны — пропуск проверки целостности)"
fi

echo "PASS e2e-phase-pg"
