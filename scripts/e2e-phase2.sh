#!/usr/bin/env bash
# E2E Phase 2: 2 default webhook → один PID bot-runner; stop/start runner;
# break healthz → unhealthy → agent restore; плюс короткий custom (Phase 1).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

TMP="${TMPDIR:-/tmp}/mvp-manager-e2e2-$$"
mkdir -p "$TMP"
STORE_FILE="$TMP/store.json"
PORT1="${E2E_PORT1:-18181}"
PORT2="${E2E_PORT2:-18182}"
CUSTOM_PORT="${E2E_CUSTOM_PORT:-18183}"

AGENT_PID=""
HC_PID=""

cleanup() {
  if [[ -n "${HC_PID}" ]] && kill -0 "$HC_PID" 2>/dev/null; then
    kill -TERM "$HC_PID" 2>/dev/null || true
    wait "$HC_PID" 2>/dev/null || true
  fi
  if [[ -n "${AGENT_PID}" ]] && kill -0 "$AGENT_PID" 2>/dev/null; then
    kill -TERM "$AGENT_PID" 2>/dev/null || true
    wait "$AGENT_PID" 2>/dev/null || true
  fi
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "==> build"
mkdir -p bin
go build -o bin/agent ./cmd/agent
go build -o bin/ctl ./cmd/ctl
go build -o bin/bot-runner ./cmd/bot-runner
go build -o bin/healthcheck ./cmd/healthcheck
go build -o bin/fake-bot ./examples/fake-bot

export NODE_ID=node-e2e2
export STORE=memory
export MEMORY_STORE_PATH="$STORE_FILE"
export RECONCILE_INTERVAL=500ms
export HEARTBEAT_INTERVAL=1s
export SHUTDOWN_GRACE=2s
export BOT_RUNNER_COMMAND="$(pwd)/bin/bot-runner"
export CHECK_INTERVAL=500ms
export HTTP_TIMEOUT=1s
export FAILURE_THRESHOLD=2

echo "==> start agent"
./bin/agent >"$TMP/agent.log" 2>&1 &
AGENT_PID=$!
sleep 0.5
if ! kill -0 "$AGENT_PID" 2>/dev/null; then
  echo "agent died:"; cat "$TMP/agent.log"; exit 1
fi

echo "==> start healthcheck"
./bin/healthcheck >"$TMP/healthcheck.log" 2>&1 &
HC_PID=$!
sleep 0.3
if ! kill -0 "$HC_PID" 2>/dev/null; then
  echo "healthcheck died:"; cat "$TMP/healthcheck.log"; exit 1
fi

wait_bot_actual() {
  local id="$1" want="$2"
  local i
  for i in $(seq 1 60); do
    if ./bin/ctl bots list | awk -v id="$id" -v want="$want" '$1==id && $6==want {found=1} END{exit !found}'; then
      return 0
    fi
    sleep 0.25
  done
  return 1
}

wait_healthz() {
  local port="$1"
  local i
  for i in $(seq 1 40); do
    if curl -sf "http://127.0.0.1:${port}/healthz" | grep -q ok; then
      return 0
    fi
    sleep 0.25
  done
  return 1
}

echo "==> create 2 default webhook bots"
OUT1="$(./bin/ctl bots create --type default --name def-a --port "$PORT1" --token t-a --mode webhook)"
OUT2="$(./bin/ctl bots create --type default --name def-b --port "$PORT2" --token t-b --mode webhook)"
echo "$OUT1"
echo "$OUT2"
BOT1="$(echo "$OUT1" | sed -n 's/.*bot_id=\([^ ]*\).*/\1/p')"
BOT2="$(echo "$OUT2" | sed -n 's/.*bot_id=\([^ ]*\).*/\1/p')"
RT1="$(echo "$OUT1" | sed -n 's/.*runtime_id=\([^ ]*\).*/\1/p')"
RT2="$(echo "$OUT2" | sed -n 's/.*runtime_id=\([^ ]*\).*/\1/p')"
if [[ -z "$BOT1" || -z "$BOT2" ]]; then
  echo "parse bot_id failed"; exit 1
fi
if [[ "$RT1" != "$RT2" ]]; then
  echo "оба default должны делить один runtime: $RT1 vs $RT2"; exit 1
fi
RUNNER_RT="$RT1"

echo "==> start both defaults"
./bin/ctl bots start "$BOT1"
./bin/ctl bots start "$BOT2"

if ! wait_bot_actual "$BOT1" running; then
  echo "timeout bot1 running"; ./bin/ctl bots list; cat "$TMP/agent.log"; exit 1
fi
if ! wait_bot_actual "$BOT2" running; then
  echo "timeout bot2 running"; ./bin/ctl bots list; cat "$TMP/agent.log"; exit 1
fi

if ! wait_healthz "$PORT1"; then
  echo "healthz :$PORT1 failed"; cat "$TMP/agent.log"; exit 1
fi
if ! wait_healthz "$PORT2"; then
  echo "healthz :$PORT2 failed"; cat "$TMP/agent.log"; exit 1
fi

PID="$(./bin/ctl runtimes list | awk -v id="$RUNNER_RT" '$1==id{print $6}')"
if [[ -z "$PID" || "$PID" == "" ]]; then
  echo "нет PID runner"; ./bin/ctl runtimes list; exit 1
fi
echo "bot-runner pid=$PID"
kill -0 "$PID"

# Оба бота — один и тот же PID runtime.
echo "==> OK: 2 webhook /healthz, один PID runner"

echo "==> stop runner (desired=stopped) → PID убит"
./bin/ctl runtimes stop "$RUNNER_RT"
for i in $(seq 1 40); do
  if ! kill -0 "$PID" 2>/dev/null; then
    break
  fi
  sleep 0.25
done
if kill -0 "$PID" 2>/dev/null; then
  echo "runner pid $PID всё ещё жив после stop"; cat "$TMP/agent.log"; exit 1
fi
echo "stop runner ok"

echo "==> start runner again → инстансы живы"
./bin/ctl runtimes start "$RUNNER_RT"
if ! wait_bot_actual "$BOT1" running; then
  echo "timeout restore bot1"; ./bin/ctl bots list; cat "$TMP/agent.log"; exit 1
fi
if ! wait_healthz "$PORT1" || ! wait_healthz "$PORT2"; then
  echo "healthz after restart failed"; exit 1
fi
PID2="$(./bin/ctl runtimes list | awk -v id="$RUNNER_RT" '$1==id{print $6}')"
echo "bot-runner new pid=$PID2"
kill -0 "$PID2"

echo "==> break healthz → unhealthy → agent restore"
curl -sf -X POST "http://127.0.0.1:${PORT1}/debug/unhealthy" >/dev/null

# Ждём actual=failed от healthcheck
if ! wait_bot_actual "$BOT1" failed; then
  echo "timeout waiting unhealthy/failed"; ./bin/ctl bots list; cat "$TMP/healthcheck.log"; cat "$TMP/agent.log"; exit 1
fi
echo "unhealthy marked"

# Агент должен рестартнуть runner; /healthz снова ok, bot running
if ! wait_bot_actual "$BOT1" running; then
  echo "timeout waiting agent restore"; ./bin/ctl bots list; cat "$TMP/agent.log"; cat "$TMP/healthcheck.log"; exit 1
fi
if ! wait_healthz "$PORT1"; then
  echo "healthz not restored"; exit 1
fi
echo "restore ok"

echo "==> short custom (Phase 1 still works)"
FAKE_BOT="$(pwd)/bin/fake-bot"
COUT="$(./bin/ctl bots create \
  --type custom \
  --name e2e-custom \
  --custom-name e2e2 \
  --port "$CUSTOM_PORT" \
  --token test-token \
  --mode webhook \
  --start-command "$FAKE_BOT")"
CBOT="$(echo "$COUT" | sed -n 's/.*bot_id=\([^ ]*\).*/\1/p')"
./bin/ctl bots start "$CBOT"
if ! wait_bot_actual "$CBOT" running; then
  echo "custom not running"; ./bin/ctl bots list; cat "$TMP/agent.log"; exit 1
fi
if ! wait_healthz "$CUSTOM_PORT"; then
  echo "custom healthz failed"; exit 1
fi
./bin/ctl bots stop "$CBOT"
if ! wait_bot_actual "$CBOT" stopped; then
  echo "custom stop failed"; exit 1
fi
echo "custom ok"

echo ""
echo "E2E Phase 2 PASSED"
