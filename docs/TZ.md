# Техническое задание: MVP Bot Runtime Manager

**Версия:** 0.2  
**Стек:** Go, PostgreSQL  
**Статус:** MVP (1 сервер → расширение до 2–3)

---

## 1. Цель

Построить систему, которая:

- хранит реестр клиентских ботов (Telegram, Max) в **одной общей PostgreSQL**;
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

```
PostgreSQL (bots + runtimes + nodes)
        ↑↓
   Agent@node-N
        ├─ Supervisor → bot-runner (multi-tenant для default*)
        └─ Supervisor → custom bot processes (1:1)
```

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
| HTTP API (минимум) | CRUD ботов, start/stop, migrate |
| Контракт запуска | Единый ENV/команда для custom и для single-bot выдачи клиенту |
| Логи агента | slog |

### 3.2. Out of scope (v1)

- Авто-failover при падении ноды
- UI-админка
- Vault секретов (токены — в БД/ENV с оговоркой на prod)
- Автосборка custom из git на агенте (MVP: CI кладёт готовый артефакт на ноду)
- Windows-агент

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

---

## 5. Архитектура

### 5.1. Компоненты

```
┌──────────────────────────────────────────────────────────────┐
│                     PostgreSQL                               │
│         nodes | bots | runtimes | bot_events                 │
└──────────────────────────▲───────────────────────────────────┘
                           │
                    Agent @ node-1
                           │
         ┌─────────────────┼─────────────────┐
         ▼                                   ▼
 ┌───────────────────┐             ┌───────────────────┐
 │ bot-runner (PID)  │             │ custom bots (PID) │
 │  bot A (port P1)  │             │  acme-bot :P3     │
 │  bot B (port P2)  │             │  shop-bot :P4     │
 │  bot C (polling)  │             └───────────────────┘
 └───────────────────┘
```

**Agent:**

1. Registrar / heartbeat  
2. Reconciler (runtimes + желаемый набор ботов в runner)  
3. Supervisor (`os/exec`)  
4. HTTP API  
5. Optional `LISTEN/NOTIFY`

**Bot Runner (отдельный бинарник, ваш код):**

- читает из БД (или получает через API агента) список ботов с `bot_type ∈ default*` и `assigned_node_id = эта нода`, `desired_state=running`;
- для каждого такого бота поднимает внутренний инстанс (горутина / модуль сценария);
- **webhook:** слушает **уникальный `port` бота** внутри того же процесса (несколько `Listen` в одном процессе);
- **polling:** long poll в горутине, порт в БД зарезервирован, но не биндится (см. §6.3);
- горячо подхватывает add/remove/update ботов по `config_version` / reconcile.

### 5.2. Инварианты

1. Multi-tenant применяется **только** к вшитым типам (`default`, `default-extended`, …).  
2. `custom` всегда dedicated: отдельный OS-процесс.  
3. `port` уникален во всей БД (для всех типов).  
4. Бот с `desired_state=running` активен только на `assigned_node_id`.  
5. Один bot не обслуживается двумя runner’ами / двумя dedicated-процессами одновременно (lease).  
6. Migrate: stop/remove на A → смена assignment → start/add на B.  
7. Reconcile идемпотентен.

### 5.3. Нагрузка (ожидание)

- N default-ботов ≈ **1 процесс** `bot-runner` (+ память на инстансы внутри, существенно меньше N отдельных процессов).  
- M custom-ботов ≈ **M процессов** (Go легче, Node тяжелее — учитывать при лимитах).  
- При росте default-нагрузки — несколько runner’ов на ноде (шардирование ботов по `runtime_id`), не процесс на бота.

---

## 6. Модель данных (PostgreSQL)

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

### 10.1. Конфиг

```yaml
node_id: node-1
database_url: postgres://...
reconcile_interval: 3s
heartbeat_interval: 5s
lease_ttl: 15s
shutdown_grace: 10s
api_addr: ":8080"
bot_runner_command: /usr/local/bin/bot-runner
bot_runner_workdir: /var/lib/mvp-manager/runner
```

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

