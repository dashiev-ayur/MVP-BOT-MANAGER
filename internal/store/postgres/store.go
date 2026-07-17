// Package postgres — реализация store на PostgreSQL (Phase PG).
//
// Бизнес-пакеты (reconcile/supervisor/runner/ops) этот пакет не импортируют:
// только wiring (storeopen / cmd/migrate). Контракт — те же интерфейсы
// internal/store, что и у memory.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"mvp-manager/internal/store"
)

// Store — фасад репозиториев над *pgxpool.Pool.
type Store struct {
	pool *pgxpool.Pool

	Nodes    *NodeRepo
	Runtimes *RuntimeRepo
	Bots     *BotRepo
	Events   *EventRepo
}

// Compile-time проверка контракта.
var (
	_ store.NodeRepository    = (*NodeRepo)(nil)
	_ store.RuntimeRepository = (*RuntimeRepo)(nil)
	_ store.BotRepository     = (*BotRepo)(nil)
	_ store.EventRepository   = (*EventRepo)(nil)
)

// Open создаёт пул и репозитории по DSN (postgres://…).
func Open(ctx context.Context, databaseURL string) (*Store, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("postgres store: empty DATABASE_URL")
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("postgres pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	return newStore(pool), nil
}

func newStore(pool *pgxpool.Pool) *Store {
	st := &Store{pool: pool}
	st.Nodes = &NodeRepo{pool: pool}
	st.Runtimes = &RuntimeRepo{pool: pool}
	st.Bots = &BotRepo{pool: pool}
	st.Events = &EventRepo{pool: pool}
	return st
}

// Close закрывает пул соединений.
func (s *Store) Close() {
	if s != nil && s.pool != nil {
		s.pool.Close()
	}
}

// AsStores возвращает общий фасад для cmd/storeopen.
func (s *Store) AsStores() store.Stores {
	return store.Stores{
		Nodes:    s.Nodes,
		Runtimes: s.Runtimes,
		Bots:     s.Bots,
		Events:   s.Events,
		Closer:   closerFunc(s.Close),
	}
}

// closerFunc адаптирует func() к io.Closer.
type closerFunc func()

func (f closerFunc) Close() error {
	f()
	return nil
}

// Pool возвращает пул (для migrate/seed/тестов внутри пакета).
func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}
