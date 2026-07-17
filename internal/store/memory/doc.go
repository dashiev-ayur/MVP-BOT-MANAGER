// Package memory — потокобезопасная реализация интерфейсов
// store.NodeRepository, store.RuntimeRepository и store.BotRepository.
//
// Режимы:
//   - New() — только RAM процесса (юнит-тесты);
//   - Open(path) — JSON-файл с flock: agent и ctl делят одно состояние
//     при STORE=memory (критично для Phase 1 E2E).
//
// Инварианты ТЗ §6 (UNIQUE port, UNIQUE runtime name, custom_name ↔ bot_type)
// проверяются при Create/Update и возвращают sentinel-ошибки package store.
//
// Wiring: STORE=memory в cmd/agent и cmd/ctl. Postgres — Phase PG.
package memory
