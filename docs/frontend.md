# Техническое задание: Frontend (UI-админка)

**Версия:** 1.0  
**Статус:** черновик для реализации  
**Каталог кода:** `web/` (monorepo `mvp-manager`)  
**Бэкенд:** только HTTP `control-api` (Bearer). UI не ходит в Postgres, agent, bot-runner и store.

Связанные документы: [TZ.md](./TZ.md) (продукт), [IMPLEMENTATION_PLAN.md](./IMPLEMENTATION_PLAN.md), корневой [README.md](../README.md).

---

## 1. Цель и контекст

### 1.1. Зачем UI

Дать оператору удобную админку вместо повседневного `ctl`/SQL:

- видеть состояние нод, runtimes и ботов;
- создавать ботов, start / stop / migrate;
- смотреть события и быстро понимать, что «сломалось».

UI — **control-plane клиент**. Это не редактор диалоговых сценариев мессенджеров и не APM.

### 1.2. Пользователь

| Роль | Описание |
|---|---|
| Оператор / владелец | Один человек (или малая команда) с общим `CONTROL_API_TOKEN`. RBAC и мультипользователи — out of scope v1 |

### 1.3. Принцип управления

Как и `ctl`: UI меняет **desired**-состояние через API; **actual** догоняет агент (reconcile). После Start/Stop/Migrate нельзя обещать мгновенный `actual_state` — UI обязан это показывать.

---

## 2. Границы scope

### 2.1. In scope (UI v1 = P0 + часть P1)

| Область | Содержание |
|---|---|
| Auth | Base URL + Bearer token, сохранение сессии |
| Оболочка | Layout, навигация, индикатор API |
| Обзор | Сводки + «требуют внимания» |
| Боты | Список, фильтры, карточка, create, start/stop, migrate, events |
| Ноды | Список (P0), карточка с ботами/runtimes (P1) |
| Runtimes | Список read-only + связь с ботами (P1) |
| UX | Confirm на опасные действия, ошибки API, пустые состояния; toasts и poll — P1 |
| Язык UI | Русский |

### 2.2. Out of scope (сознательно)

| Не делаем в UI v1 | Почему |
|---|---|
| Прямой доступ к Postgres / store | Только control-api |
| Управление процессами в обход API | Ломает lease/reconcile |
| Визуальный конструктор сценариев Telegram/Max | Продукт бота, не control-plane |
| Сборка/деплой custom из git | CI + `artifact_path`; UI только поля |
| Мастер `handoff` | Есть `cmd/handoff`; UI — P2 |
| Vault секретов | `token_ref` как поле; не строить vault |
| Grafana / полноценные метрики | Есть `/metrics`; UI — операционка |
| RBAC / аудит «кто нажал» | Один Bearer; нет API аудита операторов |
| WebSocket / realtime push | Poll или ручной refresh |
| SSR / SEO | Внутренняя админка |

---

## 3. Размещение и стек

### 3.1. Репозиторий

```
mvp-manager/
  cmd/control-api/     # HTTP API
  web/                 # SPA (свой package.json)
  docs/frontend.md     # это ТЗ
```

| Правило | Смысл |
|---|---|
| Monorepo `web/` | Один PR может менять API и UI |
| Отдельный процесс/сборка | Не смешивать с `go build` агента |
| Не встраивать в `agent` | Agent — data plane на ноде |

Позже (не блокер v1): вынести `web/` или `embed` статику в `control-api` — без смены контракта API.

### 3.2. Стек (зафиксировать при scaffold)

| Слой | Выбор |
|---|---|
| Сборка | Vite |
| UI | React + TypeScript |
| Роутинг | React Router (или аналог) |
| Стили | CSS Modules или лёгкий utility-CSS; без тяжёлого UI-kit «на вырост» |
| HTTP | `fetch` + тонкая обёртка; без обязательного React Query на старте (можно добавить на P1 для poll) |
| Состояние auth | localStorage (base URL + token) |

