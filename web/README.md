# web/

SPA-админка mvp-manager (Phase UI). Клиент ходит **только** в `control-api` по HTTP (Bearer).

Документация:

- ТЗ UI: [`docs/frontend.md`](../docs/frontend.md)
- План блоков: [`docs/FRONTEND_PLAN.md`](../docs/FRONTEND_PLAN.md)

## Требования

- Node.js 20+ (рекомендуется LTS)
- Запущенный `control-api` на `127.0.0.1:8080` (или свой URL)
- Токен оператора — см. ниже

Секрет токена в логах UI не печатаем. В репозитории токен **не зашит**.

## Токен (`CONTROL_API_TOKEN`)

Токен задаётся **только** при запуске `control-api` переменной окружения. На экране входа UI в поле **Bearer token** нужно ввести **то же значение**.

| Где | Что |
|---|---|
| API | `export CONTROL_API_TOKEN=…` перед `go run ./cmd/control-api` |
| UI (форма входа) | то же значение в «Bearer token» |
| Образец | [`.env.example`](../.env.example) (строка закомментирована; `.env` сам по себе не подхватывается) |
| После входа | сессия в `localStorage` (`mvp-manager.session`), не в git |

Без `CONTROL_API_TOKEN` все `/v1/*` отвечают **401**.

## Проверка вручную (два терминала)

Оба процесса оставьте работать (не останавливайте Ctrl+C, пока проверяете UI).

**Терминал 1** — из корня репозитория:

```bash
export NODE_ID=node-1
export DATABASE_URL='postgres://mvp:mvp@127.0.0.1:5432/mvp_manager?sslmode=disable'
export CONTROL_API_TOKEN=dev-token
# STORE по умолчанию postgres; для офлайна: STORE=memory
go run ./cmd/control-api
```

Дождитесь лога `control-api слушает addr=127.0.0.1:8080`.

**Терминал 2** — UI:

```bash
cd web
npm install   # один раз
npm run dev
```

Откройте URL Vite (обычно `http://localhost:5173/`). На экране входа:

| Поле | Значение |
|---|---|
| Base URL API | **пусто** (dev-proxy на `:8080`) |
| Bearer token | `dev-token` (или ваш `CONTROL_API_TOKEN`) |

Если меняли токен на API — в форме указывайте новое значение. После «Выйти» сессия очищается.

Для Start/Stop/Migrate с реальным `actual_state` нужен ещё `agent` (см. корневой [`README.md`](../README.md)).

## Установка

```bash
cd web
npm install
# или: npm ci   # если есть package-lock.json
```

## Разработка

```bash
npm run dev
```

Vite поднимет dev-server. Запросы к `/v1` и `/healthz` проксируются на control-api:

| Настройка | Значение по умолчанию |
|---|---|
| Proxy target | `http://127.0.0.1:8080` |
| Переопределение | `VITE_CONTROL_API_URL` (например `http://127.0.0.1:8080`) |

Пример:

```bash
VITE_CONTROL_API_URL=http://127.0.0.1:8080 npm run dev
```

На экране входа в **dev** поле Base URL лучше оставить пустым (proxy на `:8080`).
Если указать `http://127.0.0.1:8080` напрямую — нужен CORS на control-api (уже включён)
или тот же пустой Base URL (UI сам сведёт localhost:8080 к proxy).

Маршруты:

| Путь | Экран |
|---|---|
| `/login` | вход (Bearer) |
| `/` | обзор |
| `/bots` | список ботов |
| `/bots/new` | создать бота |
| `/bots/:id` | карточка бота (Start/Stop/Migrate) |
| `/bots/:id/edit` | PATCH-редактирование |
| `/nodes` | список нод |
| `/nodes/:id` | карточка ноды |
| `/runtimes` | список runtimes |

**UX (P1):** на списках (обзор/боты/ноды/runtimes) и карточке бота — авто-poll ~5s без мигания таблицы и индикатор «Обновлено N с назад»; на команды (start/stop/migrate/create/patch) — toasts об успехе/ошибке.

## Сборка и preview

```bash
npm run build    # статика в web/dist
npm run preview  # локальная проверка сборки
```

Production: раздавайте `web/dist` static server / reverse-proxy рядом с `control-api` (same-origin или CORS — см. `docs/frontend.md` §3.4).

## Структура (кратко)

```
src/
  api/       # client, types (snake_case), endpoints, mutations
  auth/      # session + LoginPage
  layout/    # AppShell, health indicator
  pages/     # обзор, боты, ноды, runtimes, карточка
  toast/     # простой стек toasts (без внешней lib)
  lib/       # useFetchList (+ poll), formatters
  styles/    # tokens + global
```
