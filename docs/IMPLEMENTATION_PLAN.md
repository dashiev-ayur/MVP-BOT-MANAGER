# План реализации — чеклист

**Версия:** 0.5  
**Основание:** [TZ.md](./TZ.md) v0.3+  
**Цель документа:** чеклист фаз для вас и для manager. Пользователь **сам** выдаёт задания и **сам** принимает этапы; целиком проект одним заданием не отдаётся (даже если MVP = все фазы).

**Хранилище:** in-memory (`STORE=memory`) и PostgreSQL (`STORE=postgres`, Phase PG).

**Manager** отмечает пункты `[x]` по мере реализации (после PASS от tester).  
Пункт **«Принято вами»** — только после вашего явного ок.

---

## Легенда

| Отметка | Значение |
|---|---|
| `[ ]` | ещё не сделано |
| `[x]` | сделано (агенты + tester) |
| **Принято вами** | ручная проверка пройдена, можно следующий этап |

**Правила**

1. Вам выдавать manager **одно** задание за раз (подпункт, блок или одна фаза) — не весь проект.
2. Следующая фаза — только после **Принято вами** на текущей.
3. Цепочка: вы → manager → developer → tester (до 2 доработок) → отчёт вам.

---

## Сводка прогресса

| Фаза | Название | Статус |
|---|---|---|
| 0 | Каркас + in-memory store | ✅ принята |
| 1 | Custom-бот + supervisor + ctl | ✅ принята |
| 2 | Multi-tenant runner + healthcheck | ✅ принята |
| PG | PostgreSQL (адаптер store) | ✅ принята |
| 3 | Типы, lease, migrate, API | ✅ принята |
| 4 | Укрепление (hardening) | ✅ принята |

*Статусы фазы: `⏳ не начата` → `🚧 в работе` → `👀 ждёт вашей проверки` → `✅ принята`.*  
*Manager обновляет строку фазы в этой таблице.*

---

## Phase 0 — Каркас репозитория

**Зачем:** собираемый каркас + **хранилище в памяти** за интерфейсами (бизнес-логика не знает про Postgres).

**Статус фазы:** ✅ принята

### 0.1. Go-модуль и структура каталогов

- [x] Инициализировать Go-модуль: `mvp-manager`
- [x] Каталоги: `cmd/agent`, `cmd/ctl`, `internal/config`, `internal/store` (интерфейсы), `internal/store/memory`
- [x] Заглушки `main.go` (версия / help)
- [x] `.gitignore` уже есть — проверить актуальность
- [x] Короткий корневой `README.md` (что это, `STORE=memory`, как запускать)
- [x] **Принято вами** (2026-07-17)

**Проверка:** `go build ./cmd/agent ./cmd/ctl`  
*( `cmd/migrate` — не в этой фазе, появится с PostgreSQL )*

### 0.2. Конфиг

- [x] `internal/config` — чтение из ENV
- [x] `.env.example`: `NODE_ID`, `STORE=memory` (позже `postgres` + DSN)
- [x] Без обязательного Docker/Postgres на этом этапе
- [x] **Принято вами** (2026-07-17)

**Проверка:** конфиг читается; неизвестный `STORE` — понятная ошибка

### 0.3. Порт хранилища (SOLID / DIP)

- [x] Узкие интерфейсы в `internal/store` (BotRepository, RuntimeRepository, NodeRepository или эквивалент) — **без** импорта `pgx`/SQL
- [x] Домен/`reconcile` зависят **только** от интерфейсов
- [x] Запрещено: тащить драйвер БД в `internal/reconcile`, `internal/supervisor`, сценарии runner
- [x] **Принято вами** (2026-07-17)

**Проверка:** пакеты бизнес-логики не импортируют `jackc/pgx` и `database/sql`

### 0.4. Реализация in-memory

- [x] `internal/store/memory` — потокобезопасное хранилище (mutex)
- [x] Те же инварианты, что в модели ТЗ: UNIQUE port, custom_name для custom, desired/actual
- [x] Данные живут, пока жив процесс (рестарт агента = пустое состояние — ок для этапа)
- [x] Wiring в `agent`/`ctl`: `STORE=memory` → memory store
- [x] Корректное завершение по SIGINT/SIGTERM
- [x] **Принято вами** (2026-07-17)

