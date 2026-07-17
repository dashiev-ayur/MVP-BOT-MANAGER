// Package storeopen — тонкий wiring выбора реализации store по конфигу.
//
// Вынесен из cmd/*, чтобы agent и ctl не дублировали switch memory/postgres.
// Бизнес-пакеты (reconcile и т.п.) этот пакет не импортируют — только main.
package storeopen

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"mvp-manager/internal/config"
	"mvp-manager/internal/store"
	"mvp-manager/internal/store/memory"
	"mvp-manager/internal/store/postgres"
	"mvp-manager/internal/watch"
)

// Open создаёт фасад store.Stores по значению STORE из конфига.
//
//   - memory → file-backed (или RAM, если MemoryStorePath пуст);
//   - postgres → pgxpool по DATABASE_URL (схема должна быть уже применена migrate).
//
// Второй результат — каноническое имя бэкенда для логов (store=memory|postgres).
// Caller обязан вызвать Stores.Close() (postgres закрывает пул; memory — no-op).
func Open(cfg config.Config) (store.Stores, string, error) {
	switch cfg.Store {
	case config.StoreMemory:
		st, err := memory.Open(cfg.MemoryStorePath)
		if err != nil {
			return store.Stores{}, "", fmt.Errorf("open memory store: %w", err)
		}
		return st.AsStores(), config.StoreMemory, nil

	case config.StorePostgres:
		if cfg.DatabaseURL == "" {
			return store.Stores{}, "", fmt.Errorf(
				"%s=%s требует %s (DSN PostgreSQL)",
				config.EnvStore, config.StorePostgres, config.EnvDatabaseURL,
			)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		st, err := postgres.Open(ctx, cfg.DatabaseURL)
		if err != nil {
			return store.Stores{}, "", fmt.Errorf("open postgres store: %w", err)
		}
		return st.AsStores(), config.StorePostgres, nil

	default:
		return store.Stores{}, "", fmt.Errorf("неизвестный store %q", cfg.Store)
	}
}

// OpenWatcher выбирает ChangeWatcher по STORE:
//
//   - memory + path → FileWatcher (mtime JSON);
//   - postgres → LISTEN/NOTIFY (отдельное соединение); при ошибке — Nop + warn;
//   - иначе → Nop.
//
// pgx остаётся только здесь и в internal/store/postgres — не в reconcile.
func OpenWatcher(cfg config.Config, log *slog.Logger) watch.ChangeWatcher {
	if log == nil {
		log = slog.Default()
	}
	switch cfg.Store {
	case config.StoreMemory:
		return watch.OpenForStore(config.StoreMemory, cfg.MemoryStorePath, log)
	case config.StorePostgres:
		w, err := postgres.NewListenWatcher(cfg.DatabaseURL, log)
		if err != nil {
			log.Warn("postgres LISTEN/NOTIFY недоступен, reconcile на poll", "err", err)
			return watch.NewNop()
		}
		return w
	default:
		return watch.NewNop()
	}
}
