# Как работать с проектом

Кратко: вы даёте **manager** одно задание → он ставит задачи **developer** → **tester** проверяет → manager отчитывается вам и пишет, **как проверить вручную**. Следующий этап — только после вашего «ок».

Документы:

| Файл | Зачем |
|---|---|
| [TZ.md](./TZ.md) | Что строим |
| [IMPLEMENTATION_PLAN.md](./IMPLEMENTATION_PLAN.md) | Чеклист фаз; manager ставит `[x]` |

Агенты: `.cursor/agents/` (`manager`, `developer`, `tester`).  
Команда: `.cursor/commands/manager.md` → в чате **`/manager`**.

---

## Хранилище данных

Бизнес-логика (**reconcile**, supervisor, runner) зависит только от **интерфейсов** `internal/store`, не от конкретной БД (SOLID / DIP).

| Этап | Реализация | Персистентность |
|---|---|---|
| Сейчас (Phase 0–2) | **In-memory** (`STORE=memory` + файл) | Пока живы процессы; общий JSON |
| Позже (Phase PG) | **PostgreSQL** (`STORE=postgres`) | На диске / общий сервер |

Переключение — конфигом, без переписывания бизнес-логики.

### Сейчас (in-memory, Phase 2)

Отдельный Postgres **не нужен**. `STORE=memory` по умолчанию.

Общий JSON (`MEMORY_STORE_PATH`, по умолчанию `.mvp-manager/store.json`) обязателен для **agent**, **ctl**, **bot-runner** и **healthcheck**. Без одного пути процессы не разделяют bots/runtimes.

**После Phase 2 (2.1–2.4 + 2.5):** multi-tenant `bot-runner`, `healthcheck`, custom Phase 1; сценарий `default` отвечает на `/start` в **Telegram** (polling / webhook) и **Max**; `token_ref` резолвится из значения или ENV (`env:NAME` / `$NAME`). Образец ENV — [`.env.example`](../.env.example). Полные команды и live-проверка Telegram — корневой [`README.md`](../README.md), `scripts/manual-telegram-start.sh`.

```bash
go build -o bin/agent ./cmd/agent
go build -o bin/ctl ./cmd/ctl
go build -o bin/bot-runner ./cmd/bot-runner
go build -o bin/healthcheck ./cmd/healthcheck
export NODE_ID=node-1 STORE=memory MEMORY_STORE_PATH=.mvp-manager/store.json
export BOT_RUNNER_COMMAND="$(pwd)/bin/bot-runner"
./scripts/e2e-phase2.sh
./scripts/e2e-phase1.sh
go test ./internal/messenger/... ./internal/launch/... ./internal/runner/...
# live Telegram (нужен токен бота):
# TELEGRAM_BOT_TOKEN=... ./scripts/manual-telegram-start.sh
```

`STORE=postgres` пока отклоняется с понятной ошибкой (Phase PG), без dial БД.

### Позже (PostgreSQL) — Phase PG

Когда пользователь даст инструкции по запуску БД (или попросит локальный compose):

```bash
# пример для локального compose (появится в Phase PG)
docker compose up -d
go run ./cmd/migrate up
# STORE=postgres DATABASE_URL=... ./agent
```

Одна общая PostgreSQL на ноды (1–3 сервера) — целевая схема для multi-node.

| Кто | Роль |
|---|---|
| Docker / админ / облако | запускает Postgres (по вашим инструкциям) |
| `cmd/migrate` | схема (Phase PG) |
| `agent`, `ctl`, … | работают через store-интерфейс |

Go-модуль: **`mvp-manager`**. `bot-runner` — в этом же репозитории.

---

## Как вызвать manager

### Способ 1 — команда `/manager` (рекомендуется)

1. Откройте **Agent** chat в Cursor.
2. Введите `/` и выберите **`manager`** (или наберите `/manager`).
3. **В том же сообщении** допишите задание.

Примеры:

```text
/manager Сделай Phase 0, блок 0.1 (Go-модуль и структура каталогов)
```

```text
/manager Phase 2.5 (Telegram + Max для default)
```

```text
/manager Пользователь принял Phase 2.5 — отметь «Принято вами»
```

```text
/manager Phase PG — локальный docker compose Postgres
```

### Способ 2 — без слэша

В Agent chat обычным текстом:

```text
Используй manager: сделай Phase 0.2 (конфиг STORE=memory)
```

или:

```text
/manager review … 
```

если в меню появляется субагент **manager** из `.cursor/agents/manager.md` — тоже нормально: смысл тот же (роль менеджера этапа).

---

## Как формулировать задание

**Хорошо**

- одна фаза: `Phase 0`
- один блок: `Phase 0.3`
- несколько соседних пунктов одной фазы: `Phase 1.1 и 1.2`
- после вашей проверки: `Phase 0 принята, отметь в плане`

**Плохо**

- `сделай весь MVP`
- `Phase 0–4`
- `и сразу Telegram и migrate на 3 сервера`

Правила процесса — в плане (легенда и сводка) и в `.cursor/agents/manager.md`.

---

## Что будет дальше

```
вы  --/manager-->  manager  -->  developer  -->  tester
                      ^                            |
                      |---- доработка (макс. 2) ---|
                      |
                      v
              отчёт вам + «Как проверить»
                      |
              ваше «ок» / замечания
                      |
         manager отмечает «Принято вами»
```

В отчёте manager должны быть:

1. что сделано и какие пункты плана стали `[x]`;
2. вердикт tester;
3. пошагово **как проверить вручную**;
4. что не входило в задание.

---

## Другие роли (обычно сами не вызываете)

| Команда / агент | Кто зовёт | Зачем |
|---|---|---|
| `developer` | manager | пишет код по handoff |
| `tester` | manager | проверяет, REWORK или PASS |

Вам достаточно общаться с **manager**.

---

## Если `/manager` не видна в меню

1. Убедитесь, что открыт корень проекта `mvp-manager` (где лежит `.cursor/`).
2. Есть файлы:
   - `.cursor/commands/manager.md` — slash-команда;
   - `.cursor/agents/manager.md` — роль/субагент.
3. Новый Agent chat → снова `/`.
4. Запасной вариант — способ 2 (текст «Используй manager: …»).