**Проверка:** `agent` стартует с memory store, пишет в лог тип хранилища, тихо выходит по сигналу

### Закрытие Phase 0

- [x] Все подпункты 0.1–0.4 отмечены `[x]`
- [x] **Принято вами** (2026-07-17)

---

## Phase 1 — Custom-бот, supervisor, ctl

**Зачем:** один custom-процесс стартует и останавливается по записям в **store** (пока memory).

**Статус фазы:** ✅ принята  
**Старт только после:** Phase 0 → ✅ принята

### 1.1. Доменные типы

- [x] Структуры Runtime / Bot в `internal/…`
- [x] Маппинг полей store ↔ Go

### 1.2. Supervisor процессов

- [x] `Start` / `Stop` / учёт PID
- [x] Ожидание выхода (`Wait`) в фоне
- [x] Process group; SIGTERM → ожидание → SIGKILL
- [x] Подробные комментарии жизненного цикла (на русском)

**Проверка:** автотест: start → процесс жив → stop → процесса нет

### 1.3. Reconcile для custom

- [x] Цикл сверки desired ↔ actual для `kind=custom_bot`
- [x] Запись в store: `pid`, `actual_state`, `last_error`, `exit_code`
- [x] Heartbeat ноды в store
- [x] Полноценный lease пока не обязателен (поля модели уже заложены)

**Проверка:** смена `desired_state` через `ctl`/store → процесс появляется/исчезает за интервал reconcile

### 1.4. CLI `ctl`

- [x] `bots create` (custom + runtime, проверка уникальности порта)
- [x] `bots start` / `bots stop`
- [x] `bots list`
- [x] `runtimes list`
- [x] Токен в MVP — простое поле/ENV (без vault)

**Проверка:** create → start → list (running) → stop

### 1.5. Заглушка custom-бота (fake-bot)

- [x] Пример бинарника (`examples/fake-bot` или `testdata/…`)
- [x] Читает `PORT`, `BOT_MODE`, …
- [x] Webhook: отвечает на `/healthz`
- [x] Polling: просто живёт до SIGTERM
- [x] Короткий E2E-сценарий/скрипт с `ctl`

**Проверка:** полный прогон ctl + fake-bot на локальной машине

### Закрытие Phase 1

- [x] Custom управляется из store / `ctl`
- [x] Краш дочернего процесса → `actual_state=failed`, агент жив
- [x] `control-api` сознательно **не** делаем в этой фазе
- [x] **Принято вами** (2026-07-17)

---

## Phase 2 — Multi-tenant bot-runner и healthcheck

**Зачем:** много default-ботов в одном OS-процессе + отдельная проверка `/healthz`.

**Статус фазы:** ✅ принята (lifecycle 2.1–2.4; блок 2.5 — отдельно)  
**Старт только после:** Phase 1 → ✅ принята

### 2.1. Размещение runner (зафиксировано)

- [x] Вариант **A**: monorepo `cmd/bot-runner` + `internal/runner` (решение 2026-07-17)
- [x] Краткая запись в README корня проекта при появлении кода

### 2.2. Ядро bot-runner (lifecycle без полного Telegram)

- [x] Загрузка default*-ботов из store по ноде / runtime / desired
- [x] In-memory реестр инстансов
- [x] Динамический add / remove / reload по `config_version`
- [x] Сценарий `default` (минимум: healthz + заглушка логики)
- [x] Webhook: listen на уникальном `port` бота
- [x] Polling: горутина без bind порта (порт зарезервирован в БД)

**Проверка:** 2 default webhook-бота → **один** PID runner, оба порта отдают `/healthz`

### 2.3. Агент управляет процессом runner

- [x] Если есть running default* — обеспечить runtime `bot_runner` на ноде
- [x] Старт / стоп бинарника runner через supervisor
- [x] Привязка `bots.runtime_id`

