// Package store — порт хранилища: доменные типы и узкие интерфейсы (SOLID / DIP).
//
// # Назначение (Phase 0.3)
//
// Пакет описывает контракт доступа к сущностям ТЗ §6 (nodes, runtimes, bots)
// без привязки к конкретной реализации. Бизнес-логика (reconcile, supervisor,
// ctl-сценарии) зависит только от этих интерфейсов и типов.
//
// # Инвариант DIP
//
//   - Здесь нет импортов database/sql, jackc/pgx и прочих SQL-драйверов.
//   - Реализации живут в отдельных пакетах: memory (Phase 0.4), postgres (Phase PG).
//   - Выбор реализации (STORE=memory|postgres) — только в wiring cmd/*, не в домене.
//
// # Интерфейсы (ISP)
//
// Узкие репозитории по зонам ответственности:
//
//   - NodeRepository    — регистрация ноды, heartbeat, чтение
//   - RuntimeRepository — OS-процессы (bot_runner / custom_bot), desired/actual, lease
//   - BotRepository     — логические боты, desired/actual, выборки по node/runtime/client
//
// Методы рассчитаны на будущий reconcile/ctl Phase 1 (CRUD/list, смена desired/actual,
// heartbeat), без раздувания «на всё будущее».
//
// # Реализации
//
// In-memory (mutex) появится в Phase 0.4 в package memory.
// PostgreSQL-адаптер — в Phase PG (internal/store/postgres).
package store
