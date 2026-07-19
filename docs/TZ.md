# Техническое задание: MVP Bot Runtime Manager

**Версия:** 0.3  
**Стек:** Go; хранилище: in-memory → PostgreSQL  
**Статус:** MVP (1 сервер → расширение до 2–3)

---

## 1. Цель

Построить систему, которая:

- хранит реестр клиентских ботов (Telegram, Max) в **хранилище за интерфейсом** (сначала in-memory, затем PostgreSQL);
- для **дефолтных сценариев** запускает ботов в **multi-tenant** режиме (один OS-процесс — много ботов);
- для **custom** ботов запускает **отдельный OS-процесс** на бота (артефакт из отдельного репозитория, стандартизированная команда);
- на каждом сервере держит агент, который сводит фактическое состояние к желаемому (reconcile);
- позволяет **переносить** рантаймы/ботов между 2–3 серверами без двойного запуска;
- позволяет **отдать клиенту исходники** бота (Go или Node.js), чтобы он запустил их на своём сервере без нашего менеджера.

---

## 2. Проблема и подход

### 2.1. Проблема

Нужно разместить на сервере **как можно больше** клиентских ботов при ограниченной памяти/CPU. У ботов разные сценарии; часть — вшитые (`default`, `default-extended`, …), часть — кастомные репозитории. Кастом и вшитые default должны управляться одним менеджером. Клиенту в итоге могут передать только исходники бота.

### 2.2. Ключевое решение: гибридный runtime

| `bot_type` | Как исполняется | Зачем |
|---|---|---|
| `default`, `default-extended`, другие вшитые | **Multi-tenant Bot Runner** — один (или несколько) OS-процессов на ноду, внутри N ботов | Экономия RAM/CPU |
| `custom` | **Dedicated process** — 1 бот = 1 OS-процесс | Изоляция, свой стек (Go/Node), выдача исходников |

Менеджер управляет **OS-процессами рантайма** (`bot-runner`, custom-бинарники), а не каждым сообщением бота.  
Состояние desired/actual — в **Store** (порт хранилища). Реализации: `memory`, позже `postgres`. Бизнес-логика от SQL/драйвера **не зависит** (DIP).

```
Store (интерфейс)
  ├─ memory     ← текущий этап
  └─ postgres   ← Phase PG
        ↑↓
   Agent / ctl / runner (через интерфейсы)
        │
   ┌────┴────┬─────────────┬──────────────┐
   │         │             │              │
 Agent   healthcheck   control-api       ctl
   │
   ├─ Supervisor → bot-runner (multi-tenant для default*)
   └─ Supervisor → custom bot processes (1:1)
```

Управление start/stop/add — через изменения в **Store** (`desired_state` и т.д.). API и CLI — обёртки над теми же полями.

---

## 3. Область MVP

### 3.1. In scope

| Функция | Описание |
|---|---|
| Реестр ботов | port (UNIQUE), bot_type, custom_name, канал, режим webhook/polling |
| Multi-tenant runner | Вшитые сценарии в одном процессе |
| Dedicated custom | Запуск по стандартизированной команде из артефакта |
| Регистрация ноды / heartbeat | Агент пишет себя в `nodes` |
| Reconcile | desired ↔ actual для runtimes и состава ботов в runner |
| Lease / assigned_node | Защита от двойного запуска |
| Migrate | Перенос runner или custom-бота между нодами |
| Управление через Store | create/update ботов → агент/runner согласовывают состояние |
| HTTP API | Отдельный `control-api` (CRUD, start/stop, migrate) |
| CLI `ctl` | Операции из терминала без ручного SQL |
| Healthcheck | Отдельный `cmd/healthcheck` для `/healthz` webhook-ботов |
| Контракт запуска | Единый ENV/команда для custom и для single-bot выдачи клиенту |
| UI-админка | Каталог `web/` в monorepo; клиент только к `control-api` — см. [frontend.md](./frontend.md) |
| Логи | slog в каждом бинарнике |

### 3.2. Out of scope (v1)

- Авто-failover при падении ноды
- Vault секретов (токены — в БД/ENV с оговоркой на prod)
- Автосборка custom из git на агенте (MVP: CI кладёт готовый артефакт на ноду)
- Windows-агент
- Визуальный конструктор сценариев бота / RBAC в UI (см. [frontend.md](./frontend.md) §2.2)

---

## 4. Термины