**Проверка:** stop runner в store → PID убит; start → инстансы снова живы

### 2.4. Healthcheck (отдельный cmd)

- [x] Бинарник `cmd/healthcheck`
- [x] Опрос `/healthz` только у webhook + desired=running
- [x] Пишет статус / events в store; **сам не рестартит**
- [x] Агент реагирует на unhealthy/failed (рестарт по policy)

**Проверка:** «сломать» healthz → unhealthy в store → агент восстанавливает

### 2.5. Мессенджеры (после стабильного lifecycle)

**Статус блока:** ✅ принят  
Делать **отдельным** заданием manager, когда 2.2–2.4 приняты.

- [x] Telegram для сценария `default` (webhook и/или polling)
- [x] Канал Max (можно после Telegram)
- [x] Подтягивание токена из `token_ref` / конфига
- [x] **Принято вами** (2026-07-17)

**Проверка:** тестовый Telegram-бот отвечает на `/start` (mock-тесты + опционально live: `scripts/manual-telegram-start.sh`)

### Закрытие Phase 2

- [x] ≥2 default в одном процессе runner
- [x] Unique port соблюдается
- [x] Custom из Phase 1 по-прежнему работает
- [x] Есть бинарники: `agent`, `bot-runner`, `healthcheck`, `ctl`
- [x] **Принято вами** (2026-07-17; lifecycle 2.1–2.4; блок 2.5 принят 2026-07-17)

---

## Phase PG — PostgreSQL (адаптер хранилища)

**Зачем:** персистентность и общая БД на несколько нод. Бизнес-логика **не меняется** — только новая реализация `internal/store`.

**Статус фазы:** ✅ принята (требования зафиксированы 2026-07-17; приёмка 2026-07-17)  
**Старт:** выполнен по `/manager Phase PG`.

### Решения (зафиксировано)

| Вопрос | Решение | Дата |
|---|---|---|
| Запуск Postgres | Docker: **`docker compose up -d`** (одна команда поднимает контейнер) | 2026-07-17 |
| Параметры подключения | В `.env` / `.env.example`: `STORE=postgres`, `DATABASE_URL` (и при необходимости host/port/user/password как часть DSN) | 2026-07-17 |
| E2E и рабочая БД | **Разные базы** в том же Docker Postgres (не отдельная schema): например `mvp_manager` (dev) и `mvp_manager_e2e` (тесты). DSN: `DATABASE_URL` vs `DATABASE_URL_E2E` | 2026-07-17 |
| Миграции | Отдельная команда: `cmd/migrate` (goose) — **не** смешивать с compose up | 2026-07-17 |
| Сиды | Отдельная **ручная** команда (не при первом `compose up`) | 2026-07-17 |
| Таблица `clients` | **Нет** — только `bots.client_id` (UUID) | 2026-07-17 |
| Состав сидов | 2 клиента (2 UUID): у 1-го — **2** бота, у 2-го — **1** бот; все `bot_type=default` | 2026-07-17 |
| Цель сидов | Посмотреть работу default/runner на Postgres на текущем этапе | 2026-07-17 |

**Порядок локального старта (после реализации фазы):**

```bash
# 1) БД
docker compose up -d

# 2) схема
go run ./cmd/migrate up
# или: ./bin/migrate up

# 3) сиды (вручную)
go run ./cmd/migrate seed
# client_a=11111111-1111-4111-8111-111111111111 (2 бота)
# client_b=22222222-2222-4222-8222-222222222222 (1 бот)

# 4) приложение
export STORE=postgres
# DATABASE_URL из .env  → база mvp_manager (dev/ручная работа)
./bin/agent

# E2E (тот же контейнер, другая БД):
# DATABASE_URL_E2E=.../mvp_manager_e2e
# migrate up / тесты только против DATABASE_URL_E2E — не трогают dev-данные
```

### PG.1. Инфраструктура

