# web/

SPA-админка mvp-manager (Phase UI). Клиент ходит **только** в `control-api` по HTTP (Bearer).

Документация:

- ТЗ UI: [`docs/frontend.md`](../docs/frontend.md)
- План блоков: [`docs/FRONTEND_PLAN.md`](../docs/FRONTEND_PLAN.md)

## Требования

- Node.js 20+ (рекомендуется LTS)
- Запущенный `control-api` на `127.0.0.1:8080` (или свой URL)
- Токен оператора: переменная окружения **`CONTROL_API_TOKEN`** на стороне API (тот же секрет вводите на экране входа — UI-1)

Секрет токена в логах UI не печатаем.

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

Маршруты: `/login`, `/`, `/bots`, `/nodes`, `/runtimes`, `/bots/:id`.

## Сборка и preview

```bash
npm run build    # статика в web/dist
npm run preview  # локальная проверка сборки
```

Production: раздавайте `web/dist` static server / reverse-proxy рядом с `control-api` (same-origin или CORS — см. `docs/frontend.md` §3.4).

## Структура (кратко)

```
src/
  api/       # client, types (snake_case), endpoints
  auth/      # session + LoginPage (заглушка)
  layout/    # AppShell (заглушка)
  pages/     # экраны (пока заглушки)
  styles/    # tokens + global
```
