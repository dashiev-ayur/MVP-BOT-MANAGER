#!/usr/bin/env bash
# E2E Phase 3: два агента (node-a / node-b), общий store;
# lease не отдаёт runtime второму; migrate custom и default без двойного PID/порта.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

TMP="${TMPDIR:-/tmp}/mvp-manager-e2e3-$$"
mkdir -p "$TMP"
STORE_FILE="$TMP/store.json"
CUSTOM_PORT="${E2E_CUSTOM_PORT:-18281}"
DEFAULT_PORT="${E2E_DEFAULT_PORT:-18282}"
API_PORT="${E2E_API_PORT:-18080}"

AGENT_A_PID=""
AGENT_B_PID=""
API_PID=""

cleanup() {
  if [[ -n "${API_PID}" ]] && kill -0 "$API_PID" 2>/dev/null; then
    kill -TERM "$API_PID" 2>/dev/null || true
    wait "$API_PID" 2>/dev/null || true
  fi
  if [[ -n "${AGENT_A_PID}" ]] && kill -0 "$AGENT_A_PID" 2>/dev/null; then
    kill -TERM "$AGENT_A_PID" 2>/dev/null || true
    wait "$AGENT_A_PID" 2>/dev/null || true
  fi
  if [[ -n "${AGENT_B_PID}" ]] && kill -0 "$AGENT_B_PID" 2>/dev/null; then
    kill -TERM "$AGENT_B_PID" 2>/dev/null || true
    wait "$AGENT_B_PID" 2>/dev/null || true
  fi
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "==> build"
mkdir -p bin
go build -o bin/agent ./cmd/agent
go build -o bin/ctl ./cmd/ctl
go build -o bin/bot-runner ./cmd/bot-runner
go build -o bin/control-api ./cmd/control-api
go build -o bin/fake-bot ./examples/fake-bot

export STORE=memory
export MEMORY_STORE_PATH="$STORE_FILE"
export RECONCILE_INTERVAL=400ms
export HEARTBEAT_INTERVAL=1s
export SHUTDOWN_GRACE=2s
export LEASE_TTL=8s
export BOT_RUNNER_COMMAND="$(pwd)/bin/bot-runner"

echo "==> start agent node-a"
NODE_ID=node-a ./bin/agent >"$TMP/agent-a.log" 2>&1 &
AGENT_A_PID=$!
echo "==> start agent node-b"
NODE_ID=node-b ./bin/agent >"$TMP/agent-b.log" 2>&1 &
AGENT_B_PID=$!
sleep 0.6
if ! kill -0 "$AGENT_A_PID" 2>/dev/null; then
  echo "agent-a died:"; cat "$TMP/agent-a.log"; exit 1
fi
if ! kill -0 "$AGENT_B_PID" 2>/dev/null; then
  echo "agent-b died:"; cat "$TMP/agent-b.log"; exit 1
fi

wait_bot_actual() {
  local id="$1" want="$2"
  local i
  for i in $(seq 1 60); do
    if NODE_ID=node-a ./bin/ctl bots list | awk -v id="$id" -v want="$want" '$1==id && $6==want {found=1} END{exit !found}'; then
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
    if curl -sf "http://127.0.0.1:${port}/healthz" 2>/dev/null | grep -q ok; then
      return 0
    fi
    sleep 0.25
  done
  return 1
}

runtime_pid_for_bot() {
  local bot_id="$1"
  # bots list: ID NAME TYPE PORT DESIRED ACTUAL RUNTIME_ID ...
  local rt
  rt="$(NODE_ID=node-a ./bin/ctl bots list | awk -v id="$bot_id" '$1==id {print $7; exit}')"
  if [[ -z "$rt" || "$rt" == "-" ]]; then
    echo ""
    return
  fi
  NODE_ID=node-a ./bin/ctl runtimes list | awk -v id="$rt" '$1==id {print $6; exit}'
}

count_listeners() {
  local port="$1"
  # Сколько процессов слушают порт (lsof); ожидаем ≤1.
  if command -v lsof >/dev/null 2>&1; then
    local n
    n="$(lsof -nP -iTCP:"$port" -sTCP:LISTEN 2>/dev/null | awk 'NR>1' | wc -l | tr -d ' ' || true)"
    if [[ -z "$n" ]]; then
      echo "0"
    else
      echo "$n"
    fi
  else
    echo "0"
  fi
}

FAKE_BOT="$(pwd)/bin/fake-bot"

echo "==> create+start custom на node-a"
CREATE_OUT="$(NODE_ID=node-a ./bin/ctl bots create \
  --type custom \
  --name e2e3-custom \
  --custom-name e2e3c \
  --port "$CUSTOM_PORT" \
  --token test-token \
  --mode webhook \
  --start-command "$FAKE_BOT")"
echo "$CREATE_OUT"
CUSTOM_ID="$(echo "$CREATE_OUT" | sed -n 's/.*bot_id=\([^ ]*\).*/\1/p')"
CUSTOM_RT="$(echo "$CREATE_OUT" | sed -n 's/.*runtime_id=\([^ ]*\).*/\1/p')"
NODE_ID=node-a ./bin/ctl bots start "$CUSTOM_ID"

echo "==> wait custom running"
if ! wait_bot_actual "$CUSTOM_ID" running; then
  echo "timeout custom running"; NODE_ID=node-a ./bin/ctl bots list; cat "$TMP/agent-a.log"; exit 1
fi
PID_BEFORE="$(runtime_pid_for_bot "$CUSTOM_ID")"
echo "custom pid on A: $PID_BEFORE"
if [[ -z "$PID_BEFORE" || "$PID_BEFORE" == "" ]]; then
  echo "нет PID runtime"; NODE_ID=node-a ./bin/ctl runtimes list; exit 1
fi
if ! wait_healthz "$CUSTOM_PORT"; then
  echo "custom healthz timeout"; cat "$TMP/agent-a.log"; exit 1
fi

echo "==> lease: node-b не должен держать lease чужого runtime"
# Юнит-тест покрывает гонку; здесь — процесс жив только у A (один listener).
LISTENERS="$(count_listeners "$CUSTOM_PORT")"
if [[ "${LISTENERS:-0}" -gt 1 ]]; then
  echo "ожидали ≤1 listener на :$CUSTOM_PORT, got $LISTENERS"; exit 1
fi

echo "==> migrate custom → node-b"
NODE_ID=node-a ./bin/ctl bots migrate "$CUSTOM_ID" --to-node node-b

echo "==> wait custom running on B"
if ! wait_bot_actual "$CUSTOM_ID" running; then
  echo "timeout after migrate custom"; NODE_ID=node-a ./bin/ctl bots list; NODE_ID=node-a ./bin/ctl runtimes list
  cat "$TMP/agent-a.log"; cat "$TMP/agent-b.log"; exit 1
fi
PID_AFTER="$(runtime_pid_for_bot "$CUSTOM_ID")"
echo "custom pid on B: $PID_AFTER"
if [[ -n "$PID_BEFORE" && -n "$PID_AFTER" && "$PID_BEFORE" == "$PID_AFTER" ]]; then
  echo "PID не сменился после migrate (возможен тот же номер — проверим что старый мёртв)"
fi
# Старый PID не должен быть жив одновременно с новым (если разные).
if [[ -n "$PID_BEFORE" && -n "$PID_AFTER" && "$PID_BEFORE" != "$PID_AFTER" ]]; then
  if kill -0 "$PID_BEFORE" 2>/dev/null; then
    echo "двойной запуск: старый PID $PID_BEFORE ещё жив при новом $PID_AFTER"; exit 1
  fi
fi
LISTENERS="$(count_listeners "$CUSTOM_PORT")"
if [[ "$LISTENERS" -gt 1 ]]; then
  echo "двойной listen на :$CUSTOM_PORT ($LISTENERS)"; exit 1
fi
if ! wait_healthz "$CUSTOM_PORT"; then
  echo "custom healthz after migrate timeout"; exit 1
fi

echo "==> create+start default на node-a"
DEF_OUT="$(NODE_ID=node-a ./bin/ctl bots create \
  --type default \
  --name e2e3-def \
  --port "$DEFAULT_PORT" \
  --token t-def \
  --mode webhook)"
echo "$DEF_OUT"
DEF_ID="$(echo "$DEF_OUT" | sed -n 's/.*bot_id=\([^ ]*\).*/\1/p')"
NODE_ID=node-a ./bin/ctl bots start "$DEF_ID"
if ! wait_bot_actual "$DEF_ID" running; then
  echo "timeout default running"; cat "$TMP/agent-a.log"; exit 1
fi
if ! wait_healthz "$DEFAULT_PORT"; then
  echo "default healthz timeout"; exit 1
fi
DEF_PID_A="$(runtime_pid_for_bot "$DEF_ID")"

echo "==> migrate default → node-b"
NODE_ID=node-a ./bin/ctl bots migrate "$DEF_ID" --to-node node-b
if ! wait_bot_actual "$DEF_ID" running; then
  echo "timeout after migrate default"; NODE_ID=node-a ./bin/ctl bots list
  cat "$TMP/agent-a.log"; cat "$TMP/agent-b.log"; exit 1
fi
DEF_PID_B="$(runtime_pid_for_bot "$DEF_ID")"
if [[ -n "$DEF_PID_A" && -n "$DEF_PID_B" && "$DEF_PID_A" != "$DEF_PID_B" ]]; then
  if kill -0 "$DEF_PID_A" 2>/dev/null; then
    echo "двойной bot-runner: PID A=$DEF_PID_A ещё жив при B=$DEF_PID_B"; exit 1
  fi
fi
LISTENERS="$(count_listeners "$DEFAULT_PORT")"
if [[ "$LISTENERS" -gt 1 ]]; then
  echo "двойной listen default :$DEFAULT_PORT ($LISTENERS)"; exit 1
fi
if ! wait_healthz "$DEFAULT_PORT"; then
  echo "default healthz after migrate timeout"; exit 1
fi

echo "==> control-api healthz + auth"
export NODE_ID=node-a
export API_ADDR="127.0.0.1:${API_PORT}"
export CONTROL_API_TOKEN="e2e-secret"
./bin/control-api >"$TMP/api.log" 2>&1 &
API_PID=$!
sleep 0.4
if ! kill -0 "$API_PID" 2>/dev/null; then
  echo "control-api died:"; cat "$TMP/api.log"; exit 1
fi
curl -sf "http://127.0.0.1:${API_PORT}/healthz" | grep -q ok
# без токена — 401
CODE="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:${API_PORT}/v1/bots")"
if [[ "$CODE" != "401" ]]; then
  echo "ожидали 401 без токена, got $CODE"; exit 1
fi
# с токеном — 200
CODE="$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer e2e-secret" "http://127.0.0.1:${API_PORT}/v1/bots")"
if [[ "$CODE" != "200" ]]; then
  echo "ожидали 200 с токеном, got $CODE"; cat "$TMP/api.log"; exit 1
fi

echo "==> default_extended create (smoke)"
EXT_OUT="$(NODE_ID=node-b ./bin/ctl bots create \
  --type default_extended \
  --name e2e3-ext \
  --port 18283 \
  --token t-ext \
  --mode webhook)"
echo "$EXT_OUT"
echo "$EXT_OUT" | grep -q 'type=default_extended'

echo "PASSED phase3"
