package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"mvp-manager/internal/watch"
)

// Каналы NOTIFY из миграции 00001_schema.sql.
const (
	channelBotChanges     = "bot_changes"
	channelRuntimeChanges = "runtime_changes"
)

// ListenWatcher слушает pg_notify на bot_changes / runtime_changes
// и будит reconcile через watch.ChangeWatcher.
//
// Использует отдельное соединение (LISTEN держит conn занятым).
type ListenWatcher struct {
	ch     chan struct{}
	cancel context.CancelFunc
	log    *slog.Logger
}

// NewListenWatcher поднимает LISTEN на DSN. При ошибке dial возвращает err
// (caller может откатить к NopWatcher).
func NewListenWatcher(databaseURL string, log *slog.Logger) (*ListenWatcher, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, fmt.Errorf("listen watcher: empty DATABASE_URL")
	}
	if log == nil {
		log = slog.Default()
	}

	ctx, cancel := context.WithCancel(context.Background())
	w := &ListenWatcher{
		ch:     make(chan struct{}, 1),
		cancel: cancel,
		log:    log,
	}

	// Отдельный пул MaxConns=1: LISTEN нельзя делить с обычными запросами store.
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("listen watcher parse: %w", err)
	}
	cfg.MaxConns = 1
	cfg.MinConns = 0

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("listen watcher pool: %w", err)
	}

	go w.loop(ctx, pool)
	return w, nil
}

// Events реализует watch.ChangeWatcher.
func (w *ListenWatcher) Events() <-chan struct{} { return w.ch }

// Close останавливает LISTEN-горутину.
func (w *ListenWatcher) Close() error {
	w.cancel()
	return nil
}

func (w *ListenWatcher) loop(ctx context.Context, pool *pgxpool.Pool) {
	defer close(w.ch)
	defer pool.Close()

	for {
		if ctx.Err() != nil {
			return
		}
		if err := w.listenOnce(ctx, pool); err != nil {
			if ctx.Err() != nil {
				return
			}
			w.log.Warn("postgres LISTEN/NOTIFY: reconnect", "err", err)
			// Poll reconcile подстрахует; короткая пауза перед повтором LISTEN.
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}
	}
}

func (w *ListenWatcher) listenOnce(ctx context.Context, pool *pgxpool.Pool) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "LISTEN "+channelBotChanges); err != nil {
		return err
	}
	if _, err := conn.Exec(ctx, "LISTEN "+channelRuntimeChanges); err != nil {
		return err
	}
	w.log.Info("postgres LISTEN/NOTIFY active",
		"channels", []string{channelBotChanges, channelRuntimeChanges})

	for {
		if _, err := conn.Conn().WaitForNotification(ctx); err != nil {
			return err
		}
		select {
		case w.ch <- struct{}{}:
		default:
		}
	}
}

// Compile-time: ListenWatcher реализует ChangeWatcher.
var _ watch.ChangeWatcher = (*ListenWatcher)(nil)