| Термин | Значение |
|---|---|
| **Node** | Сервер с агентом |
| **Bot** | Логический бот клиента (токен, тип, порт, сценарий) |
| **Bot Runner** | Multi-tenant процесс для вшитых типов (`default*`) |
| **Runtime process** | OS-процесс, которым управляет supervisor (`bot-runner` или custom) |
| **Dedicated bot** | Custom-бот в отдельном OS-процессе |
| **Launch contract** | Стандартизированная команда + ENV |
| **Desired / actual state** | Желаемое и фактическое состояние |
| **Lease** | Временное владение runtime/ботом нодой |
| **Handoff** | Передача клиенту исходников бота для запуска у себя |
| **cmd** | Отдельный бинарник/`main` в `cmd/<name>` с своей ролью и жизненным циклом |

---

## 5. Архитектура

### 5.1. Компоненты

```
┌──────────────────────────────────────────────────────────────┐
│                     Store (memory → postgres)                 │
│         nodes | bots | runtimes | bot_events                 │
└─▲─────────▲──────────▲──────────▲────────────────────────────┘
  │         │          │          │
  │    control-api    ctl    healthcheck
  │
 Agent @ node-1
  │
  ├─ Supervisor → bot-runner (multi-tenant)
  └─ Supervisor → custom bots (1:1)
```

**Принцип разделения cmd:** выносить то, что отличается по жизненному циклу (разовое / другой интервал / не должно ронять демон). Ядро start/stop процессов остаётся в `agent`.

### 5.2. Состав бинарников (`cmd`)

#### Остаётся в `cmd/agent` (демон на каждой ноде)

| Ответственность | Почему здесь |
|---|---|
| Heartbeat ноды | Непрерывный цикл ноды |
| Reconcile + lease | Ядро desired → actual |
| Supervisor (`os/exec`) | Единственный владелец start/stop OS-процессов |
| `Wait()` на children | Мгновенно видеть падение runtime |
| Optional `LISTEN/NOTIFY` | Ускорение reconcile |

Агент **не** занимается HTTP-проверками `/healthz` ботов и **не** является единственной точкой CRUD (это `control-api` / `ctl` / прямой SQL).

#### Вынести в отдельные cmd

| Cmd | Тип | Назначение |
|---|---|---|
| **`bot-runner`** | демон (под супервизором агента) | Multi-tenant runtime вшитых сценариев |
| **`healthcheck`** | демон или cron-цикл | Опрос `GET /healthz` у webhook-ботов; пишет статус в БД; **не** рестартует процессы |
| **`ctl`** | CLI (разовый) | `bots start\|stop\|add\|migrate\|list` через БД/API |
| **`control-api`** | демон (часто 1 на кластер) | HTTP API; при 1 ноде можно временно совместить с агентом, целевая схема — отдельно |
| **`migrate`** (DB tooling) | разовый | Применение SQL-миграций (`goose up`) |
| **`handoff`** | разовый, после MVP | Сборка исходников + `.env.example` для выдачи клиенту |

#### Позже (не MVP)

| Cmd | Назначение |
|---|---|
| `drain-node` | Увести ботов с ноды перед ребутом |
| `doctor` | Сверка портов, PID, lease, orphan-процессов |
| `load-artifact` | Проверка наличия custom-артефакта и команды запуска |

#### Не выносить из агента

- логика lease и исполнение migrate на уровне процессов (stop → wait → start по факту строк в БД);
- отдельный «мини-API только для health» — достаточно `healthcheck`.

### 5.3. `cmd/healthcheck` — контракт

1. Читает из БД ботов: `desired_state=running`, `run_mode=webhook`, `assigned_node_id` (локальная нода или все — по флагу).  
2. Для каждого: `GET http://127.0.0.1:{port}/healthz` с таймаутом.  
3. При успехе/провале обновляет `bots.last_error`, пишет `bot_events`, при серии фейлов выставляет маркер (например `actual_state`/`health_status=unhealthy`).  
4. **Рестарт делает только `agent`** по reconcile (увидел unhealthy / failed → restart runtime или сигнал runner’у).  
5. Для `polling` `/healthz` не обязателен: опора на PID/`Wait()` и статус от runner’а; опциональный внутренний heartbeat позже.

Интервал healthcheck (например 10–30s) **независим** от `reconcile_interval`, чтобы таймауты HTTP не блокировали supervisor.

### 5.4. Отказоустойчивость: кто упал

