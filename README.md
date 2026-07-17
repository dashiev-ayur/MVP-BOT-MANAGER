# mvp-manager

Менеджер runtime ботов на ноде (Go): демон `agent` сверки desired/actual и CLI `ctl`.

На ранних этапах (Phase 0–2) хранилище — **in-memory** (`STORE=memory`). Данные сбрасываются при рестарте процесса. PostgreSQL появится позже (Phase PG).

Подробности ТЗ, плана и процесса работы с агентами — в [`docs/`](./docs/) ([TZ](./docs/TZ.md), [план](./docs/IMPLEMENTATION_PLAN.md), [как работать](./docs/Readme.md)).

## Сборка

```bash
go build -o bin/agent ./cmd/agent
go build -o bin/ctl ./cmd/ctl
```

Или одной командой (бинарники в текущем каталоге / как настроено у `go build`):

```bash
go build ./cmd/agent ./cmd/ctl
```

## Конфигурация (ENV)

Конфиг читается из переменных окружения (`internal/config`), файл `.env` сам по себе не подхватывается. Образец — [`.env.example`](./.env.example).

| Переменная | Обязательно | По умолчанию | Описание |
|---|---|---|---|
| `NODE_ID` | да | — | Идентификатор ноды |
| `STORE` | нет | `memory` | `memory` или `postgres` (БД — Phase PG) |
| `DATABASE_URL` | нет | — | DSN для Postgres (позже) |

Задать в shell:

```bash
export NODE_ID=node-1
export STORE=memory
# или из файла: set -a && source .env && set +a
```

Неизвестный `STORE` (например `redis`) даёт понятную ошибку при `config.Load()` / запуске `agent`.

## Запуск

`ctl` — заглушка help/version. `agent` при обычном запуске загружает ENV и пишет `NODE_ID`/`STORE` в лог (store ещё не подключается).

```bash
export NODE_ID=node-1
go run ./cmd/agent
go run ./cmd/agent --help
go run ./cmd/agent --version

go run ./cmd/ctl --help
go run ./cmd/ctl --version
```

После сборки:

```bash
NODE_ID=node-1 ./bin/agent
./bin/agent --help
./bin/ctl --version
```

Тесты конфига:

```bash
go test ./internal/config/...
```
