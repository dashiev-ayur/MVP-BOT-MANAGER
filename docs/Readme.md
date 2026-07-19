# Как работать с проектом

Кратко: вы даёте **manager** одно задание → он ставит задачи **developer** → **tester** проверяет → manager отчитывается вам и пишет, **как проверить вручную**. Следующий этап — только после вашего «ок».

Документы:

| Файл | Зачем |
|---|---|
| [TZ.md](./TZ.md) | Что строим |
| [IMPLEMENTATION_PLAN.md](./IMPLEMENTATION_PLAN.md) | Чеклист фаз; manager ставит `[x]` |
| [frontend.md](./frontend.md) | ТЗ UI-админки `web/`: экраны, API-контракт, приёмка |

Агенты: `.cursor/agents/` (`manager`, `developer`, `tester`).  
Команда: `.cursor/commands/manager.md` → в чате **`/manager`**.

---

## Хранилище данных

Бизнес-логика (**reconcile**, supervisor, runner) зависит только от **интерфейсов** `internal/store`, не от конкретной БД (SOLID / DIP).

| Этап | Реализация | Персистентность |
|---|---|---|
| Memory | **In-memory** (`STORE=memory` + файл) | Пока живы процессы; общий JSON |
| Postgres (Phase PG) | **PostgreSQL** (`STORE=postgres`) | Docker Compose; в `.env` / `.env.example` — **по умолчанию** |

Переключение — конфигом (`STORE`, `DATABASE_URL` / `POSTGRES_*`), без переписывания бизнес-логики.  
Compose читает `POSTGRES_*` из `.env` (согласованы с `DATABASE_URL`). Без `source .env` у Go остаётся `DefaultStore=memory`.

### Сейчас (Phase PG — ✅ принята)

```bash
# .env / .env.example: STORE=postgres + POSTGRES_* + DATABASE_URL*
docker compose up -d          # credentials из .env
set -a && source .env && set +a
go run ./cmd/migrate up
go run ./cmd/migrate seed         # 2+1 default, desired=stopped; идемпотентно
go run ./cmd/ctl bots list        # STORE=postgres из .env
go run ./cmd/agent
./scripts/e2e-phase-pg.sh         # только mvp_manager_e2e
STORE=memory ./scripts/e2e-phase1.sh   # memory e2e задают STORE явно
```

| Шаг | Команда | Когда |
|---|---|---|
| Поднять БД | `docker compose up -d` | `mvp_manager` + `mvp_manager_e2e` |
| Миграции | `go run ./cmd/migrate up` | e2e: `migrate up --e2e` |
| Сиды | `go run ./cmd/migrate seed` | вручную в **dev** |
| Приложение | `STORE=postgres` + `DATABASE_URL` | |
| E2E | `./scripts/e2e-phase-pg.sh` | не трогает dev-сиды |

UUID сидов: клиент A `11111111-1111-4111-8111-111111111111` (2 бота), клиент B `22222222-2222-4222-8222-222222222222` (1 бот). Таблицы `clients` нет. Подробности — [IMPLEMENTATION_PLAN.md](./IMPLEMENTATION_PLAN.md) Phase PG, корневой [`README.md`](../README.md).

Memory по-прежнему: `STORE=memory` (см. корневой README).

Одна общая PostgreSQL на ноды (1–3 сервера) — целевая схема для multi-node.

| Кто | Роль |
|---|---|
| Docker Compose | запускает Postgres |
| `cmd/migrate` | схема + (отдельно) сиды |
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
/manager Phase 4 целиком (hardening)
```

```text
/manager Пользователь принял Phase PG — отметь «Принято вами»
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