| Событие | Поведение |
|---|---|
| Агент упал аварийно (panic, OOM, SIGKILL) | Дочерние боты/`bot-runner` **обычно продолжают работать**; управление по БД недоступно до рестарта агента |
| Агент штатный stop (SIGTERM) | MVP: агент останавливает управляемые runtimes (политику «оставить детей» можно сменить позже) |
| Упал custom-процесс | Агент видит через `Wait()` → `actual_state=failed` → restart по policy |
| Упал весь `bot-runner` | То же через `Wait()` |
| Упал один default-инстанс внутри runner | Runner обновляет статус бота в БД; агент/healthcheck дополняют картину |
| Бот «жив, но завис» (webhook) | Обнаруживает **`healthcheck`** по `/healthz` |

### 5.5. Bot Runner (кратко)

- читает из БД список ботов с `bot_type ∈ default*` и `assigned_node_id = эта нода`, `desired_state=running`;
- для каждого такого бота поднимает внутренний инстанс (горутина / модуль сценария);
- **webhook:** слушает **уникальный `port` бота** внутри того же процесса (несколько `Listen` в одном процессе);
- **polling:** long poll в горутине, порт в БД зарезервирован, но не биндится (см. §6.3);
- горячо подхватывает add/remove/update ботов по `config_version` / reconcile.

Подробности — §7.

### 5.6. Инварианты

1. Multi-tenant применяется **только** к вшитым типам (`default`, `default-extended`, …).  
2. `custom` всегда dedicated: отдельный OS-процесс.  
3. `port` уникален во всей БД (для всех типов).  
4. Бот с `desired_state=running` активен только на `assigned_node_id`.  
5. Один bot не обслуживается двумя runner’ами / двумя dedicated-процессами одновременно (lease).  
6. Migrate: stop/remove на A → смена assignment → start/add на B.  
7. Reconcile идемпотентен.  
8. Проверка `/healthz` — только в `healthcheck`; рестарт процессов — только в `agent`.

### 5.7. Нагрузка (ожидание)

- N default-ботов ≈ **1 процесс** `bot-runner` (+ память на инстансы внутри, существенно меньше N отдельных процессов).  
- M custom-ботов ≈ **M процессов** (Go легче, Node тяжелее — учитывать при лимитах).  
- При росте default-нагрузки — несколько runner’ов на ноде (шардирование ботов по `runtime_id`), не процесс на бота.

---

## 6. Модель данных (логическая + PostgreSQL)

Логическая модель едина для **memory** и **postgres**.  
Ниже — целевая схема PostgreSQL (Phase PG). In-memory реализует те же сущности и инварианты в структурах Go.

### 6.1. `nodes`

```sql
CREATE TABLE nodes (
    id              TEXT PRIMARY KEY,
    hostname        TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'online'
                    CHECK (status IN ('online', 'offline', 'draining')),
    last_seen_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    agent_version   TEXT,
    meta            JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 6.2. `runtimes` — OS-процессы, которыми управляет агент

```sql
CREATE TYPE runtime_kind AS ENUM ('bot_runner', 'custom_bot');
CREATE TYPE desired_state AS ENUM ('running', 'stopped');
CREATE TYPE actual_state AS ENUM (
    'unknown', 'starting', 'running', 'stopping', 'stopped', 'failed', 'migrating'
);

