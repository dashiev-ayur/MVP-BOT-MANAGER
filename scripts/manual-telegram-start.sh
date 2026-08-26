#!/usr/bin/env bash
# Ручная проверка живого Telegram-бота через PostgreSQL (не для CI).
# Требует: Docker Compose (postgres), TELEGRAM_BOT_TOKEN от @BotFather.
#
#   export TELEGRAM_BOT_TOKEN=123456:ABC
#   ./scripts/manual-telegram-start.sh
#
# По умолчанию STORE=postgres + DATABASE_URL (dev БД mvp_manager).
# Офлайн без Docker: STORE=memory ./scripts/manual-telegram-start.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ -z "${TELEGRAM_BOT_TOKEN:-}" ]]; then
  echo "Задайте TELEGRAM_BOT_TOKEN (токен от @BotFather)" >&2
  exit 1
fi

PORT="${MANUAL_TG_PORT:-18290}"
TMP="${TMPDIR:-/tmp}/mvp-manager-manual-tg-$$"
mkdir -p "$TMP" bin

# STORE не задан → postgres (DefaultStore). Memory — только явно.
STORE_KIND="${STORE:-postgres}"
export STORE="$STORE_KIND"
export NODE_ID="${NODE_ID:-node-manual-tg}"
export RECONCILE_INTERVAL="${RECONCILE_INTERVAL:-500ms}"
export BOT_RUNNER_COMMAND="$(pwd)/bin/bot-runner"

AGENT_PID=""
TAIL_PID=""
BOT_ID=""
CLEANED=0

# Останавливаем agent и его детей (bot-runner в отдельной pgid — kill -P).
stop_agent_tree() {
  local pid="${1:-}"
  [[ -z "$pid" ]] && return 0
  if ! kill -0 "$pid" 2>/dev/null; then
    return 0
  fi
  # Сначала дети (bot-runner), потом agent.
  pkill -TERM -P "$pid" 2>/dev/null || true
  kill -TERM "$pid" 2>/dev/null || true
  local i
  for i in $(seq 1 40); do
    kill -0 "$pid" 2>/dev/null || return 0
    sleep 0.25
  done
  pkill -KILL -P "$pid" 2>/dev/null || true
  kill -KILL "$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
}

cleanup() {
  # Защита от двойного вызова (EXIT после INT).
  if [[ "$CLEANED" -eq 1 ]]; then
    return 0
  fi
  CLEANED=1

  if [[ -n "${TAIL_PID:-}" ]] && kill -0 "$TAIL_PID" 2>/dev/null; then
    kill -TERM "$TAIL_PID" 2>/dev/null || true
    wait "$TAIL_PID" 2>/dev/null || true
  fi

  if [[ -n "${BOT_ID:-}" ]] && [[ -x ./bin/ctl ]]; then
    # Не блокируем выход, если ctl/БД недоступны.
    ./bin/ctl bots stop "$BOT_ID" >/dev/null 2>&1 || true
  fi

  stop_agent_tree "${AGENT_PID:-}"
  rm -rf "$TMP"
}

on_interrupt() {
  echo "" >&2
  echo "Ctrl+C — останавливаем agent/bot-runner…" >&2
  cleanup
  exit 130
}

trap on_interrupt INT TERM
trap cleanup EXIT

go build -o bin/agent ./cmd/agent
go build -o bin/ctl ./cmd/ctl
go build -o bin/bot-runner ./cmd/bot-runner
go build -o bin/migrate ./cmd/migrate

if [[ "$STORE_KIND" == "postgres" ]]; then
  export DATABASE_URL="${DATABASE_URL:-postgres://mvp:mvp@127.0.0.1:5432/mvp_manager?sslmode=disable}"
  export DATABASE_URL_E2E="${DATABASE_URL_E2E:-postgres://mvp:mvp@127.0.0.1:5432/mvp_manager_e2e?sslmode=disable}"

  echo "==> postgres: docker compose + migrate"
  if ! docker compose ps --status running 2>/dev/null | grep -q postgres; then
    docker compose up -d
  fi
  for i in $(seq 1 30); do
    if docker compose exec -T postgres pg_isready -U mvp -d mvp_manager >/dev/null 2>&1; then
      break
    fi
    sleep 1
    if [[ "$i" -eq 30 ]]; then
      echo "postgres не готов (docker compose up -d?)" >&2
      exit 1
    fi
  done
  ./bin/migrate up
elif [[ "$STORE_KIND" == "memory" ]]; then
  export MEMORY_STORE_PATH="${MEMORY_STORE_PATH:-$TMP/store.json}"
else
  echo "STORE=$STORE_KIND: допустимы postgres|memory" >&2
  exit 1
fi

# Job control: agent в своей process group — Ctrl+C не шлёт SIGINT ему напрямую,
# остановку делает trap → cleanup (иначе гонка с graceful shutdown / getUpdates).
set -m
./bin/agent >"$TMP/agent.log" 2>&1 &
AGENT_PID=$!
set +m
sleep 0.5

OUT="$(./bin/ctl bots create --type default --name "manual-tg-$$" --port "$PORT" \
  --token 'env:TELEGRAM_BOT_TOKEN' --mode polling --channel telegram)"
echo "$OUT"
# Формат: created bot_id=<uuid> runtime_id=... — нужен только uuid (как в e2e-*.sh).
BOT_ID="$(echo "$OUT" | sed -n 's/.*bot_id=\([^ ]*\).*/\1/p')"
if [[ -z "$BOT_ID" ]]; then
  echo "не удалось извлечь bot_id из вывода create" >&2
  exit 1
fi
./bin/ctl bots start "$BOT_ID"

# Ждём, пока agent поднимет bot-runner и actual_state=running.
echo "Ожидание actual=running…"
for i in $(seq 1 40); do
  LIST="$(./bin/ctl bots list 2>/dev/null || true)"
  if echo "$LIST" | awk -v id="$BOT_ID" '$1==id && $6=="running" {found=1} END{exit !found}'; then
    break
  fi
  if [[ "$i" -eq 40 ]]; then
    echo "бот не перешёл в running за ~20s; list:" >&2
    ./bin/ctl bots list >&2 || true
    echo "--- agent.log ---" >&2
    cat "$TMP/agent.log" >&2 || true
    exit 1
  fi
  sleep 0.5
done

# Быстрая проверка токена у Telegram (getMe).
ME="$(curl -fsS "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/getMe" || true)"
if ! echo "$ME" | grep -q '"ok":true'; then
  echo "getMe не ок — проверьте TELEGRAM_BOT_TOKEN:" >&2
  echo "$ME" >&2
  exit 1
fi
BOT_USERNAME="$(echo "$ME" | sed -n 's/.*"username":"\([^"]*\)".*/\1/p')"

echo
echo "STORE=$STORE_KIND  bot=$BOT_ID  @${BOT_USERNAME:-?}"
echo "1) Дождитесь в логе: «telegram webhook cleared», затем /start боту @${BOT_USERNAME:-bot}."
echo "   Ответ: «Привет! Сценарий default (mvp-manager) готов.»"
echo "2) При /start в логе — «telegram incoming» (ошибки — WARN getUpdates/handler)."
echo "Лог: $TMP/agent.log  |  Ctrl+C — stop бота и agent."

# tail в фоне + wait: INT надёжно ловит bash trap (не только tail).
tail -n 50 -F "$TMP/agent.log" &
TAIL_PID=$!
wait "$TAIL_PID" || true