## 11. HTTP API (минимум)

| Method | Path | Описание |
|---|---|---|
| `GET` | `/healthz` | liveness агента |
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

---

## 12. Структура репозиториев

```
mvp-manager/                 # этот репо: агент + ТЗ
├── cmd/agent/
├── internal/...
├── migrations/
└── docs/TZ.md

bot-runner/                  # multi-tenant рантайм вшитых сценариев (можно monorepo)
├── cmd/runner/
└── internal/scenarios/
      ├── default/
      └── default_extended/

bots-custom/<custom_name>/   # отдельные репо; агент только запускает артефакт
```

Зависимости агента: `pgx/v5`, migrate (goose), `log/slog`, тонкий HTTP router.

---

## 13. Этапы реализации

### Phase 0 — каркас

- [ ] Go-модуль агента, миграции `nodes`, `runtimes`, `bots`, `bot_events`  
- [ ] docker-compose с PostgreSQL  
- [ ] конфиг / подключение к БД  

### Phase 1 — custom dedicated + supervisor

- [ ] Supervisor start/stop  
- [ ] Reconcile для `kind=custom_bot`  
- [ ] API create/start/stop custom  
- [ ] Уникальность port  

### Phase 2 — multi-tenant runner

- [ ] Бинарник `bot-runner` со сценарием `default`  
- [ ] Несколько ботов в одном процессе (webhook ports + polling)  
- [ ] Динамический add/remove инстансов  
- [ ] Агент поднимает один runner на ноду  

### Phase 3 — типы и migrate

- [ ] `default_extended` и расширяемый каталог типов  
- [ ] Lease, multi-node assignment  
- [ ] Migrate custom и default-бота  
- [ ] Шаблон handoff (`.env.example` + single-bot README)  

### Phase 4 — hardening

- [ ] LISTEN/NOTIFY, метрики, лимиты ботов на ноду  
- [ ] Шардирование на несколько runner’ов  
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
| API security | localhost или token |

---

## 15. Риски и решения

| Риск | Митигация |
|---|---|
| Утечка памяти в runner бьёт по всем default | лимиты, шардирование runner’ов, метрики |
| Падение runner гасит всех default на ноде | быстрый restart runtime; позже несколько runner’ов |
| Custom Node жрёт RAM | лимит custom на ноду; вынос на вторую ноду |
| Два runner подхватили одного бота | `runtime_id` + `assigned_node` + lease |
| Порт занят | проверка перед start; UNIQUE в БД |
| Handoff расходится с prod | один launch contract для runner-instance export и custom |

---

## 16. Критерии приёмки MVP

1. ≥2 бота типа `default` работают в **одном** OS-процессе `bot-runner`.  
2. У каждого бота уникальный `port`; webhook-боты слушают свои порты.  
3. Polling-бот работает без bind порта, порт зарезервирован в БД.  
4. `custom` бот стартует отдельным процессом по `start_command` + ENV.  
5. `custom_name` обязателен только для `custom`.  
6. Start/stop через БД/API отражается в actual state.  
7. Задокументирован handoff: исходники + `.env.example`.  
8. Закладываются `assigned_node_id` и migrate без двойного запуска.

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
| БД | Одна PostgreSQL |
| Default-сценарии | **Multi-tenant bot-runner** |
| Custom | Отдельный OS-процесс, отдельные репо |
| Порт | UNIQUE в БД на каждого бота |
| Поля бота | port, bot_type, custom_name (для custom) |
| Режимы | webhook + long polling |
| Handoff | Исходники Go/Node + launch contract |
| Масштаб | MVP: 1 нода; дизайн на 2–3 |
| Авто-failover | Нет в MVP |

---

## 19. Дальнейшие шаги

1. Утвердить v0.2 (особенно: multi-listen портов в одном runner vs path-based — выбран multi-listen под UNIQUE port).  
2. Phase 0–1 (агент + custom).  
3. Phase 2 (`bot-runner` + 2 default-бота в одном процессе).  
4. Шаблон handoff для default → single-bot репо.