Альтернативы допустимы, если сохраняют SPA + TS и не тянут лишний фреймворк.

### 3.3. Конфигурация окружения

| Переменная / настройка | Назначение |
|---|---|
| `VITE_CONTROL_API_URL` | Default base URL (например `http://127.0.0.1:8080`) |
| Dev proxy | Proxy `/v1` и `/healthz` → `API_ADDR` (`127.0.0.1:8080`), чтобы упростить CORS в dev |
| Runtime override | На экране входа оператор может задать другой base URL |

### 3.4. Запуск

Команды (UI-0b, 2026-07-19) — также в [`web/README.md`](../web/README.md):

```bash
cd web
npm install
npm run dev      # Vite; proxy /v1 и /healthz → control-api (127.0.0.1:8080 или VITE_CONTROL_API_URL)
npm run build    # статика в web/dist
npm run preview  # проверка сборки
```

Нужен запущенный `control-api` и `CONTROL_API_TOKEN` для реальных запросов (после UI-1 — через экран входа).

Production: раздача `web/dist` любым static server / reverse-proxy рядом с `control-api`. CORS: либо same-origin через proxy, либо явная настройка на API (если понадобится — отдельная задача в control-api).

---

## 4. Доменная модель для UI

Значения enum совпадают с [TZ.md](./TZ.md) §6 и `internal/store/types.go`.

### 4.1. Enums

| Поле | Значения |
|---|---|
| `node.status` | `online`, `offline`, `draining` |
| `runtime.kind` | `bot_runner`, `custom_bot` |
| `desired_state` | `running`, `stopped` |
| `actual_state` | `unknown`, `starting`, `running`, `stopping`, `stopped`, `failed`, `migrating` |
| `bot_type` | `custom`, `default`, `default_extended` |
| `channel` | `telegram`, `max` |
| `run_mode` | `webhook`, `polling` |

### 4.2. Сущности (логические поля UI)

**Node**

| Поле | Тип | Показ |
|---|---|---|
| `id` | string | да |
| `hostname` | string | да |
| `status` | enum | пилюля |
| `last_seen_at` | datetime | относительное + абсолютное |
| `agent_version` | string \| null | да |
| `meta` | object | опционально (P2) |
| `created_at` / `updated_at` | datetime | карточка |

**Bot**

| Поле | Тип | Показ |
|---|---|---|
| `id` | UUID string | да (mono) |
| `client_id` | UUID string \| null | список/фильтр |
| `name` | string | заголовок |
| `bot_type` | enum | да |
| `custom_name` | string \| null | обязателен для custom |
| `channel` | enum | да |
| `run_mode` | enum | да |
| `port` | int | да (UNIQUE глобально) |
| `token_ref` | string | **маскированный** в ответах API |
| `runtime_id` | UUID \| null | ссылка |
| `artifact_path` / `repo_url` / `start_command` | string \| null | custom |
| `desired_state` / `actual_state` | enum | пилюли |
| `assigned_node_id` | string \| null | ссылка на ноду |
| `last_error` | string \| null | плашка/tooltip |
| `config_version` | int64 | да |
| `scenario_config` | object | edit P1 |
| `created_at` / `updated_at` | datetime | карточка |

**Runtime**

| Поле | Тип | Показ |
|---|---|---|
| `id` | UUID | да |
| `kind` / `name` | enum / string | да |
| `start_command` / `workdir` | string | карточка/деталь |
| `desired_state` / `actual_state` | enum | пилюли |
| `assigned_node_id` | string \| null | да |
| `lease_owner` / `lease_until` | string / datetime \| null | да; просрочка — тревога |
| `pid` / `exit_code` | int \| null | да |
| `last_error` | string \| null | да |
| `config_version` | int64 | да |

**BotEvent**

