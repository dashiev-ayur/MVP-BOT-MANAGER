// Package watch — уведомления об изменениях store для ускорения reconcile.
//
// Периодический poll в agent остаётся safety net; watcher только «будит»
// цикл раньше полного RECONCILE_INTERVAL.
//
// Реализации:
//   - FileWatcher — mtime файла MEMORY_STORE_PATH (STORE=memory);
//   - NopWatcher — заглушка (нет пути / тесты);
//   - Phase PG: отдельный пакет сможет добавить LISTEN/NOTIFY без правок
//     reconcile — достаточно реализовать ChangeWatcher и передать в Loop.
//
// Пакет намеренно не импортирует pgx / database/sql.
package watch

import (
	"context"
	"log/slog"
	"os"
	"time"
)

// ChangeWatcher сигнализирует о возможном изменении желаемого состояния в store.
// Канал Events закрывается при остановке Watcher (отмена ctx или Close).
type ChangeWatcher interface {
	// Events — сигналы «перечитай store» (значения игнорируются; важен сам факт).
	Events() <-chan struct{}
	// Close останавливает фоновое наблюдение.
	Close() error
}

// NopWatcher — пустая реализация: Events никогда не срабатывает.
// Используется, когда путь к файлу пуст или STORE ещё без watcher’а.
type NopWatcher struct {
	ch chan struct{}
}

// NewNop возвращает watcher без сигналов.
func NewNop() *NopWatcher {
	return &NopWatcher{ch: make(chan struct{})}
}

// Events реализует ChangeWatcher.
func (n *NopWatcher) Events() <-chan struct{} { return n.ch }

// Close закрывает канал событий.
func (n *NopWatcher) Close() error {
	select {
	case <-n.ch:
		// уже закрыт
	default:
		close(n.ch)
	}
	return nil
}

// FileWatcher следит за mtime JSON-файла memory store.
// Эквивалент лёгкого fsnotify без внешней зависимости: poll stat с коротким шагом.
//
// LISTEN/NOTIFY для STORE=postgres подключается отдельно (Phase PG) через
// ту же абстракцию ChangeWatcher — agent wiring выбирает реализацию по STORE.
type FileWatcher struct {
	path     string
	interval time.Duration
	ch       chan struct{}
	cancel   context.CancelFunc
	log      *slog.Logger
}

// NewFileWatcher запускает фоновый poll mtime path.
// interval — как часто проверять файл (типично 200–500ms); <=0 → 300ms.
func NewFileWatcher(path string, interval time.Duration, log *slog.Logger) *FileWatcher {
	if interval <= 0 {
		interval = 300 * time.Millisecond
	}
	if log == nil {
		log = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	w := &FileWatcher{
		path:     path,
		interval: interval,
		ch:       make(chan struct{}, 1),
		cancel:   cancel,
		log:      log,
	}
	go w.loop(ctx)
	return w
}

// Events реализует ChangeWatcher.
func (w *FileWatcher) Events() <-chan struct{} { return w.ch }

// Close останавливает poll.
func (w *FileWatcher) Close() error {
	w.cancel()
	return nil
}

func (w *FileWatcher) loop(ctx context.Context) {
	defer close(w.ch)

	var lastMod time.Time
	var lastSize int64
	have := false

	tick := time.NewTicker(w.interval)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			fi, err := os.Stat(w.path)
			if err != nil {
				// Файл ещё не создан или временно недоступен — ждём.
				continue
			}
			mod := fi.ModTime()
			size := fi.Size()
			if !have {
				lastMod, lastSize, have = mod, size, true
				continue
			}
			if mod.Equal(lastMod) && size == lastSize {
				continue
			}
			lastMod, lastSize = mod, size
			// Неблокирующая отправка: если reconcile ещё не успел — сигнал уже «есть».
			select {
			case w.ch <- struct{}{}:
			default:
			}
		}
	}
}

// OpenForStore выбирает watcher по типу store и пути.
//
//   - memory + непустой path → FileWatcher;
//   - postgres → Nop (Phase PG подключит LISTEN/NOTIFY поверх того же интерфейса);
//   - иначе → Nop.
func OpenForStore(storeKind, memoryPath string, log *slog.Logger) ChangeWatcher {
	switch storeKind {
	case "memory":
		if memoryPath == "" {
			return NewNop()
		}
		return NewFileWatcher(memoryPath, 300*time.Millisecond, log)
	case "postgres":
		// LISTEN/NOTIFY поднимает storeopen.OpenWatcher (pgx только там).
		// Здесь оставляем Nop на случай прямого вызова без storeopen.
		if log != nil {
			log.Info("store change watcher: для postgres используйте storeopen.OpenWatcher")
		}
		return NewNop()
	default:
		return NewNop()
	}
}