- [x] `docker-compose.yml` — сервис PostgreSQL; подъём: `docker compose up -d`
- [x] При инициализации тома (или init-скрипт) создать **две** базы: рабочую (`mvp_manager`) и e2e (`mvp_manager_e2e`) — либо документированный способ `CREATE DATABASE` один раз
- [x] `.env.example`: `STORE=postgres`, `DATABASE_URL=…` (dev), `DATABASE_URL_E2E=…` (e2e; другое имя БД, тот же хост/порт)
- [x] `cmd/migrate` + goose — применение SQL-миграций отдельной командой; для e2e — migrate на `DATABASE_URL_E2E`
- [x] SQL-схема по ТЗ §6 (`nodes`, `runtimes`, `bots`, `bot_events`, …); **без** таблицы `clients`
- [x] Команда сидов (отдельно от migrate up и от compose): идемпотентно или с защитой от повторного загрязнения — на усмотрение реализации, но запуск **только вручную**; сиды по умолчанию в **dev**-БД (`DATABASE_URL`), не в e2e (если не указано иное)

### PG.2. Адаптер

- [x] `internal/store/postgres` реализует те же интерфейсы, что memory (включая lease CAS, events)
- [x] Wiring: `STORE=postgres` → postgres store
- [x] Бизнес-пакеты по-прежнему **без** импорта pgx
- [x] Опционально в рамках PG: реализация `LISTEN/NOTIFY` для `ChangeWatcher` (задел Phase 4) — желательно, не блокирует закрытие, если poll достаточен

### PG.3. Сиды (ручные)

- [x] Два стабильных UUID клиента (зафиксировать в SQL/коде сида, задокументировать в README)
- [x] Клиент A (`client_id=11111111-1111-4111-8111-111111111111`): **2** бота, `bot_type=default`
- [x] Клиент B (`client_id=22222222-2222-4222-8222-222222222222`): **1** бот, `bot_type=default`
- [x] Уникальные `port`; токены-заглушки / `token_ref` для локального просмотра; `desired_state` — разумный дефолт для демо (например `stopped`, чтобы не требовать живой Telegram до `ctl bots start`)
- [x] При необходимости — строка `bot_runner` runtime и привязка `runtime_id` / `assigned_node_id` под демо на одной ноде (согласовать с `NODE_ID` из `.env.example`)

### PG.4. E2E на Postgres

- [x] Скрипты e2e (или отдельный `e2e-phase-pg.sh`) используют **`DATABASE_URL_E2E`**, не `DATABASE_URL`
- [x] Перед прогоном: migrate up на e2e-БД; без сидов dev (или свой минимальный фикстурный набор только в e2e)
- [x] Dev-БД и ручные сиды не затираются e2e

**Проверка:** `docker compose up -d` → migrate up (dev) → seed вручную (dev) → `STORE=postgres` демо default; отдельно migrate + e2e на `mvp_manager_e2e`; memory ↔ postgres только конфигом

### Закрытие Phase PG

- [x] Compose + две БД (dev/e2e) + миграции + ручные сиды + адаптер готовы
- [x] Документация запуска в корневом README и `.env.example` (`DATABASE_URL`, `DATABASE_URL_E2E`)
- [x] **Принято вами** (2026-07-17; в т.ч. `STORE=postgres` по умолчанию в `.env` / `.env.example`, compose из `POSTGRES_*`)

---

## Phase 3 — Типы сценариев, lease, migrate, API

**Зачем:** расширение сценариев и перенос между нодами (2–3 сервера).

**Статус фазы:** ✅ принята  
**Старт только после:** Phase 2 → ✅ принята (Phase PG желателен до multi-node migrate, для single-node можно позже)

### 3.1. Каталог вшитых типов

- [x] Сценарий `default_extended`
- [x] Узкий интерфейс регистрации сценариев (без монолитного switch «навсегда»)

### 3.2. Lease на runtime

- [x] Acquire / renew / release
- [x] Старт процесса только при успешном lease
- [x] Проверка гонки: два агента с разными `NODE_ID` — второй не захватывает чужой runtime

### 3.3. Migrate бота между нодами

