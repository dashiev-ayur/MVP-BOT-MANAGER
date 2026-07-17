// Package memory — потокобезопасная in-memory реализация интерфейсов
// store.NodeRepository, store.RuntimeRepository и store.BotRepository.
//
// Данные живут только в памяти процесса: рестарт агента/ctl обнуляет состояние.
// Инварианты ТЗ §6 (UNIQUE port, UNIQUE runtime name, custom_name ↔ bot_type)
// проверяются при Create/Update и возвращают sentinel-ошибки package store.
//
// Wiring: STORE=memory в cmd/agent и cmd/ctl (Phase 0.4). Postgres — Phase PG.
package memory
