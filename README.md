# mvp-manager

Менеджер runtime ботов на ноде (Go): демон `agent`, multi-tenant `bot-runner`, отдельный `healthcheck`, CLI `ctl` и HTTP `control-api`.

Хранилище на текущем этапе — **memory** с общим JSON-файлом (`MEMORY_STORE_PATH`), чтобы `agent`, `ctl`, `bot-runner`, `healthcheck` и `control-api` видели одно состояние. PostgreSQL появится позже (Phase PG).

Подробности ТЗ, плана и процесса работы с агентами — в [`docs/`](./docs/) ([TZ](./docs/TZ.md), [план](./docs/IMPLEMENTATION_PLAN.md), [как работать](./docs/Readme.md)). Handoff клиенту (single-bot) — [`docs/handoff/`](./docs/handoff/).

## Сборка

```bash
go build -o bin/agent ./cmd/agent
go build -o bin/ctl ./cmd/ctl
go build -o bin/bot-runner ./cmd/bot-runner
go build -o bin/healthcheck ./cmd/healthcheck
go build -o bin/control-api ./cmd/control-api
go build -o bin/fake-bot ./examples/fake-bot
```

Или:

```bash
go build ./cmd/...
```

## Конфигурация (ENV)

Конфиг читается из переменных окружения (`internal/config`), файл `.env` сам по себе не подхватывается. Образец — [`.env.example`](./.env.example).

| Переменная | Обязательно | По умолчанию | Описание |
|---|---|---|---|
| `NODE_ID` | да | — | Идентификатор ноды |
| `STORE` | нет | `memory` | `memory` или `postgres` (БД — Phase PG) |
| `MEMORY_STORE_PATH` | нет* | `.mvp-manager/store.json` | Общий JSON для всех процессов; `""` = только RAM |
| `BOT_RUNNER_COMMAND` | для default* | — | Команда запуска `bot-runner` (агент) |
| `BOT_RUNNER_WORKDIR` | нет | — | Workdir процесса runner |
| `BOT_RUNNER_HEALTH_PORT` | нет | — | Служебный `/healthz` самого runner |
| `RUNTIME_ID` | у runner | — | Какой runtime обслуживает процесс (прокидывает agent) |
| `LEASE_TTL` | нет | `15s` | TTL lease на runtime (Acquire/Renew) |
| `RECONCILE_INTERVAL` | нет | `3s` | Период reconcile в agent |
| `HEARTBEAT_INTERVAL` | нет | `5s` | Период heartbeat ноды |
| `SHUTDOWN_GRACE` | нет | `10s` | SIGTERM→SIGKILL для дочерних процессов |
| `CHECK_INTERVAL` | нет | `10s` | Интервал опроса в healthcheck |
| `HTTP_TIMEOUT` | нет | `2s` | Таймаут HTTP `/healthz` |
| `FAILURE_THRESHOLD` | нет | `3` | Подряд фейлов → unhealthy в store |
| `HEALTHCHECK_ALL_NODES` | нет | false | Опрашивать все ноды, не только `NODE_ID` |
| `API_ADDR` | нет | `127.0.0.1:8080` | Bind `control-api` |
| `CONTROL_API_TOKEN` | для API | — | Bearer-токен; без него `/v1/*` → 401 |
| `PUBLIC_URL` | нет | — | Опционально в launch ENV custom-бота |
| `DATABASE_URL` | нет | — | DSN Postgres (Phase PG) |

\* если переменная **не задана** — дефолтный файл; если задана пустой — persistence выключена.

```bash
export NODE_ID=node-1
export STORE=memory
export MEMORY_STORE_PATH=.mvp-manager/store.json
export BOT_RUNNER_COMMAND="$(pwd)/bin/bot-runner"
# или: set -a && source .env && set +a
```

## Компоненты

| Бинарник | Роль |
|---|---|
| `agent` | Heartbeat + reconcile + lease: custom_bot и bot_runner через supervisor |
| `bot-runner` | Multi-tenant: N default* в **одном** OS-процессе; сценарии `default` / `default_extended` (registry) |
| `healthcheck` | Опрос `GET /healthz` у webhook; пишет unhealthy в store; **не** рестартует процессы |
| `ctl` | CRUD desired-состояния; `bots migrate --to-node` |
| `control-api` | HTTP API (ТЗ §11) над тем же store |

