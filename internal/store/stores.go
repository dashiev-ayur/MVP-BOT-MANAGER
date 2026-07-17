package store

import "io"

// Stores — общий фасад репозиториев для wiring в cmd/* (agent, ctl, …).
//
// И memory, и postgres отдают один и тот же набор полей-интерфейсов.
// Бизнес-пакеты (reconcile/supervisor/runner/ops) по-прежнему принимают
// узкие NodeRepository / BotRepository и т.п. — без знания бэкенда и без pgx.
//
// Closer: для postgres закрывает пул соединений; для memory обычно nil
// (или no-op). cmd/* вызывает Close() в defer при наличии.
type Stores struct {
	Nodes    NodeRepository
	Runtimes RuntimeRepository
	Bots     BotRepository
	Events   EventRepository

	// Closer освобождает ресурсы бэкенда (например *pgxpool.Pool).
	// Может быть nil — тогда Close() ничего не делает.
	Closer io.Closer
}

// Close вызывает Closer, если он задан.
func (s Stores) Close() error {
	if s.Closer == nil {
		return nil
	}
	return s.Closer.Close()
}
