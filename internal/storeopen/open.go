// Package storeopen — тонкий wiring выбора реализации store по конфигу.
//
// Вынесен из cmd/*, чтобы agent и ctl не дублировали switch memory/postgres.
// Бизнес-пакеты (reconcile и т.п.) этот пакет не импортируют — только main.
package storeopen

import (
	"fmt"

	"mvp-manager/internal/config"
	"mvp-manager/internal/store/memory"
)

// Open создаёт store по значению STORE из конфига.
//
//   - memory → file-backed (или RAM, если MemoryStorePath пуст);
//   - postgres → понятная ошибка «пока не реализовано» (Phase PG), без dial БД.
//
// Второй результат — каноническое имя бэкенда для логов (store=memory).
func Open(cfg config.Config) (*memory.Store, string, error) {
	switch cfg.Store {
	case config.StoreMemory:
		st, err := memory.Open(cfg.MemoryStorePath)
		if err != nil {
			return nil, "", fmt.Errorf("open memory store: %w", err)
		}
		return st, config.StoreMemory, nil
	case config.StorePostgres:
		// Явно не трогаем DATABASE_URL и не импортируем pgx — Phase PG.
		return nil, "", fmt.Errorf(
			"%s=%s пока не реализован (Phase PG): используйте %s=%s",
			config.EnvStore, config.StorePostgres,
			config.EnvStore, config.StoreMemory,
		)
	default:
		// config.Load уже отсекает неизвестные значения; защита на границе wiring.
		return nil, "", fmt.Errorf("неизвестный store %q", cfg.Store)
	}
}
