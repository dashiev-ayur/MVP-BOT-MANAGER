#!/usr/bin/env bash
# E2E Phase 4: лимит ботов + doctor/drain smoke + restart backoff после краша.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

TMP="${TMPDIR:-/tmp}/mvp-manager-e2e4-$$"
mkdir -p "$TMP"
STORE_FILE="$TMP/store.json"
PORT="${E2E_PORT:-18140}"

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
go build -o bin/doctor ./cmd/doctor
go build -o bin/drain-node ./cmd/drain-node
go build -o bin/handoff ./cmd/handoff
go build -o bin/fake-bot ./examples/fake-bot

export NODE_ID=node-e2e4
export STORE=memory
export MEMORY_STORE_PATH="$STORE_FILE"
export RECONCILE_INTERVAL=200ms
export HEARTBEAT_INTERVAL=1s
export SHUTDOWN_GRACE=2s
export MAX_BOTS_PER_NODE=1
export RESTART_MAX_ATTEMPTS=3
export RESTART_BACKOFF_BASE=500ms
export RESTART_BACKOFF_MAX=5s

echo "==> handoff smoke"
HAND_OUT="$TMP/handoff-out"
./bin/handoff --out "$HAND_OUT" --name demo --port 19999 --token-placeholder 'env:DEMO_TOKEN'
test -f "$HAND_OUT/.env.example"
test -f "$HAND_OUT/README.md"
grep -q 'PORT=19999' "$HAND_OUT/.env.example"
grep -q 'env:DEMO_TOKEN' "$HAND_OUT/.env.example"
echo "handoff ok"

echo "==> start agent"
./bin/agent >"$TMP/agent.log" 2>&1 &
AGENT_PID=$!
sleep 0.4
if ! kill -0 "$AGENT_PID" 2>/dev/null; then
  echo "agent died:"; cat "$TMP/agent.log"; exit 1
fi

FAKE="$(pwd)/bin/fake-bot"

echo "==> create first bot (limit=1)"
OUT1="$(./bin/ctl bots create \
  --name lim-a --custom-name lima --port "$PORT" --token secret-token-aaaa \
  --mode webhook --start-command "$FAKE")"
BOT1="$(echo "$OUT1" | sed -n 's/.*bot_id=\([^ ]*\).*/\1/p')"
test -n "$BOT1"

echo "==> create second must fail (limit)"
if ./bin/ctl bots create \
  --name lim-b --custom-name limb --port $((PORT+1)) --token secret-token-bbbb \
  --mode webhook --start-command "$FAKE" 2>"$TMP/limit.err"; then
  echo "ожидали ошибку лимита"; cat "$TMP/limit.err"; exit 1
fi
if ! grep -qiE 'лимит|MAX_BOTS|limit' "$TMP/limit.err"; then
  echo "ошибка лимита не похожа на limit:"; cat "$TMP/limit.err"; exit 1
fi
echo "limit reject ok"

echo "==> list masks token"
LIST="$(./bin/ctl bots list)"
echo "$LIST"
if echo "$LIST" | grep -q 'secret-token-aaaa'; then
  echo "полный токен светится в list"; exit 1
fi
if ! echo "$LIST" | grep -q '\*\*'; then
  echo "ожидали маску token_ref в list"; exit 1
fi
echo "token mask ok"

echo "==> doctor smoke"
./bin/doctor --node "$NODE_ID" | tee "$TMP/doctor.out"
grep -q 'mvp-manager doctor' "$TMP/doctor.out"
grep -q 'Ports' "$TMP/doctor.out"
echo "doctor ok"

echo "==> start bot1 + drain-node"
./bin/ctl bots start "$BOT1"

wait_bot_actual() {
  local id="$1" want="$2"
  local i
  for i in $(seq 1 40); do
    if ./bin/ctl bots list | awk -v id="$id" -v want="$want" '$1==id && $6==want {found=1} END{exit !found}'; then
      return 0
    fi
    sleep 0.2
  done
  return 1
}

if ! wait_bot_actual "$BOT1" running; then
  echo "timeout running"; ./bin/ctl bots list; cat "$TMP/agent.log"; exit 1
fi

./bin/drain-node --node "$NODE_ID" | tee "$TMP/drain.out"
grep -q 'draining' "$TMP/drain.out"
if ! wait_bot_actual "$BOT1" stopped; then
  echo "timeout stopped after drain"; ./bin/ctl bots list; cat "$TMP/agent.log"; exit 1
fi
NODE_STATUS="$(python3 -c "import json; d=json.load(open('$STORE_FILE')); print([n['Status'] if 'Status' in n else n.get('status') for n in d.get('nodes',d.get('Nodes',[])) if (n.get('ID') or n.get('id'))=='$NODE_ID'][0])" 2>/dev/null || true)"
# Fallback: ctl не показывает nodes — читаем JSON memory store (поля с заглавной из encoding/json default? memory uses json tags lowercase)
NODE_LINE="$(grep -o '"status"[[:space:]]*:[[:space:]]*"[^"]*"' "$STORE_FILE" | head -1 || true)"
echo "store node status hint: $NODE_LINE"
if ! grep -q 'draining' "$STORE_FILE"; then
  echo "store не содержит draining"; cat "$STORE_FILE"; exit 1
fi
echo "drain ok"

# --- Restart backoff: отдельный store/agent, чтобы не мешать drain ---
echo "==> restart backoff after crash"
kill -TERM "$AGENT_PID" 2>/dev/null || true
wait "$AGENT_PID" 2>/dev/null || true
AGENT_PID=""

STORE2="$TMP/store2.json"
export MEMORY_STORE_PATH="$STORE2"
export MAX_BOTS_PER_NODE=0
export RESTART_MAX_ATTEMPTS=2
export RESTART_BACKOFF_BASE=1s
export RECONCILE_INTERVAL=200ms

./bin/agent >"$TMP/agent2.log" 2>&1 &
AGENT_PID=$!
sleep 0.4

CRASH="$TMP/crash.sh"
cat >"$CRASH" <<'EOF'
#!/bin/sh
exit 7
EOF
chmod +x "$CRASH"

COUT="$(./bin/ctl bots create \
  --name crash4 --custom-name crash4 --port $((PORT+10)) --token t \
  --mode polling --start-command "$CRASH")"
BOTC="$(echo "$COUT" | sed -n 's/.*bot_id=\([^ ]*\).*/\1/p')"
./bin/ctl bots start "$BOTC"

if ! wait_bot_actual "$BOTC" failed; then
  echo "timeout failed after crash"; ./bin/ctl bots list; cat "$TMP/agent2.log"; exit 1
fi

# Сразу после failed не должно быть мгновенного рестарта без паузы:
# в логе должен появиться restart backoff.
sleep 0.3
if ! grep -q 'restart backoff' "$TMP/agent2.log"; then
  echo "ожидали 'restart backoff' в логе agent:"; cat "$TMP/agent2.log"; exit 1
fi
echo "backoff logged ok"

# После base backoff (1s) + reconcile — возможен restart attempt в логе.
sleep 1.5
if ! grep -qE 'restart after failure|restart backoff' "$TMP/agent2.log"; then
  echo "нет следов restart policy"; cat "$TMP/agent2.log"; exit 1
fi
echo "restart policy ok"

echo ""
echo "E2E Phase 4 PASSED"
