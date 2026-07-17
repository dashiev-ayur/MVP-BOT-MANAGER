#!/usr/bin/env bash
# E2E Phase 1: create custom (fake-bot) → start → running → stop;
# отдельно: краш ребёнка → failed, agent жив.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

TMP="${TMPDIR:-/tmp}/mvp-manager-e2e-$$"
mkdir -p "$TMP"
STORE_FILE="$TMP/store.json"
PORT="${E2E_PORT:-18081}"

cleanup() {
  if [[ -n "${AGENT_PID:-}" ]] && kill -0 "$AGENT_PID" 2>/dev/null; then
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
go build -o bin/fake-bot ./examples/fake-bot

export NODE_ID=node-e2e
export STORE=memory
export MEMORY_STORE_PATH="$STORE_FILE"
export RECONCILE_INTERVAL=500ms
export HEARTBEAT_INTERVAL=1s
export SHUTDOWN_GRACE=2s

echo "==> start agent (store=$MEMORY_STORE_PATH)"
./bin/agent >"$TMP/agent.log" 2>&1 &
AGENT_PID=$!
sleep 0.5
if ! kill -0 "$AGENT_PID" 2>/dev/null; then
  echo "agent died:"; cat "$TMP/agent.log"; exit 1
fi

FAKE_BOT="$(pwd)/bin/fake-bot"

echo "==> bots create"
CREATE_OUT="$(./bin/ctl bots create \
  --name e2e-bot \
  --custom-name e2e \
  --port "$PORT" \
  --token test-token \
  --mode webhook \
  --channel telegram \
  --start-command "$FAKE_BOT")"
echo "$CREATE_OUT"
BOT_ID="$(echo "$CREATE_OUT" | sed -n 's/.*bot_id=\([^ ]*\).*/\1/p')"
if [[ -z "$BOT_ID" ]]; then
  echo "не удалось разобрать bot_id"; exit 1
fi

echo "==> bots start"
./bin/ctl bots start "$BOT_ID"

# tabwriter выравнивает пробелами: поля awk $1=id … $5=desired $6=actual $7=runtime
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

echo "==> wait actual=running"
if ! wait_bot_actual "$BOT_ID" running; then
  echo "timeout waiting running"; ./bin/ctl bots list; ./bin/ctl runtimes list; cat "$TMP/agent.log"; exit 1
fi

echo "==> healthz"
ok=0
for i in $(seq 1 20); do
  if curl -sf "http://127.0.0.1:${PORT}/healthz" | grep -q ok; then
    ok=1
    break
  fi
  sleep 0.25
done
if [[ "$ok" != "1" ]]; then
  echo "healthz failed on :${PORT}"; cat "$TMP/agent.log"; exit 1
fi

RT_ID="$(./bin/ctl bots list | awk -v bid="$BOT_ID" '$1==bid{print $7}')"
PID="$(./bin/ctl runtimes list | awk -v id="$RT_ID" '$1==id{print $6}')"
if [[ -z "$PID" ]]; then
  echo "нет PID в runtimes"; ./bin/ctl runtimes list; exit 1
fi
echo "child pid=$PID"
kill -0 "$PID"

echo "==> bots stop"
./bin/ctl bots stop "$BOT_ID"
if ! wait_bot_actual "$BOT_ID" stopped; then
  echo "timeout waiting stopped"; ./bin/ctl bots list; cat "$TMP/agent.log"; exit 1
fi
if kill -0 "$PID" 2>/dev/null; then
  echo "процесс $PID всё ещё жив после stop"; exit 1
fi
echo "stop ok"

echo "==> crash → failed (polling bot that exits)"
CRASH_PORT=$((PORT + 1))
CRASH_BIN="$TMP/crash.sh"
cat >"$CRASH_BIN" <<'EOF'
#!/bin/sh
exit 9
EOF
chmod +x "$CRASH_BIN"

CREATE2="$(./bin/ctl bots create \
  --name crash-bot \
  --custom-name crash \
  --port "$CRASH_PORT" \
  --token t \
  --mode polling \
  --start-command "$CRASH_BIN")"
BOT2="$(echo "$CREATE2" | sed -n 's/.*bot_id=\([^ ]*\).*/\1/p')"
./bin/ctl bots start "$BOT2"

if ! wait_bot_actual "$BOT2" failed; then
  echo "timeout waiting failed"; ./bin/ctl bots list; ./bin/ctl runtimes list; cat "$TMP/agent.log"; exit 1
fi

if ! kill -0 "$AGENT_PID" 2>/dev/null; then
  echo "agent умер после краша ребёнка"; cat "$TMP/agent.log"; exit 1
fi
echo "crash→failed ok, agent alive"

echo ""
echo "E2E Phase 1 PASSED"