- [x] Протокол: stop/remove → wait → reassign → start/add
- [x] Путь для custom
- [x] Путь для default (через runner)
- [x] Команда `ctl bots migrate`

**Проверка:** E2E на двух агентах (можно одна машина, два `NODE_ID`)

### 3.4. HTTP `control-api`

- [x] Отдельный `cmd/control-api`
- [x] Эндпоинты из ТЗ §11
- [x] Auth token; по умолчанию bind localhost

### 3.5. Выдача бота клиенту (документы)

- [x] Описание handoff + `.env.example` для single-bot режима
- [x] `cmd/handoff` — по желанию здесь или в Phase 4 *(сделано в Phase 4)*

### Закрытие Phase 3

- [x] Migrate без двойного запуска
- [x] Lease работает
- [x] **Принято вами** (2026-07-17)

---

## Phase 4 — Hardening

**Зачем:** надёжность и удобство в проде (часть полного MVP проекта).

**Статус фазы:** ✅ принята  
**Старт только после:** Phase 3 → ✅ принята (или явное решение начать раньше точечно)

### 4.1. Реакция на изменения в БД

- [x] `LISTEN/NOTIFY` + периодический poll как safety net *(poll + file-watch для memory; LISTEN/NOTIFY — интерфейс/nop до Phase PG)*

### 4.2. Устойчивость процессов

- [x] Restart policy / backoff
- [x] Лимиты числа ботов на ноду

### 4.3. Наблюдаемость и сопровождение

- [x] Метрики (slog-счётчики или Prometheus)
- [x] Улучшение хранения секретов токенов
- [x] Опционально: `doctor`, `drain-node` *(включены в согласованный набор)*
- [ ] Опционально: шардирование нескольких runner’ов *(вне согласованного набора; TODO в README)*
- [ ] **TODO:** unhealthy webhook → reload **одного** инстанса в runner, а не Stop+Start всего `bot_runner` *(сейчас agent рестартит весь runtime — лишний blast radius; отдельным заданием)*
- [x] `cmd/handoff` (отложено из Phase 3.5)

### Закрытие Phase 4

- [x] Согласованный набор пунктов 4.x закрыт *(без multi-runner sharding; LISTEN/NOTIFY — задел под PG)*
- [x] **Принято вами** (2026-07-17)

---

## Backlog (после принятых фаз)

Задачи вне закрытого MVP; брать отдельным `/manager`, не смешивать с уже принятыми фазами.

| TODO | Суть | Зачем |
|---|---|---|
| Unhealthy → reload инстанса | Healthcheck помечает одного webhook → runner перезапускает **только** этот бот (remove/add или bump `config_version`); Stop+Start всего `bot_runner` оставить для краша PID / массовых фейлов | Не гасить соседние default при одном unhealthy |
| Multi-runner sharding | Несколько `bot_runner` на ноде, раздача ботов по `runtime_id` | Меньший blast radius и нагрузка при многих default |

| Вопрос | Решение | Дата |
|---|---|---|
| Go-модуль / module path | `mvp-manager` | 2026-07-17 |
| Размещение `bot-runner` | **A** — monorepo (`cmd/bot-runner` + `internal/runner`) | 2026-07-17 |
| Граница MVP | **Весь проект** (Phase 0–4 + Phase PG); этапы сдаются по одному | 2026-07-17 |
| Кто выдаёт задания | Пользователь сам | 2026-07-17 |
| Хранилище на старте | **In-memory** за интерфейсами (`internal/store`); бизнес-логика не зависит от Postgres | 2026-07-17 |
| PostgreSQL | **Phase PG**; `docker compose up -d`; миграции и сиды — ручные команды; без `clients`; сиды 2+1 default; e2e — **отдельная БД** (`DATABASE_URL_E2E`), не schema | 2026-07-17 |

---

## Как читать отчёт manager

После каждого задания вы получите:

1. что сделано и какие пункты плана отмечены `[x]`;
2. вердикт tester;
3. **как проверить вручную** (команды);
4. что сознательно не входило в задание.

Вы проверяете → пишете «ок» / замечания → только потом следующее задание.