**Lease:** старт OS-процесса только после успешного `Acquire` текущим `NODE_ID`; `Renew` в цикле reconcile; чужой lease → не Start / Stop локального процесса.

**Policy восстановления unhealthy:** healthcheck ставит `actual_state=failed` и `last_error` с префиксом `healthcheck:`; agent при следующем reconcile делает Stop+Start всего `bot_runner`. Runner заново поднимает инстансы со «здоровым» `/healthz`.

### token_ref

Поле `bots.token_ref` резолвится так (`internal/launch.ResolveTokenRef`):

| Значение | Результат |
|---|---|
| обычная строка | используется как токен напрямую (MVP / `ctl --token`) |
| `env:NAME` | `os.Getenv("NAME")` |
| `$NAME` | то же |

Полный токен в slog не пишется (только `TokenHint`: длина / хвост).

### Мессенджеры

- **Telegram** (`channel=telegram`): тонкий `net/http` к Bot API (`getUpdates` / `sendMessage`). Режимы:
  - `polling` — long poll, ответ на `/start`;
  - `webhook` — `GET /healthz` + приём `POST /` или `POST /webhook` с JSON Update. Исходящий `setWebhook` к Telegram **не** вызывается без публичного HTTPS `PUBLIC_URL`.
- **Max** (`channel=max`): HTTP к `https://platform-api2.max.ru`. На `/start` и `bot_started` — приветствие сценария.

Сценарий `default_extended`: другой текст на `/start` и команда `/ping` → `pong` (см. `internal/runner/scenarios`).

#### Ручная проверка с живым Telegram

```bash
export TELEGRAM_BOT_TOKEN="123456:ABC-DEF"
export NODE_ID=node-1 STORE=memory MEMORY_STORE_PATH=.mvp-manager/store.json
export BOT_RUNNER_COMMAND="$(pwd)/bin/bot-runner"
go run ./cmd/agent
go run ./cmd/ctl bots create --type default --name tg --port 18090 \
  --token 'env:TELEGRAM_BOT_TOKEN' --mode polling --channel telegram
go run ./cmd/ctl bots start <bot-id>
```

## Phase 1: custom (fake-bot)

```bash
export NODE_ID=node-1 STORE=memory MEMORY_STORE_PATH=.mvp-manager/store.json
go run ./cmd/agent
go build -o bin/fake-bot ./examples/fake-bot
go run ./cmd/ctl bots create --type custom --name demo --custom-name demo \
  --port 18080 --token test-token --mode webhook \
  --start-command "$(pwd)/bin/fake-bot"
go run ./cmd/ctl bots start <bot-id>
```

## Phase 2: default ×2 в одном bot-runner

```bash
export BOT_RUNNER_COMMAND="$(pwd)/bin/bot-runner"
go run ./cmd/agent
go run ./cmd/ctl bots create --type default --name a --port 18081 --token t1 --mode webhook
go run ./cmd/ctl bots create --type default --name b --port 18082 --token t2 --mode webhook
go run ./cmd/ctl bots start <bot-a>
go run ./cmd/ctl bots start <bot-b>
```

## Phase 3: migrate + control-api

```bash
export MEMORY_STORE_PATH=.mvp-manager/store.json BOT_RUNNER_COMMAND="$(pwd)/bin/bot-runner"
NODE_ID=node-a go run ./cmd/agent   # терминал 1
NODE_ID=node-b go run ./cmd/agent   # терминал 2
NODE_ID=node-a go run ./cmd/ctl bots migrate <bot-id> --to-node node-b

export CONTROL_API_TOKEN=secret API_ADDR=127.0.0.1:8080 NODE_ID=node-a
go run ./cmd/control-api
curl -s http://127.0.0.1:8080/healthz
curl -s -H "Authorization: Bearer secret" http://127.0.0.1:8080/v1/bots
```

## E2E-скрипты

```bash
chmod +x scripts/e2e-phase1.sh scripts/e2e-phase2.sh scripts/e2e-phase3.sh
./scripts/e2e-phase1.sh
./scripts/e2e-phase2.sh
./scripts/e2e-phase3.sh
```

## Тесты

```bash
go test ./internal/...
```
