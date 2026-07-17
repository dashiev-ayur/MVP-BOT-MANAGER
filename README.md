# mvp-manager

Менеджер runtime ботов на ноде (Go): демон `agent` сверки desired/actual и CLI `ctl`.

На Phase 1 хранилище — **memory** с общим JSON-файлом (`MEMORY_STORE_PATH`), чтобы `agent` и `ctl` видели одно состояние. PostgreSQL появится позже (Phase PG).

Подробности ТЗ, плана и процесса работы с агентами — в [`docs/`](./docs/) ([TZ](./docs/TZ.md), [план](./docs/IMPLEMENTATION_PLAN.md), [как работать](./docs/Readme.md)).

## Сборка

```bash
go build -o bin/agent ./cmd/agent
go build -o bin/ctl ./cmd/ctl
go build -o bin/fake-bot ./examples/fake-bot
```

Или:

```bash
go build ./cmd/agent ./cmd/ctl ./examples/fake-bot
```

## Конфигурация (ENV)

Конфиг читается из переменных окружения (`internal/config`), файл `.env` сам по себе не подхватывается. Образец — [`.env.example`](./.env.example).

| Переменная | Обязательно | По умолчанию | Описание |
|---|---|---|---|
| `NODE_ID` | да | — | Идентификатор ноды |
| `STORE` | нет | `memory` | `memory` или `postgres` (БД — Phase PG) |
| `MEMORY_STORE_PATH` | нет* | `.mvp-manager/store.json` | Общий JSON для agent↔ctl; `""` = только RAM |
| `RECONCILE_INTERVAL` | нет | `3s` | Период reconcile в agent |
| `HEARTBEAT_INTERVAL` | нет | `5s` | Период heartbeat ноды |
| `SHUTDOWN_GRACE` | нет | `10s` | SIGTERM→SIGKILL для дочерних процессов |
| `PUBLIC_URL` | нет | — | Опционально в launch ENV бота |
| `DATABASE_URL` | нет | — | DSN Postgres (Phase PG) |

\* если переменная **не задана** — дефолтный файл; если задана пустой — persistence выключена.

```bash
export NODE_ID=node-1
export STORE=memory
export MEMORY_STORE_PATH=.mvp-manager/store.json
# или: set -a && source .env && set +a
```

## Phase 1: agent + ctl + fake-bot

`agent` регистрирует ноду, крутит heartbeat и reconcile для `kind=custom_bot`, стартует/останавливает процессы через supervisor (process group, SIGTERM→grace→SIGKILL).

`ctl` пишет desired-состояние в тот же store-файл.

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
  --name demo \
  --custom-name demo \
  --port 18080 \
  --token test-token \
  --mode webhook \
  --start-command "$(pwd)/bin/fake-bot"

go run ./cmd/ctl bots start <bot-id>
go run ./cmd/ctl bots list
go run ./cmd/ctl runtimes list
curl -s http://127.0.0.1:18080/healthz

go run ./cmd/ctl bots stop <bot-id>
```

Help/version не требуют `NODE_ID`:

```bash
go run ./cmd/agent --help
go run ./cmd/ctl --help
```

## E2E-скрипт

```bash
chmod +x scripts/e2e-phase1.sh
./scripts/e2e-phase1.sh
```

Скрипт: create → start → healthz → stop; затем краш ребёнка → `actual=failed`, agent жив.

## Тесты

```bash
go test ./internal/config/...
go test ./internal/store/...
go test ./internal/store/memory/...
go test ./internal/supervisor/...
go test ./internal/reconcile/...
```