| Поле | Тип | Показ |
|---|---|---|
| `id` | string | скрыть или mono |
| `bot_id` | string | контекст страницы |
| `type` | string | тип события |
| `message` | string | текст |
| `at` | datetime | время |
| `meta` | object | раскрытие (P1) |

### 4.3. Инварианты (UI должен знать)

1. `port` уникален во всём кластере.  
2. При `bot_type=custom` → `custom_name` обязателен и непустой; иначе `custom_name` должен быть пустым/null.  
3. `run_mode=webhook` → порт слушается процессом; `polling` → порт только зарезервирован в store.  
4. Создание `default*` не создаёт новый OS-процесс — только строку бота (+ привязка к runner). Создание `custom` — bot + dedicated runtime.  
5. Start/Stop меняют **desired**; actual обновит agent.  
6. В ответах API `token_ref` маскируется (`launch.MaskTokenRef`); plaintext токен в list/detail не показывать как полный секрет. При edit — поле «новый token_ref» (отправка только если пользователь ввёл значение).

### 4.4. Цвета статусов (обязательная легенда)

| Состояние | Цвет (смысл) |
|---|---|
| `running` / `online` | зелёный |
| `stopped` | серый |
| `failed` / unhealthy-проблема | красный |
| `migrating` / `starting` / `stopping` | янтарный |
| `unknown` / `offline` | приглушённый красный или серый |
| desired ≠ actual | лёгкая подсветка строки / бейдж «сходится…» |

---

## 5. Контракт HTTP API

Канон: [TZ.md](./TZ.md) §11, реализация `internal/api/server.go`.

### 5.1. Auth

| Правило | Деталь |
|---|---|
| Заголовок | `Authorization: Bearer <CONTROL_API_TOKEN>` |
| Без/неверный токен | `401` + `{"error":"..."}` |
| `/healthz` | **без** auth (liveness control-api) |
| `/metrics` | без auth; UI v1 **не использует** |

При `401` UI сбрасывает сессию на экран входа (или показывает баннер «токен отклонён»).

### 5.2. Формат ошибок

Все ошибки API (кроме plain `/healthz`):

```json
{ "error": "текст сообщения" }
```

| HTTP | Когда (store/ops) | Поведение UI |
|---|---|---|
| `400` | `ErrConflict` / `ErrInvalidArgument`, плохой JSON, неизвестный `bot_type` | показать у формы / toast |
| `401` | auth | выход / повторный вход |
| `404` | `ErrNotFound` | «не найдено», вернуться к списку |
| `409` | `ErrLimitExceeded` (лимит ботов на ноду) | понятное сообщение про лимит |
| `500` | прочее | общий error banner |

Тело запроса: JSON, `Content-Type: application/json`. Сервер `DisallowUnknownFields` — лишние поля → `400`.

### 5.3. Эндпоинты ↔ экраны

| Method | Path | Auth | UI |
|---|---|---|---|
| `GET` | `/healthz` | нет | индикатор API (тело `ok\n`, text/plain) |
| `GET` | `/v1/nodes` | да | ноды, migrate targets, сводка |
| `GET` | `/v1/bots` | да | список, обзор; `?client_id=` — серверный фильтр |
| `POST` | `/v1/bots` | да | создать |
| `PATCH` | `/v1/bots/{id}` | да | редактировать (P1) |
| `POST` | `/v1/bots/{id}/start` | да | Start |
| `POST` | `/v1/bots/{id}/stop` | да | Stop |
| `POST` | `/v1/bots/{id}/migrate` | да | Migrate |
| `GET` | `/v1/runtimes` | да | runtimes, сводка |
| `GET` | `/v1/bots/{id}/events` | да | лента на карточке |

Отдельного `GET /v1/bots/{id}` **нет** — карточку собирать из списка или после create/patch ответа. Если понадобится — добавить в control-api (не ходить в БД из UI).

### 5.4. Тела запросов

**POST `/v1/bots`** — create

