// Package metrics — простые счётчики процесса (без обязательного Prometheus).
//
// Значения пишутся в slog с attr metric=<name> (удобно грепать в логах agent)
// и доступны через Snapshot / Text для опционального GET /metrics.
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
	log  *slog.Logger
}

// Default — общий registry процесса (agent / control-api).
var Default = New(nil)

// New создаёт пустой registry. log=nil → slog.Default при Inc.
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

// Inc увеличивает счётчик на 1 и пишет slog с metric=<name>.
func (r *Registry) Inc(name string, attrs ...any) {
	n := r.counter(name).Add(1)
	log := r.log
	if log == nil {
		log = slog.Default()
	}
	args := []any{"metric", name, "value", n}
	args = append(args, attrs...)
	log.Info("metric", args...)
}

// Add увеличивает счётчик на delta (>=1 имеет смысл для batch).
func (r *Registry) Add(name string, delta int64, attrs ...any) {
	if delta == 0 {
		return
	}
	n := r.counter(name).Add(delta)
	log := r.log
	if log == nil {
		log = slog.Default()
	}
	args := []any{"metric", name, "value", n, "delta", delta}
	args = append(args, attrs...)
	log.Info("metric", args...)
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
