# Handoff: single-bot без менеджера

Этот каталог описывает выдачу бота клиенту в режиме **одного процесса** —
без `agent`, `bot-runner` multi-tenant и общего store менеджера.

Ориентир: launch contract в [`docs/TZ.md`](../TZ.md) §9.

## Что отдаём клиенту

1. Исходники бота (custom-репозиторий **или** шаблон сценария default / default_extended).
2. Файл [`.env.example`](./.env.example) с переменными launch contract.
3. Краткую инструкцию запуска (этот README).

Утилита `cmd/handoff` (автосборка архива) — опциональна и может появиться в Phase 4.

## Launch contract (ENV)

| Переменная | Обязательно | Описание |
|---|---|---|
| `PORT` | да | порт HTTP (webhook) или зарезервированный номер (polling) |
| `BOT_TOKEN` | да | токен Telegram / Max |
| `BOT_MODE` | да | `webhook` или `polling` |
| `CHANNEL` | да | `telegram` или `max` |
| `PUBLIC_URL` | нет | публичный HTTPS для setWebhook |

## Пример запуска (Go custom)

```bash
cp .env.example .env
# отредактируйте BOT_TOKEN, PORT, …
set -a && source .env && set +a
./bot
# или: go run .
```

При `BOT_MODE=webhook` процесс должен отвечать на `GET /healthz` на `PORT`.

## Связь с mvp-manager

В кластере менеджер сам подставляет те же ENV при старте custom-процесса
(`internal/launch`). Handoff — тот же контракт, но клиент запускает бинарник
самостоятельно, без reconcile/lease.
