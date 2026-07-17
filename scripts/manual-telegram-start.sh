#!/usr/bin/env bash
# Ручная проверка живого Telegram-бота (не для CI).
# Требует: TELEGRAM_BOT_TOKEN от @BotFather, собранные bin/*, свободный порт.
#
#   export TELEGRAM_BOT_TOKEN=123456:ABC
#   ./scripts/manual-telegram-start.sh
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
STORE_FILE="$TMP/store.json"

cleanup() {
  if [[ -n "${AGENT_PID:-}" ]] && kill -0 "$AGENT_PID" 2>/dev/null; then
    kill -TERM "$AGENT_PID" 2>/dev/null || true
    wait "$AGENT_PID" 2>/dev/null || true
  fi
  rm -rf "$TMP"
}
trap cleanup EXIT

go build -o bin/agent ./cmd/agent
go build -o bin/ctl ./cmd/ctl
go build -o bin/bot-runner ./cmd/bot-runner

export NODE_ID=node-manual-tg
export STORE=memory
export MEMORY_STORE_PATH="$STORE_FILE"
export RECONCILE_INTERVAL=500ms
export BOT_RUNNER_COMMAND="$(pwd)/bin/bot-runner"

./bin/agent >"$TMP/agent.log" 2>&1 &
AGENT_PID=$!
sleep 0.5

OUT="$(./bin/ctl bots create --type default --name manual-tg --port "$PORT" \
  --token 'env:TELEGRAM_BOT_TOKEN' --mode polling --channel telegram)"
echo "$OUT"
BOT_ID="$(echo "$OUT" | awk '/^created/{print $2}')"
./bin/ctl bots start "$BOT_ID"

echo
echo "Бот $BOT_ID в режиме polling на резерве порта $PORT."
echo "Напишите /start вашему боту в Telegram — ожидайте приветствие."
echo "Лог agent: $TMP/agent.log"
echo "Ctrl+C — выход (agent будет остановлен)."
tail -f "$TMP/agent.log"
