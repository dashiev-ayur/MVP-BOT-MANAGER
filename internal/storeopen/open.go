// Package storeopen — тонкий wiring выбора реализации store по строке STORE.
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
//   - memory → готовый in-memory фасад;
//   - postgres → понятная ошибка «пока не реализовано» (Phase PG), без dial БД.
//
// Второй результат — каноническое имя бэкенда для логов (store=memory).
func Open(storeKind string) (*memory.Store, string, error) {
	switch storeKind {
	case config.StoreMemory:
		return memory.New(), config.StoreMemory, nil
	case config.StorePostgres:
		// Явно не трогаем DATABASE_URL и не импортируем pgx — Phase PG.
		return nil, "", fmt.Errorf(
			"%s=%s пока не реализован (Phase PG): используйте %s=%s",
			config.EnvStore, config.StorePostgres,
			config.EnvStore, config.StoreMemory,
		)
	default:
		// config.Load уже отсекает неизвестные значения; защита на границе wiring.
		return nil, "", fmt.Errorf("неизвестный store %q", storeKind)
	}
}
