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
| Сейчас (Phase 0–2) | **In-memory** (`STORE=memory`) | Пока жив процесс агента |
| Позже (Phase PG) | **PostgreSQL** (`STORE=postgres`) | На диске / общий сервер |

Переключение — конфигом, без переписывания бизнес-логики.

### Сейчас (in-memory)

Отдельный Postgres **не нужен**. Достаточно запустить `agent` / `ctl` с `STORE=memory` (значение по умолчанию после Phase 0).

Данные сбросятся при рестарте процесса — для ранних этапов это ожидаемо.

**После Phase 0 (0.1–0.4):** модуль `mvp-manager`, конфиг из ENV, порт `internal/store` + реализация `internal/store/memory`, wiring `STORE=memory` в `agent`/`ctl`. Образец ENV — [`.env.example`](../.env.example). Команды и таблица ENV — в корневом [`README.md`](../README.md).

```bash
go build -o bin/agent ./cmd/agent
go build -o bin/ctl ./cmd/ctl
export NODE_ID=node-1 STORE=memory
./bin/agent          # slog: store=memory, регистрация ноды; ждёт SIGINT/SIGTERM
# Ctrl+C или: kill -TERM <pid>  → тихий выход
go test ./internal/config/... ./internal/store/...
```

`STORE=postgres` пока отклоняется с понятной ошибкой (Phase PG), без dial БД. Reconcile / supervisor / `ctl bots*` — Phase 1.

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
/manager Phase 0 целиком (0.1–0.4, in-memory store), потом отчёт как проверить
```

```text
/manager Только порт store + memory: Phase 0.3 и 0.4
```

```text
/manager Пользователь принял Phase 0 — отметь «Принято вами» и обнови сводку
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
