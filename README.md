# mvp-manager

Менеджер runtime ботов на ноде (Go): демон `agent`, multi-tenant `bot-runner`, отдельный `healthcheck` и CLI `ctl`.

Хранилище на текущем этапе — **memory** с общим JSON-файлом (`MEMORY_STORE_PATH`), чтобы `agent`, `ctl`, `bot-runner` и `healthcheck` видели одно состояние. PostgreSQL появится позже (Phase PG).

Подробности ТЗ, плана и процесса работы с агентами — в [`docs/`](./docs/) ([TZ](./docs/TZ.md), [план](./docs/IMPLEMENTATION_PLAN.md), [как работать](./docs/Readme.md)).

## Сборка

```bash
go build -o bin/agent ./cmd/agent
go build -o bin/ctl ./cmd/ctl
go build -o bin/bot-runner ./cmd/bot-runner
go build -o bin/healthcheck ./cmd/healthcheck
go build -o bin/fake-bot ./examples/fake-bot
```

Или:

```bash
go build ./cmd/agent ./cmd/ctl ./cmd/bot-runner ./cmd/healthcheck ./examples/fake-bot
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
| `RECONCILE_INTERVAL` | нет | `3s` | Период reconcile в agent |
| `HEARTBEAT_INTERVAL` | нет | `5s` | Период heartbeat ноды |
| `SHUTDOWN_GRACE` | нет | `10s` | SIGTERM→SIGKILL для дочерних процессов |
| `CHECK_INTERVAL` | нет | `10s` | Интервал опроса в healthcheck |
| `HTTP_TIMEOUT` | нет | `2s` | Таймаут HTTP `/healthz` |
| `FAILURE_THRESHOLD` | нет | `3` | Подряд фейлов → unhealthy в store |
| `HEALTHCHECK_ALL_NODES` | нет | false | Опрашивать все ноды, не только `NODE_ID` |
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
| `agent` | Heartbeat + reconcile: custom_bot и bot_runner через supervisor |
| `bot-runner` | Multi-tenant: N default* в **одном** OS-процессе (webhook `/healthz` / polling-stub) |
| `healthcheck` | Опрос `GET /healthz` у webhook; пишет unhealthy в store; **не** рестартует процессы |
| `ctl` | CRUD desired-состояния в том же store |

**Policy восстановления unhealthy:** healthcheck ставит `actual_state=failed` и `last_error` с префиксом `healthcheck:`; agent при следующем reconcile делает Stop+Start всего `bot_runner`. Runner заново поднимает инстансы со «здоровым» `/healthz`.

## Phase 1: custom (fake-bot)

```bash
export NODE_ID=node-1
export STORE=memory
export MEMORY_STORE_PATH=.mvp-manager/store.json
export RECONCILE_INTERVAL=1s

# терминал 1
go run ./cmd/agent

# терминал 2
go build -o bin/fake-bot ./examples/fake-bot
go run ./cmd/ctl bots create \
  --type custom \
  --name demo \
  --custom-name demo \
  --port 18080 \
  --token test-token \
  --mode webhook \
  --start-command "$(pwd)/bin/fake-bot"

go run ./cmd/ctl bots start <bot-id>
curl -s http://127.0.0.1:18080/healthz
go run ./cmd/ctl bots stop <bot-id>
```

## Phase 2: default ×2 в одном bot-runner

```bash
go build -o bin/bot-runner ./cmd/bot-runner
go build -o bin/healthcheck ./cmd/healthcheck
export BOT_RUNNER_COMMAND="$(pwd)/bin/bot-runner"

# терминал 1 — agent; терминал 2 — healthcheck (опционально для unhealthy)
go run ./cmd/agent
go run ./cmd/healthcheck

# терминал 3
go run ./cmd/ctl bots create --type default --name a --port 18081 --token t1 --mode webhook
go run ./cmd/ctl bots create --type default --name b --port 18082 --token t2 --mode webhook
go run ./cmd/ctl bots start <bot-a>
go run ./cmd/ctl bots start <bot-b>
curl -s http://127.0.0.1:18081/healthz
curl -s http://127.0.0.1:18082/healthz
go run ./cmd/ctl runtimes list   # один PID у bot-runner-*
```

Debug (E2E): `POST /debug/unhealthy` на порту бота ломает `/healthz` без kill процесса.

Help/version не требуют `NODE_ID`:

```bash
go run ./cmd/agent --help
go run ./cmd/ctl --help
go run ./cmd/bot-runner --help
go run ./cmd/healthcheck --help
```

## E2E-скрипты

```bash
chmod +x scripts/e2e-phase1.sh scripts/e2e-phase2.sh
./scripts/e2e-phase1.sh
./scripts/e2e-phase2.sh
```

- Phase 1: custom create → start → healthz → stop; краш → failed.
- Phase 2: 2 default / один PID; stop/start runner; break healthz → restore; короткий custom.

## Тесты

```bash
go test ./internal/config/...
go test ./internal/store/...
go test ./internal/store/memory/...
go test ./internal/supervisor/...
go test ./internal/reconcile/...
go test ./internal/runner/...
go test ./internal/health/...
```
