// Package metrics — простые счётчики процесса (без обязательного Prometheus).
//
// Счётчики только в памяти: Snapshot / Text для GET /metrics.
// В slog не пишем на каждый Inc — иначе reconcile_ticks заливает консоль.
package metrics

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// Имена счётчиков Phase 4.
const (
	ReconcileTicks = "reconcile_ticks"
	Starts         = "process_starts"
	Stops          = "process_stops"
	Restarts       = "process_restarts"
	LeaseFails     = "lease_fails"
	LimitRejects   = "bot_limit_rejects"
)

// Registry — потокобезопасный набор счётчиков.
type Registry struct {
	mu   sync.Mutex
	vals map[string]*atomic.Int64
	log  *slog.Logger // зарезервирован; Inc/Add в лог не пишут
}

// Default — общий registry процесса (agent / control-api).
var Default = New(nil)

// New создаёт пустой registry. log сохраняется для совместимости API.
func New(log *slog.Logger) *Registry {
	return &Registry{
		vals: make(map[string]*atomic.Int64),
		log:  log,
	}
}

func (r *Registry) counter(name string) *atomic.Int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.vals[name]
	if !ok {
		c = &atomic.Int64{}
		r.vals[name] = c
	}
	return c
}

// Inc увеличивает счётчик на 1 (без записи в slog).
func (r *Registry) Inc(name string, attrs ...any) {
	_ = attrs
	r.counter(name).Add(1)
}

// Add увеличивает счётчик на delta (без записи в slog).
func (r *Registry) Add(name string, delta int64, attrs ...any) {
	_ = attrs
	if delta == 0 {
		return
	}
	r.counter(name).Add(delta)
}

// Get возвращает текущее значение (0 если счётчика ещё не было).
func (r *Registry) Get(name string) int64 {
	r.mu.Lock()
	c, ok := r.vals[name]
	r.mu.Unlock()
	if !ok {
		return 0
	}
	return c.Load()
}

// Snapshot копирует все счётчики.
func (r *Registry) Snapshot() map[string]int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]int64, len(r.vals))
	for k, c := range r.vals {
		out[k] = c.Load()
	}
	return out
}

// Text — простой text exposition (имя значение\n), удобно для /metrics.
func (r *Registry) Text() string {
	snap := r.Snapshot()
	keys := make([]string, 0, len(snap))
	for k := range snap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s %d\n", k, snap[k])
	}
	return b.String()
}
