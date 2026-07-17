# mvp-manager

Менеджер runtime ботов на ноде (Go): демон `agent` сверки desired/actual и CLI `ctl`.

На ранних этапах (Phase 0–2) хранилище — **in-memory** (`STORE=memory`). Данные сбрасываются при рестарте процесса. PostgreSQL появится позже (Phase PG).

Подробности ТЗ, плана и процесса работы с агентами — в [`docs/`](./docs/) ([TZ](./docs/TZ.md), [план](./docs/IMPLEMENTATION_PLAN.md), [как работать](./docs/Readme.md)).

## Сборка

```bash
go build -o bin/agent ./cmd/agent
go build -o bin/ctl ./cmd/ctl
```

Или одной командой:

```bash
go build ./cmd/agent ./cmd/ctl
```

## Конфигурация (ENV)

Конфиг читается из переменных окружения (`internal/config`), файл `.env` сам по себе не подхватывается. Образец — [`.env.example`](./.env.example).

| Переменная | Обязательно | По умолчанию | Описание |
|---|---|---|---|
| `NODE_ID` | да | — | Идентификатор ноды |
| `STORE` | нет | `memory` | `memory` или `postgres` (БД — Phase PG) |
| `DATABASE_URL` | нет | — | DSN для Postgres (позже; сейчас не используется) |

Задать в shell:

```bash
export NODE_ID=node-1
export STORE=memory
# или из файла: set -a && source .env && set +a
```

Неизвестный `STORE` (например `redis`) даёт понятную ошибку при `config.Load()` / запуске.  
`STORE=postgres` пока даёт ошибку «не реализован (Phase PG)» — без подключения к БД.

## Запуск agent (memory)

`agent` загружает ENV, создаёт in-memory store, регистрирует ноду (`Upsert` по `NODE_ID`) и ждёт сигнал завершения. Reconcile / supervisor процессов — Phase 1.

```bash
export NODE_ID=node-1
export STORE=memory
go run ./cmd/agent
```

В логе ожидается `store=memory` и сообщение о регистрации ноды. Процесс живёт до сигнала:

```bash
# в другом терминале — тихий выход (exit 0):
kill -TERM <pid>   # или Ctrl+C (SIGINT) в терминале agent
```

Help/version не требуют `NODE_ID`:

```bash
go run ./cmd/agent --help
go run ./cmd/agent --version
```

## Запуск ctl

`ctl` при обычном запуске тоже читает конфиг и создаёт memory store (подтверждение в логе). Подкоманды `bots*` — Phase 1.

```bash
NODE_ID=node-1 STORE=memory go run ./cmd/ctl
go run ./cmd/ctl --help
go run ./cmd/ctl --version
```

После сборки:

```bash
NODE_ID=node-1 ./bin/agent
./bin/agent --help
NODE_ID=node-1 ./bin/ctl
./bin/ctl --version
```

## Тесты

```bash
go test ./internal/config/...
go test ./internal/store/...
go test ./internal/store/memory/...
```