CREATE TABLE runtimes (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind              runtime_kind NOT NULL,
    name              TEXT NOT NULL UNIQUE,

    -- для bot_runner: команда запуска runner'а
    -- для custom_bot: стандартизированная команда из репо
    start_command     TEXT NOT NULL,
    workdir           TEXT,
    env               JSONB NOT NULL DEFAULT '{}'::jsonb,

    desired_state     desired_state NOT NULL DEFAULT 'stopped',
    actual_state      actual_state  NOT NULL DEFAULT 'unknown',

    assigned_node_id  TEXT REFERENCES nodes(id),
    lease_owner       TEXT REFERENCES nodes(id),
    lease_until       TIMESTAMPTZ,

    pid               INT,
    exit_code         INT,
    last_error        TEXT,
    config_version    BIGINT NOT NULL DEFAULT 1,

    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

На MVP допустим **один** `bot_runner` на ноду. Custom-бот ↔ строка `runtimes` с `kind=custom_bot` (связь с `bots` ниже).

### 6.3. `bots` — логические боты клиентов

```sql
CREATE TYPE bot_type AS ENUM (
    'custom',
    'default',
    'default_extended'
    -- далее: другие вшитые сценарии
);

CREATE TYPE bot_channel AS ENUM ('telegram', 'max');
CREATE TYPE bot_run_mode AS ENUM ('webhook', 'polling');

CREATE TABLE bots (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id         UUID,                         -- опционально, если будет таблица clients

    name              TEXT NOT NULL,                -- человекочитаемое имя
    bot_type          bot_type NOT NULL,
    custom_name       TEXT,                         -- обязательно при bot_type = custom
    channel           bot_channel NOT NULL,
    run_mode          bot_run_mode NOT NULL DEFAULT 'webhook',

    port              INT NOT NULL,                 -- UNIQUE во всей БД
    token_ref         TEXT NOT NULL,                -- ссылка/секрет; в MVP допустим encrypted/token column

    -- multi-tenant: к какому runner привязан (NULL если stopped / ещё не назначен)
    runtime_id        UUID REFERENCES runtimes(id),

    -- для custom: тот же runtime_id указывает на dedicated runtime
    -- artifact
    artifact_path     TEXT,                         -- путь на ноде к собранному боту
    repo_url          TEXT,                         -- для документации / handoff
    start_command     TEXT,                         -- override; иначе дефолт контракта

    desired_state     desired_state NOT NULL DEFAULT 'stopped',
    actual_state      actual_state  NOT NULL DEFAULT 'unknown',
    assigned_node_id  TEXT REFERENCES nodes(id),

    last_error        TEXT,
    config_version    BIGINT NOT NULL DEFAULT 1,
    scenario_config   JSONB NOT NULL DEFAULT '{}'::jsonb,  -- параметры дефолтного сценария

    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT bots_port_unique UNIQUE (port),
    CONSTRAINT bots_custom_name_required CHECK (
        (bot_type = 'custom' AND custom_name IS NOT NULL AND length(custom_name) > 0)
        OR (bot_type <> 'custom' AND custom_name IS NULL)
    )
);

CREATE INDEX bots_runtime_idx ON bots (runtime_id);
CREATE INDEX bots_node_desired_idx ON bots (assigned_node_id, desired_state);
CREATE INDEX bots_type_idx ON bots (bot_type);
```

**Порт:**

- уникален глобально;
- при `run_mode=webhook` — runner или custom-процесс **биндит** этот порт;
- при `run_mode=polling` — порт **зарезервирован** в БД (бронь ресурса / будущий webhook), процесс может его не слушать; агент не требует listen-check.

### 6.4. `bot_events`

```sql
CREATE TABLE bot_events (
    id           BIGSERIAL PRIMARY KEY,
    bot_id       UUID REFERENCES bots(id) ON DELETE CASCADE,
    runtime_id   UUID REFERENCES runtimes(id) ON DELETE SET NULL,
    node_id      TEXT,
    event_type   TEXT NOT NULL,
    message      TEXT,
    payload      JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 6.5. NOTIFY (опционально)

Триггеры на `bots` и `runtimes` → `pg_notify('bot_changes' | 'runtime_changes', ...)`.  
Poll остаётся safety net.

---

## 7. Multi-tenant Bot Runner (вшитые сценарии)

### 7.1. Ответственность

- Загрузить ботов: `bot_type <> 'custom'`, `assigned_node_id = self`, `desired_state = running`, `runtime_id = этот runner`.  
- Поднять/остановить внутренние инстансы при изменении набора.  
- Маршрутизация сценария по `bot_type` (+ `scenario_config`).  
- Поддержка **webhook и long polling** на уровне инстанса.  
- Health: процесс runner слушает служебный порт (из env runner’а) `/healthz`; опционально per-bot health на `bot.port` для webhook.

### 7.2. Динамика без рестарта всего runner (желательно в MVP)

| Событие | Действие runner |
|---|---|
| Новый bot desired=running | add instance |
| desired=stopped | remove instance, освободить listen |
| смена token/config_version | reload instance |
| смена run_mode | restart instance |

Полный рестарт OS-процесса runner — только при падении или обновлении бинарника runner’а.

### 7.3. Чего runner не делает

- Не запускает `custom` ботов.  
- Не раздаёт исходники клиенту.  
- Не выбирает ноду (это control plane / API).

---

## 8. Custom bots (dedicated)

### 8.1. Репозитории

- Отдельный git-репозиторий на бота (Go или Node.js).  
- Одна **стандартизированная команда запуска** (зафиксирована в README репо и в `bots.start_command` / `runtimes.start_command`).  
- Соблюдает launch contract (§9).

### 8.2. Как агент запускает

1. В БД: `bot_type=custom`, `custom_name`, `port`, `artifact_path`, `start_command`, `assigned_node_id`.  
2. Создаётся/обновляется `runtimes` (`kind=custom_bot`, связь 1:1 с ботом).  
3. Supervisor: `chdir(artifact_path)` + env + `start_command`.  
4. Stop: SIGTERM → grace → SIGKILL (process group).

MVP: артефакт уже лежит на ноде (CI/CD). Агент не обязан клонировать git.

---

## 9. Launch contract (единый)

Обязательные ENV:

| Переменная | Описание |
|---|---|
| `PORT` | порт из БД |
| `BOT_TOKEN` | токен мессенджера |
| `BOT_MODE` | `webhook` \| `polling` |
| `CHANNEL` | `telegram` \| `max` |
| `PUBLIC_URL` | для webhook (опционально) |

Команды (примеры):

```bash
# Go
./bot

# Node
npm start
# или: node dist/index.js
```

Рекомендуется: `GET /healthz` на `PORT` при webhook.

**Handoff клиенту:** исходники репо (custom) или сгенерированный/шаблонный проект для default-сценария в single-bot режиме + `.env.example`. Клиент запускает той же командой **без** нашего агента и multi-tenant runner.

---

## 10. Поведение агента

### 10.1. Конфиг агента

```yaml
node_id: node-1
database_url: postgres://...
reconcile_interval: 3s
heartbeat_interval: 5s
lease_ttl: 15s
shutdown_grace: 10s
bot_runner_command: /usr/local/bin/bot-runner
bot_runner_workdir: /var/lib/mvp-manager/runner
```

Конфиг `healthcheck` (отдельно): `check_interval`, `http_timeout`, `failure_threshold`, `database_url`, опционально `node_id`.  
Конфиг `control-api`: `api_addr`, `database_url`, auth token.

### 10.2. Reconcile (упрощённо)

```
1) Обеспечить runtime bot_runner на ноде, если есть default*-боты desired=running
2) Для custom ботов: ensure dedicated runtime process per bot
3) Runner сам (или агент через IPC/файл/DB watch) синхронизирует набор default*-инстансов
4) Обновить actual_state ботов и runtimes в БД
```

**Кто пишет actual_state ботов в multi-tenant:**

- предпочтительно сам **bot-runner** (он знает, какие инстансы живы);
- агент пишет actual_state **runtime** (PID runner’а / custom).

### 10.3. Lease

- Lease на уровне `runtimes` (как в v0.1 для processes).  
- Для default-бота «владение» = владение runner’ом на ноде + запись `runtime_id` / `assigned_node_id`.  
- Не допускать два runner’а с пересекающимся набором ботов.

### 10.4. Migrate

**Custom:** stop dedicated runtime на A → `assigned_node_id=B` → start на B (нужен артефакт на B).

**Default bot:**  
1) убрать бота из runner на A (`desired`/assignment);  
2) дождаться `actual_state=stopped` у бота;  
3) назначить на runner ноды B;  
4) runner B поднимает инстанс на том же `port`.

Перенос целого runner’а — крайний случай (обновление бинарника / drain ноды).

---

## 11. HTTP API (`cmd/control-api`)

Канонический путь управления — записи в **Store**. `control-api` и `ctl` только меняют те же поля.

| Method | Path | Описание |
|---|---|---|
| `GET` | `/healthz` | liveness **control-api** (не путать с `/healthz` ботов) |
| `GET` | `/v1/nodes` | ноды |
| `GET` | `/v1/bots` | список ботов |
| `POST` | `/v1/bots` | создать (port, bot_type, custom_name, …) |
| `PATCH` | `/v1/bots/{id}` | изменить конфиг / desired |
| `POST` | `/v1/bots/{id}/start` | desired=running |
| `POST` | `/v1/bots/{id}/stop` | desired=stopped |
| `POST` | `/v1/bots/{id}/migrate` | `{ "to_node_id": "node-2" }` |
| `GET` | `/v1/runtimes` | OS-процессы |
| `GET` | `/v1/bots/{id}/events` | аудит |

Создание `default` бота не создаёт новый OS-процесс — только строку в `bots` и привязку к runner.  
Создание `custom` — строка в `bots` + `runtimes`.

Эквивалент через CLI: `ctl bots start|stop|create|migrate|list`.

---

## 12. Структура репозиториев и cmd

```
mvp-manager/
├── cmd/
│   ├── agent/           # демон ноды: reconcile, lease, supervisor
│   ├── healthcheck/     # опрос /healthz webhook-ботов → БД
│   ├── ctl/             # CLI-операции над ботами
│   ├── control-api/     # HTTP API (1 на кластер; на MVP допустим совместно с agent)
│   ├── migrate/         # goose/sql migrations runner (опционально)
│   └── handoff/         # позже: упаковка исходников клиенту
├── internal/
│   ├── config/
│   ├── db/
│   ├── node/
│   ├── reconcile/
│   ├── supervisor/
│   ├── lease/
│   ├── health/          # общая логика проверки, используется cmd/healthcheck
│   └── api/             # handlers для control-api
├── migrations/
└── docs/TZ.md