```json
{
  "name": "client-42-main",
  "bot_type": "default",
  "custom_name": null,
  "channel": "telegram",
  "run_mode": "webhook",
  "port": 18042,
  "token_ref": "secret:bot-42",
  "assigned_node_id": "node-1",
  "desired_state": "stopped",
  "client_id": "11111111-1111-4111-8111-111111111111",
  "artifact_path": null,
  "start_command": null,
  "workdir": null
}
```

| Поле | Обязательность | Примечание |
|---|---|---|
| `name` | да | |
| `bot_type` | да | `default` \| `default_extended` \| `custom` |
| `custom_name` | да для custom | для non-custom не передавать / null |
| `channel` | нет | default API: `telegram` |
| `run_mode` | нет | default API: `webhook` |
| `port` | да | int, unique |
| `token_ref` | да | |
| `assigned_node_id` | нет | иначе нода из конфига API (`NODE_ID`) |
| `client_id` | нет | UUID клиента; omit/пусто → `null`; невалидный UUID → `400` |
| `desired_state` | нет | если `running` — API сразу вызовет start (с лимитом) |
| `artifact_path`, `start_command`, `workdir` | для custom | `start_command` обязателен по валидации custom |

Успех: `201` + объект бота (masked).

**POST `/v1/bots/{id}/start`** — тело пустое.  
Ответ `200`:

```json
{ "status": "ok", "desired": "running", "bot_id": "<id>" }
```

**POST `/v1/bots/{id}/stop`** — аналогично, `"desired": "stopped"`.

**POST `/v1/bots/{id}/migrate`**

```json
{ "to_node_id": "node-2" }
```

`to_node_id` обязателен. Ответ `200`: `{ "status": "ok", "bot_id", "to_node_id" }`.

**PATCH `/v1/bots/{id}`** (P1) — все поля опциональны:

```json
{
  "desired_state": "running",
  "token_ref": "env:TELEGRAM_BOT_TOKEN",
  "client_id": "11111111-1111-4111-8111-111111111111",
  "assigned_node_id": "node-2",
  "config_version": 2,
  "scenario_config": {}
}
```

Предпочтительно для lifecycle использовать start/stop/migrate, а не PATCH `desired_state` (меньше сюрпризов с лимитами/ops). PATCH — для `token_ref`, `client_id`, `scenario_config`, точечного assignment. Пустой `client_id` (`""`) сбрасывает поле в `null`.

### 5.5. Формат JSON в ответах

**Статус (UI-0a, 2026-07-19): выровнено.** Запросы и ответы control-api для Node/Bot/Runtime/BotEvent — канонический **snake_case**. Реализация: ответные DTO в `internal/api` (store-типы без field-level `json`-тегов; memory persist изолирован).

Запросы create/patch/migrate используют **snake_case** (`bot_type`, `to_node_id`).

| Решение для UI v1 | Статус |
|---|---|
| **A (предпочтительно)** | **Сделано (UI-0a):** ответные DTO snake_case в `internal/api` |
| **B** | Не нужен для v1 после UI-0a (normalize в client не требуется) |

**Требование ТЗ:** в коде UI типы и парсер работают по **каноническому snake_case** (как в таблицах §4).

Канонические имена полей ответов:

```text
id, hostname, status, last_seen_at, agent_version, meta, created_at, updated_at
name, bot_type, custom_name, channel, run_mode, port, token_ref, runtime_id,
artifact_path, repo_url, start_command, desired_state, actual_state,
assigned_node_id, last_error, config_version, scenario_config, client_id
kind, workdir, env, lease_owner, lease_until, pid, exit_code
bot_id, type, message, at   # events: type = тип события
```

Даты — RFC3339 strings (как отдаёт Go `time.Time`).

### 5.6. Примеры (из продуктового ТЗ)

