# mvp-manager

Менеджер runtime ботов на ноде (Go): демон `agent`, multi-tenant `bot-runner`, отдельный `healthcheck`, CLI `ctl` и HTTP `control-api`.

Хранилище выбирается конфигом: **`STORE=postgres`** (Docker Compose + goose; так в `.env.example`) или **`STORE=memory`** (JSON-файл; memory e2e задают явно). Бизнес-логика (reconcile/supervisor/runner/ops) от драйвера БД не зависит.

Подробности ТЗ, плана и процесса работы с агентами — в [`docs/`](./docs/) ([TZ](./docs/TZ.md), [план](./docs/IMPLEMENTATION_PLAN.md), [как работать](./docs/Readme.md), [frontend / UI](./docs/frontend.md)). Handoff клиенту (single-bot) — [`docs/handoff/`](./docs/handoff/).

**UI:** каталог [`web/`](./web/) в этом репозитории (monorepo). Клиент ходит только в `control-api`. Подробное ТЗ — [`docs/frontend.md`](./docs/frontend.md).

## Сборка

```bash
go build -o bin/agent ./cmd/agent
go build -o bin/ctl ./cmd/ctl
go build -o bin/bot-runner ./cmd/bot-runner
go build -o bin/healthcheck ./cmd/healthcheck
go build -o bin/control-api ./cmd/control-api
go build -o bin/doctor ./cmd/doctor
go build -o bin/drain-node ./cmd/drain-node
go build -o bin/handoff ./cmd/handoff
go build -o bin/migrate ./cmd/migrate
go build -o bin/fake-bot ./examples/fake-bot
```

Или: `go build ./cmd/...`

## Конфигурация (ENV)

Конфиг читается из переменных окружения (`internal/config`), файл `.env` сам по себе не подхватывается. Образец — [`.env.example`](./.env.example).

| Переменная | Обязательно | По умолчанию | Описание |
|---|---|---|---|
| `NODE_ID` | да | — | Идентификатор ноды |
| `STORE` | нет | `memory` (Go); в `.env.example` — `postgres` | `memory` или `postgres` |
| `MEMORY_STORE_PATH` | нет* | `.mvp-manager/store.json` | Общий JSON для всех процессов; `""` = только RAM |
| `POSTGRES_USER` / `PASSWORD` / `DB` / `DB_E2E` / `PORT` | для compose | `mvp` / `mvp` / `mvp_manager` / `mvp_manager_e2e` / `5432` | Credentials Docker; должны совпадать с DSN |
| `DATABASE_URL` | при postgres | — | DSN рабочей БД `mvp_manager` |
| `DATABASE_URL_E2E` | для e2e/migrate --e2e | — | DSN БД `mvp_manager_e2e` (тот же хост/порт) |
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
| `RESTART_MAX_ATTEMPTS` | нет | `5` | Авто-рестарты после crash/unhealthy; `0` = выкл. |
| `RESTART_BACKOFF_BASE` | нет | `1s` | Начальная пауза экспоненциального backoff |
| `RESTART_BACKOFF_MAX` | нет | `60s` | Потолок backoff |
| `MAX_BOTS_PER_NODE` | нет | `0` | Лимит ботов на ноду; `0` = без лимита |
| `PUBLIC_URL` | нет | — | Опционально в launch ENV custom-бота |

\* если переменная **не задана** — дефолтный файл; если задана пустой — persistence выключена.

## PostgreSQL (Phase PG)

Две базы в одном контейнере (не schema): `mvp_manager` (dev) и `mvp_manager_e2e` (тесты).

```bash
# 1) БД
docker compose up -d

# 2) схема (dev)
export DATABASE_URL='postgres://mvp:mvp@127.0.0.1:5432/mvp_manager?sslmode=disable'
export DATABASE_URL_E2E='postgres://mvp:mvp@127.0.0.1:5432/mvp_manager_e2e?sslmode=disable'
go run ./cmd/migrate up
# e2e: go run ./cmd/migrate --e2e up

# 3) сиды вручную (только dev по умолчанию; идемпотентно)
go run ./cmd/migrate seed

# 4) приложение
export STORE=postgres NODE_ID=node-1
export BOT_RUNNER_COMMAND="$(pwd)/bin/bot-runner"
./bin/agent
./bin/ctl bots list   # видны 3 seed default-бота
```

### Сиды (стабильные UUID)

| Роль | UUID |
|---|---|
| Клиент A | `11111111-1111-4111-8111-111111111111` — **2** бота `default` (порты 19001, 19002) |
| Клиент B | `22222222-2222-4222-8222-222222222222` — **1** бот `default` (порт 19003) |
| Runtime | `33333333-3333-4333-8333-333333333333` (`bot-runner-node-1`) |
| Нода | `node-1` (как `NODE_ID` в `.env.example`) |

`desired_state=stopped`, `token_ref=seed:…` — без живого Telegram до `ctl bots start`.

Миграции и сиды **не** выполняются при `compose up`.

## Memory

Для локальной работы без Docker задайте `STORE=memory` явно (или не делайте `source .env` с postgres-defaults из `.env.example`). Memory e2e-скрипты уже экспортируют `STORE=memory`.

```bash
export NODE_ID=node-1
export STORE=memory
export MEMORY_STORE_PATH=.mvp-manager/store.json
export BOT_RUNNER_COMMAND="$(pwd)/bin/bot-runner"
```

## Компоненты

| Бинарник | Роль |
|---|---|
| `agent` | Heartbeat + reconcile + lease + restart/backoff; file-watch (memory) / LISTEN/NOTIFY (postgres) |
| `bot-runner` | Multi-tenant: N default* в **одном** OS-процессе |
| `healthcheck` | Опрос `GET /healthz` у webhook; пишет unhealthy; **не** рестартует |
| `ctl` | CRUD desired-состояния; `bots migrate --to-node` |
| `control-api` | HTTP API (ТЗ §11); `GET /metrics` |
| `migrate` | goose up/down/status + ручной `seed` |
| `doctor` / `drain-node` / `handoff` | диагностика / drain / упаковка handoff |

**Lease:** старт OS-процесса только после успешного `Acquire` текущим `NODE_ID`.

**Store wake:** poll — safety net; memory + файл → mtime; postgres → `LISTEN/NOTIFY` (`bot_changes` / `runtime_changes`).

### token_ref

| Значение | Результат |
|---|---|
| обычная строка | токен напрямую |
| `env:NAME` / `$NAME` | `os.Getenv("NAME")` |

### Multi-runner sharding

Вне текущего MVP. Сейчас один `bot_runner` на `NODE_ID`.

### TODO: unhealthy → reload одного инстанса

Сейчас: healthcheck помечает webhook unhealthy → agent делает Stop+Start **всего** `bot_runner` (все default в процессе кратко гаснут).  
Желательно: перезапускать **только** проблемный инстанс внутри runner; полный рестарт runtime — при краше PID или эскалации. См. backlog в [`docs/IMPLEMENTATION_PLAN.md`](./docs/IMPLEMENTATION_PLAN.md).

## E2E

```bash
./scripts/e2e-phase1.sh          # memory
./scripts/e2e-phase2.sh
./scripts/e2e-phase3.sh
./scripts/e2e-phase4.sh
./scripts/e2e-phase-pg.sh        # Postgres, DATABASE_URL_E2E
```

## Тесты

```bash
go test ./internal/...
# postgres unit (compose + migrate --e2e up):
DATABASE_URL_E2E='postgres://mvp:mvp@127.0.0.1:5432/mvp_manager_e2e?sslmode=disable' \
  go test ./internal/store/postgres/...
```