bot-runner/              # отдельно или monorepo
├── cmd/runner/
└── internal/scenarios/
      ├── default/
      └── default_extended/

bots-custom/<custom_name>/   # отдельные репо; агент только запускает артефакт
```

Зависимости ранних фаз: `log/slog`; HTTP-router — в `control-api`.  
`pgx/v5`, goose — только в Phase PG (`internal/store/postgres`, `cmd/migrate`).

---

## 13. Этапы реализации

### Phase 0 — каркас

- [ ] Go-модуль `mvp-manager`, каркас `cmd/agent`, `cmd/ctl`  
- [ ] `internal/store` (интерфейсы) + `internal/store/memory`  
- [ ] конфиг `STORE=memory`  
- [ ] бизнес-логика без зависимости от pgx/SQL  

### Phase 1 — custom dedicated + supervisor

- [ ] Supervisor start/stop в `agent`  
- [ ] Reconcile для `kind=custom_bot`  
- [ ] Управление через Store + `ctl` (start/stop/create)  
- [ ] Уникальность port  
- [ ] Минимальный `control-api` (или только `ctl` на первом шаге)  

### Phase 2 — multi-tenant runner + healthcheck

- [ ] Бинарник `bot-runner` со сценарием `default`  
- [ ] Несколько ботов в одном процессе (webhook ports + polling)  
- [ ] Динамический add/remove инстансов  
- [ ] Агент поднимает один runner на ноду  
- [ ] `cmd/healthcheck`: опрос `/healthz`, запись в БД, без рестартов  
- [ ] Агент реагирует на unhealthy/failed  

### Phase PG — PostgreSQL

- [ ] goose + схема ТЗ §6  
- [ ] `internal/store/postgres`  
- [ ] `STORE=postgres` без изменения reconcile/supervisor  

### Phase 3 — типы и migrate

- [ ] `default_extended` и расширяемый каталог типов  
- [ ] Lease, multi-node assignment  
- [ ] Migrate custom и default-бота (`ctl` / `control-api`)  
- [ ] Вынос `control-api` в отдельный процесс при 2-й ноде  
- [ ] Шаблон handoff (`.env.example` + single-bot README)  

### Phase 4 — hardening

- [ ] LISTEN/NOTIFY, метрики, лимиты ботов на ноду  
- [ ] Шардирование на несколько runner’ов  
- [ ] `cmd/handoff`, опционально `doctor` / `drain-node`  
- [ ] Улучшение секретов токенов  

---

## 14. Нефункциональные требования

| Требование | MVP |
|---|---|
| Плотность default-ботов | multi-tenant обязателен |
| Custom | process-per-bot |
| Порт | UNIQUE в БД |
| Мессенджеры | Telegram, Max |
| Режимы | webhook и long polling |
| Handoff | исходники Go/Node + launch contract |
| Надёжность | нет двойного обслуживания одного bot_id |
| Разделение cmd | agent / healthcheck / ctl / control-api / bot-runner |
| Обнаружение зависания webhook | отдельный `healthcheck`, не агент |
| API security | localhost или token |

---

## 15. Риски и решения

| Риск | Решение |
|---|---|
| Утечка памяти в runner бьёт по всем default | лимиты, шардирование runner’ов, метрики |
| Падение runner гасит всех default на ноде | быстрый restart runtime; позже несколько runner’ов |
| Custom Node жрёт RAM | лимит custom на ноду; вынос на вторую ноду |
| Два runner подхватили одного бота | `runtime_id` + `assigned_node` + lease |
| Порт занят | проверка перед start; UNIQUE в БД |
| Handoff расходится с prod | один launch contract для runner-instance export и custom |
| Healthcheck блокирует reconcile | вынесен в отдельный `cmd/healthcheck` |
| Агент упал — потеряли ботов | children живут при аварийном падении; после рестарта агент догоняет БД |

---

## 16. Критерии приёмки MVP

1. ≥2 бота типа `default` работают в **одном** OS-процессе `bot-runner`.  
2. У каждого бота уникальный `port`; webhook-боты слушают свои порты.  
3. Polling-бот работает без bind порта, порт зарезервирован в БД.  
4. `custom` бот стартует отдельным процессом по `start_command` + ENV.  
5. `custom_name` обязателен только для `custom`.  
6. Start/stop через Store (`ctl`/API) отражается в actual state.  
7. `healthcheck` детектит падение `/healthz` и пишет в Store; рестарт выполняет `agent`.  
8. Бинарники разделены: как минимум `agent`, `bot-runner`, `healthcheck`, `ctl`.  
9. Задокументирован handoff: исходники + `.env.example`.  
10. Закладываются `assigned_node_id` и migrate без двойного запуска.

---

## 17. Примеры

### 17.1. Создать default-бота (multi-tenant)

```http
POST /v1/bots
{
  "name": "client-42-main",
  "bot_type": "default",
  "channel": "telegram",
  "run_mode": "webhook",
  "port": 18042,
  "assigned_node_id": "node-1",
  "desired_state": "running",
  "token_ref": "secret:bot-42",
  "scenario_config": {}
}
```

Агент гарантирует running `bot-runner` на node-1; runner поднимает инстанс на `:18042`.

### 17.2. Создать custom-бота

```http
POST /v1/bots
{
  "name": "acme-shop",
  "bot_type": "custom",
  "custom_name": "acme-shop-bot",
  "channel": "telegram",
  "run_mode": "polling",
  "port": 18099,
  "assigned_node_id": "node-1",
  "desired_state": "running",
  "artifact_path": "/var/bots/acme-shop-bot",
  "start_command": "./bot",
  "token_ref": "secret:acme"
}
```

Агент стартует dedicated runtime: `PORT=18099 BOT_MODE=polling BOT_TOKEN=... ./bot`.

### 17.3. Migrate default-бота на node-2

```http
POST /v1/bots/{id}/migrate
{ "to_node_id": "node-2" }
```

→ remove из runner node-1 → assign → add в runner node-2 на том же port.

---

## 18. Решения, зафиксированные для проекта

| Вопрос | Решение |
|---|---|
| Язык менеджера | Go |
| БД / Store | Сначала **in-memory**; PostgreSQL — Phase PG (одна общая БД на ноды) |
| Изоляция от БД | Бизнес-логика зависит только от интерфейсов `store` (DIP) |
| Default-сценарии | **Multi-tenant bot-runner** |
| Custom | Отдельный OS-процесс, отдельные репо |
| Порт | UNIQUE в Store на каждого бота |
| Поля бота | port, bot_type, custom_name (для custom) |
| Режимы | webhook + long polling |
| Управление | Изменения в Store; `ctl` / `control-api` — обёртки |
| `/healthz` ботов | Отдельный `cmd/healthcheck`; рестарт только в `agent` |
| HTTP API | `cmd/control-api` (не смешивать с ядром агента) |
| CLI | `cmd/ctl` |
| UI | Monorepo: каталог **`web/`**; только `control-api`; ТЗ — [frontend.md](./frontend.md) |
| Handoff | Исходники Go/Node + launch contract; утилита `cmd/handoff` позже |
| Go-модуль | `mvp-manager` |
| `bot-runner` | Monorepo: `cmd/bot-runner` + `internal/runner` |
| Объём MVP | Весь проект по плану (Phase 0–4 + PG); сдача поэтапная |
| Масштаб | MVP: 1 нода; дизайн на 2–3 |
| Авто-failover | Нет в MVP |
| Запуск Postgres | Инструкции пользователя позже |

---

## 19. Дальнейшие шаги

1. Пользователь выдаёт задания manager по одному этапу.  
2. Backend-фазы (0–4, PG) — по плану.  
3. UI — Phase UI / задания по [frontend.md](./frontend.md) (§14), каталог `web/`.