Default:

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
  "client_id": "11111111-1111-4111-8111-111111111111"
}
```

Custom:

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

---

## 6. Информационная архитектура и маршруты

| Route | Экран | Приоритет |
|---|---|---|
| `/login` | Вход (URL + token) | P0 |
| `/` | Обзор | P0 |
| `/bots` | Список ботов | P0 |
| `/bots/new` | Создать бота | P0 |
| `/bots/:id` | Карточка бота | P0 |
| `/bots/:id/edit` | Редактировать | P1 |
| `/nodes` | Список нод | P0 |
| `/nodes/:id` | Карточка ноды | P1 |
| `/runtimes` | Список runtimes | P1 (read-only допустим уже в P0 минимально) |

Неавторизованный доступ к `/*` кроме `/login` → редирект на `/login`.

Query-фильтры на `/bots` (желательно, чтобы шарились ссылкой):

```
?bot_type=&desired_state=&actual_state=&assigned_node_id=&client_id=&channel=&runtime_id=
```

Фильтрация в v1 — **на клиенте** по `GET /v1/bots`, кроме `client_id`: его можно передать query-параметром (`GET /v1/bots?client_id=<uuid>`), сервер отдаёт только совпадения. Без параметра — полный список. Невалидный UUID → `400`.

---

## 7. Экраны и UX (подробно)

### 7.1. Оболочка

```
┌─────────────────────────────────────────────────────────────┐
│  mvp-manager          [API ● online]     token ••••  [Выйти] │
├──────────┬──────────────────────────────────────────────────┤
│ Обзор    │  заголовок                     [Обновить] [+ …]  │
│ Боты     │──────────────────────────────────────────────────│
│ Ноды     │              основной контент                    │
│ Runtimes │                                                  │
└──────────┴──────────────────────────────────────────────────┘
```

- Индикатор API: периодический `GET /healthz` (например 10–15s) или проверка при каждом запросе.  
- `401`/сеть — баннер под шапкой, списки не выглядят «просто пустыми».  
- Узкий экран: сайдбар → табы / drawer.

### 7.2. Вход (`/login`)

Поля:

1. Base URL API  
2. Bearer token (password input)  
3. Кнопка «Подключиться»

Проверка: `GET /healthz` (доступность) + пробный `GET /v1/bots` или `/v1/nodes` (валидность токена).  
Успех → сохранить в localStorage → `/`.  
Подсказка: токен = `CONTROL_API_TOKEN`.

### 7.3. Обзор (`/`)

Данные: параллельно `nodes`, `bots`, `runtimes`.

1. **Сводки (компактные строки, не огромные KPI-карточки):**
   - Ноды: N online / M offline (/ draining)
   - Боты: counts по `actual_state` (и опционально desired)
   - Runtimes: count по `kind` + число failed/unknown
2. **«Требуют внимания»:** боты с `actual_state=failed` или непустым `last_error`; ноды `offline`; runtimes `failed`. Клик → соответствующая карточка/список.
3. Глобальная лента событий — **P2** (нет API).

Критерий online для ноды: поле `status` (+ UI может дополнительно тускнеть, если `last_seen_at` старше порога, напр. 30s — конфиг константой в UI).

### 7.4. Список ботов (`/bots`)

- Фильтры: `bot_type`, `desired_state`, `actual_state`, `assigned_node_id`, `client_id`, `channel` (+ сброс).  
- Таблица:

  | name | type | channel | port | node | desired | actual | client | last_error |
  |---|---|---|---|---|---|---|---|---|

- desired≠actual → подсветка строки.  
- Клик по строке → `/bots/:id`.  
- Действия шапки: Обновить, Создать.  
- Row actions (опционально): Start / Stop (Stop → confirm).

Пусто: «Нет ботов» + CTA «Создать».

### 7.5. Карточка бота (`/bots/:id`)

**Паспорт:** все ключевые поля §4.2; desired/actual крупнее; `last_error` плашкой.

**Действия:**

| Действие | Условие показа | Confirm |
|---|---|---|
| Start | desired≠running или хотим перезапросить running | нет |
| Stop | desired=running или actual running/starting | **да** |
| Migrate… | есть другие ноды | **да** + выбор `to_node_id` |
| Редактировать | P1 | — |

После команды:

1. Кнопка loading / disabled.  
2. Toast (P1) или inline: «Команда принята; actual обновится после reconcile».  
3. Перезагрузить бота (из list) и events.

**События:** `GET /v1/bots/{id}/events` — таблица/timeline: `at` · `type` · `message`. Новые сверху. Пусто: «Событий пока нет». Фильтр по типу — P1.

### 7.6. Создать бота (`/bots/new`)

Группы полей:

1. **Идентичность:** name, bot_type, channel, client_id (опциональный UUID), (custom_name если custom)  
2. **Размещение:** assigned_node_id (select из nodes), port, run_mode  
3. **Секрет:** token_ref  
4. **Custom-only:** artifact_path, start_command, workdir  
5. **Старт:** checkbox/select desired_state `stopped` (default) | `running`

Клиентская валидация до submit:

| Правило | Сообщение (RU) |
|---|---|
| name непустой | Укажите имя |
| port целое > 0 | Некорректный порт |
| port уже есть в списке ботов | Порт уже занят (проверка на клиенте; сервер — источник истины) |
| custom без custom_name | Для custom укажите custom_name |
| custom без start_command | Укажите start_command |
| token_ref пустой | Укажите token_ref |
| client_id непустой, но не UUID | Некорректный client_id (нужен UUID) |

Успех `201` → переход на `/bots/:id`.  
Ошибка API → текст `error` у формы.

### 7.7. Редактировать (P1, `/bots/:id/edit`)

Поля PATCH: token_ref (пусто = не менять), client_id (UUID; пусто = сбросить), assigned_node_id, scenario_config (JSON editor простой), config_version только если нужно явно.  
Не дублировать Start/Stop через форму, если есть кнопки на карточке.

### 7.8. Migrate (dialog)

1. Select `to_node_id` из `GET /v1/nodes`.  
2. Сортировка: online выше offline; текущую ноду бота исключить или пометить disabled.  
3. Confirm-текст: куда переносим, что будет краткий downtime / migrating.  
4. `POST .../migrate` → закрыть dialog → обновить карточку.

### 7.9. Ноды

**Список P0:** id, hostname, status, last_seen_at (например `12s ago`), agent_version.

**Карточка P1:** паспорт + секции «Боты на ноде» / «Runtimes на ноде» (client filter по `assigned_node_id`).

### 7.10. Runtimes (P1)

Таблица: kind, name, node, desired, actual, pid, lease_owner, lease_until, last_error.  
`lease_until` в прошлом при ненулевом lease → визуальный warning.  
Клик: фильтр ботов `?runtime_id=` или карточка ноды.

### 7.11. Общие состояния

| Состояние | Поведение |
|---|---|
| Loading | skeleton или spinner в области контента, не блокировать весь chrome навсегда |
| Empty | одна фраза + CTA |
| Error | баннер + Retry |
| Stale (poll P1) | «Обновлено N с назад» без мигания таблицы |
| Confirm | modal, Esc/Отмена, destructive кнопка явно вторичной/красной |

---

## 8. Визуальный образ

Операционная консоль в духе Portainer / Nomad UI / «маленький k9s в браузере» — не маркетинговый SaaS-dashboard.

| Решение | Как выглядит |
|---|---|
| Плотность | Компактные таблицы, мало декоративного воздуха |
| Фон | Нейтральный холодный серый / graphite |
| Акцент | Один цвет (teal или steel-blue), без purple-glow |
| Шрифт | UI-sans (IBM Plex / Source Sans / Geist и т.п.) + mono для id, port, pid, hostname |
| Карточки | Только для форм/confirm/изолированных блоков; списки — таблицы |
| Тема | Одна рабочая тема в v1; dark/light switch — P2 |
| Запрещено в v1 | hero-градиенты, emoji-иконки как основа, огромные KPI со спарклайнами |

Подробные wireframe-описания экранов — §7.

---

## 9. Нефункциональные требования

| Требование | MVP UI |
|---|---|
| Язык интерфейса | Русский (строки в коде или простой словарь; полноценный i18n-framework — по желанию) |
| Браузеры | Последние Chrome / Firefox / Safari |
| Адаптив | Desktop-first; узкий экран usable (табы, горизонтальный скролл таблиц) |
| Безопасность | Token только в localStorage/memory вкладки; не логировать token; HTTPS на prod — зона деплоя |
| Производительность | Кластер маленький (единицы–десятки ботов); полный list в памяти ок |
| A11y | Клавиатура на формах и dialog; focus trap в modal; контраст статусов не только цветом (текст) |
| Тесты | Минимум: unit на парсер/фильтры; e2e — later (P2) |
| Логирование UI | console только для неожиданных ошибок; без PII токенов |

---

## 10. Структура `web/` (целевая)

```
web/
  package.json
  vite.config.ts
  index.html
  src/
    main.tsx
    App.tsx
    routes.tsx
    api/
      client.ts          # fetch + auth header + normalize
      types.ts           # snake_case типы
      endpoints.ts
    auth/
      session.ts         # localStorage
      LoginPage.tsx
    layout/
      AppShell.tsx
      StatusPill.tsx
    pages/
      OverviewPage.tsx
      BotsListPage.tsx
      BotDetailPage.tsx
      BotCreatePage.tsx
      BotEditPage.tsx      # P1
      NodesListPage.tsx
      NodeDetailPage.tsx   # P1
      RuntimesPage.tsx     # P1
    components/
      ConfirmDialog.tsx
      DataTable.tsx
      EmptyState.tsx
      ErrorBanner.tsx
      FiltersBar.tsx
    styles/
      tokens.css
      global.css
  README.md
```

Имена можно уточнять при scaffold; ответственность слоёв сохранить: **api / pages / layout**.

---

## 11. Приоритеты фич

### P0 — первый usable UI

- [x] Scaffold Vite+React+TS в `web/` (UI-0b, 2026-07-19)  
- [x] Login (URL + token) + session (UI-1, 2026-07-19)  
- [x] App shell + индикатор `/healthz` (UI-1, 2026-07-19)  
- [x] Обзор (сводки + проблемы) (UI-2, 2026-07-19)  
- [x] Список ботов + фильтры + refresh (UI-2, 2026-07-19)  
- [x] Карточка бота + events (UI-3, 2026-07-19)  
- [x] Create bot (UI-4, 2026-07-19)  
- [x] Start / Stop (+ confirm stop) (UI-4, 2026-07-19)  
- [x] Migrate (+ выбор ноды, confirm) (UI-5, 2026-07-19)  
- [x] Список нод (UI-2, 2026-07-19)  
- [ ] Документация запуска в `web/README.md`  
- [x] Выравнивание JSON snake_case (API задача A, UI-0a, 2026-07-19)

### P1 — паритет с типичным ctl + удобство

- [x] PATCH / edit bot (UI-6.2, 2026-07-19)  
- [x] Карточка ноды (UI-6.1, 2026-07-19)  
- [x] Runtimes list + связь с ботами (UI-6.1, 2026-07-19)  
- [x] Toasts (UI-6.3, 2026-07-19)  
- [x] Авто-poll 3–10s на списках/карточке (UI-6.3, 2026-07-19)  
- [ ] Фильтр событий по типу  
- [ ] Улучшение пустых/error состояний  

### P2 — later

- [ ] Dark/light  
- [ ] Дублирование бота  
- [ ] Глобальная лента событий (нужен API)  
- [ ] Handoff wizard  
- [ ] Embed статики в control-api  
- [ ] E2E tests  

---

## 12. Зависимости от control-api

| Gap | Нужно для UI | Действие |
|---|---|---|
| ~~PascalCase в JSON ответов~~ | — | **Закрыто UI-0a:** DTO snake_case в `internal/api` |
| Нет `GET /v1/bots/{id}` | Карточка без полного list | Опционально добавить; иначе filter list |
| Нет серверных фильтров list | Ок для MVP | Client-side |
| Нет глобальных events | Обзор P2 | Не блокирует v1 |
| CORS при раздельных origin | Dev/prod | Vite proxy в dev; prod same-origin или CORS на API |

Недостающие возможности добавляются в **control-api**, не обходом в БД.

---

## 13. Критерии приёмки

### 13.1. UI v1 (P0)

1. Подключение к `control-api` с токеном; неверный токен → понятная ошибка.  
2. Список ботов + карточка + лента событий.  
3. Create + start + stop (stop с подтверждением).  
4. Migrate с выбором ноды (confirm).  
5. Список нод.  
6. Обзор: counts + проблемные сущности.  
7. Runtimes хотя бы доступны read-only (отдельная страница или блок) — желательно в P0, обязательно не позже P1.  
8. Запуск `web/` задокументирован.  
9. UI на русском.  
10. После start/stop UI не врёт, что actual уже совпал с desired без обновления данных.

### 13.2. Ручной сценарий проверки

```text
1. Поднять control-api + agent + store (как в корневом README).
2. cd web && npm run dev
3. Войти с CONTROL_API_TOKEN и base URL.
4. Обзор: видны сиды/ноды (если есть).
5. Создать default-бота (stopped), открыть карточку.
6. Start → desired=running; дождаться actual=running (или failed + last_error).
7. Stop → confirm → desired=stopped.
8. Migrate на другую ноду (если есть 2 ноды) или убедиться, что dialog показывает список.
9. Открыть события бота — есть записи после действий.
10. Список нод отображает last_seen / status.
```

---

## 14. Порядок внедрения (для `/manager`)

Подробный план блоков, handoff-шаблоны и критерии tester — **[FRONTEND_PLAN.md](./FRONTEND_PLAN.md)**.  
Чеклист статусов — [IMPLEMENTATION_PLAN.md](./IMPLEMENTATION_PLAN.md) Phase UI.

Выдавать **по одному блоку**, не «весь frontend сразу»:

| Блок | Содержание |
|---|---|
| UI-0a | snake_case JSON в ответах control-api |
| UI-0b | Scaffold `web/` + README + api client |
| UI-1 | Auth + shell + healthz indicator |
| UI-2 | Nodes list + Bots list (read-only) + Overview |
| UI-3 | Bot detail + events |
| UI-4 | Create + start/stop |
| UI-5 | Migrate |
| UI-6 | Runtimes + edit/patch + poll/toasts (P1) |

Цепочка: manager → developer → tester → отчёт вам → «Принято вами» → следующий блок.

---

## 15. Решения, зафиксированные для frontend

| Вопрос | Решение |
|---|---|
| Где код | `web/` в monorepo |
| Чем ходим | Только `control-api` |
| Стек | Vite + React + TypeScript (ориентир) |
| Auth | Bearer token, localStorage |
| Фильтры list | Client-side в v1 |
| Язык UI | Русский |
| Визуал | Ops-console, таблицы, статус-пилюли |
| Lifecycle | Через start/stop/migrate, не «тихий» SQL |
| Секреты | Показывать masked `token_ref`; полный секрет не логировать |
| JSON | Канон snake_case; выровнять API или normalize в client |

---

## 16. История документа

| Версия | Дата | Изменение |
|---|---|---|
| 0.1 | 2026-07 | Краткий scope UI, фичи P0–P2, визуальный набросок |
| 1.0 | 2026-07-19 | Полное ТЗ: модель, API, экраны, валидации, приёмка, фазы |
| 1.0.1 | 2026-07-19 | §14 ссылается на FRONTEND_PLAN.md |
