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
go build -o bin/doctor ./cmd/doctor
go build -o bin/drain-node ./cmd/drain-node
go build -o bin/handoff ./cmd/handoff
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
| `RESTART_MAX_ATTEMPTS` | нет | `5` | Авто-рестарты после crash/unhealthy; `0` = выкл. |
| `RESTART_BACKOFF_BASE` | нет | `1s` | Начальная пауза экспоненциального backoff |
| `RESTART_BACKOFF_MAX` | нет | `60s` | Потолок backoff |
| `MAX_BOTS_PER_NODE` | нет | `0` | Лимит ботов на ноду; `0` = без лимита |
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
| `agent` | Heartbeat + reconcile + lease + restart/backoff; file-watch wake (memory) |
| `bot-runner` | Multi-tenant: N default* в **одном** OS-процессе; сценарии `default` / `default_extended` (registry) |
| `healthcheck` | Опрос `GET /healthz` у webhook; пишет unhealthy в store; **не** рестартует процессы |
| `ctl` | CRUD desired-состояния; `bots migrate --to-node`; лимит `MAX_BOTS_PER_NODE` |
| `control-api` | HTTP API (ТЗ §11) над тем же store; `GET /metrics` |
| `doctor` | Сверка портов / PID / lease / listen (отчёт в stdout) |
| `drain-node` | `status=draining` + stop ботов или migrate на `--to-node` |
| `handoff` | Упаковка `docs/handoff` → каталог выдачи клиенту |

**Lease:** старт OS-процесса только после успешного `Acquire` текущим `NODE_ID`; `Renew` в цикле reconcile; чужой lease → не Start / Stop локального процесса.

**Store wake (Phase 4):** периодический poll остаётся safety net; при `STORE=memory` + `MEMORY_STORE_PATH` agent следит за mtime файла (`internal/watch.ChangeWatcher`) и будит reconcile раньше интервала. Для `STORE=postgres` точка расширения — `LISTEN/NOTIFY` (Phase PG), сейчас nop.

**Restart policy:** после crash custom / bot_runner (и unhealthy runner) — экспоненциальный backoff (`RESTART_*`); сброс после успешного `running`.

**Лимит ботов:** `MAX_BOTS_PER_NODE` — create/start (ctl и control-api) отклоняют превышение; reconcile не стартует сверх лимита.

**Метрики:** slog с attr `metric=…` и `GET /metrics` на control-api (простой text).

**Policy восстановления unhealthy:** healthcheck ставит `actual_state=failed` и `last_error` с префиксом `healthcheck:`; agent при следующем reconcile (после backoff) делает Stop+Start всего `bot_runner`.

### token_ref

Поле `bots.token_ref` резолвится так (`internal/launch.ResolveTokenRef`):

| Значение | Результат |
|---|---|
| обычная строка | используется как токен напрямую (MVP / `ctl --token`) |
| `env:NAME` | `os.Getenv("NAME")` |
| `$NAME` | то же |

Полный токен в slog не пишется (`TokenHint`: только длина). В `ctl bots list` и HTTP API `token_ref` маскируется (`MaskTokenRef`); plaintext → warn в лог. Предпочтительно `env:NAME`.

### Multi-runner sharding

Несколько runner’ов на ноду / шардирование — **вне** текущего Phase 4 (TODO на будущее). Сейчас один `bot_runner` на `NODE_ID`.

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

## Phase 4: hardening

```bash
# лимит / doctor / drain / handoff / backoff — см. скрипт
./scripts/e2e-phase4.sh

go run ./cmd/doctor
go run ./cmd/drain-node --node node-1
go run ./cmd/drain-node --node node-1 --to-node node-2
go run ./cmd/handoff --out /tmp/out --name demo --port 18080
```

## E2E-скрипты

```bash
chmod +x scripts/e2e-phase1.sh scripts/e2e-phase2.sh scripts/e2e-phase3.sh scripts/e2e-phase4.sh
./scripts/e2e-phase1.sh
./scripts/e2e-phase2.sh
./scripts/e2e-phase3.sh
./scripts/e2e-phase4.sh
```

## Тесты

```bash
go test ./internal/...
```
