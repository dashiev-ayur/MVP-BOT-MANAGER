package reconcile

import (
	"sync"
	"time"
)

// RestartPolicy — параметры авто-рестарта failed/crashed процессов (Phase 4).
//
// Экспоненциальный backoff: base * 2^(attempt-1), capped MaxBackoff.
// После успешного ActualRunning счётчик попыток сбрасывается.
type RestartPolicy struct {
	// MaxAttempts — сколько рестартов после краша; 0 = выключено.
	MaxAttempts int
	// Base — пауза перед первой попыткой рестарта.
	Base time.Duration
	// Max — потолок backoff.
	Max time.Duration
}

// DefaultRestartPolicy — разумные дефолты, если Loop не сконфигурирован.
func DefaultRestartPolicy() RestartPolicy {
	return RestartPolicy{
		MaxAttempts: 5,
		Base:        time.Second,
		Max:         60 * time.Second,
	}
}

// restartTracker хранит per-runtime состояние backoff (только в памяти агента).
type restartTracker struct {
	mu    sync.Mutex
	items map[string]*restartState
}

type restartState struct {
	attempts    int
	nextAt      time.Time
	lastFailAt  time.Time
}

func newRestartTracker() *restartTracker {
	return &restartTracker{items: make(map[string]*restartState)}
}

// backoffFor возвращает длительность паузы для attempt (1-based).
func (p RestartPolicy) backoffFor(attempt int) time.Duration {
	if p.Base <= 0 {
		p.Base = time.Second
	}
	if p.Max <= 0 {
		p.Max = 60 * time.Second
	}
	if attempt < 1 {
		attempt = 1
	}
	d := p.Base
	for i := 1; i < attempt; i++ {
		if d >= p.Max/2 {
			return p.Max
		}
		d *= 2
	}
	if d > p.Max {
		return p.Max
	}
	return d
}

// recordFailure фиксирует краш/unhealthy и решает, можно ли рестартовать сейчас.
//
// Возвращает:
//   - allow=true → пора стартовать снова (попытка уже зарезервирована);
//   - allow=false, wait>0 → ещё рано (подождать wait);
//   - allow=false, wait=0 → лимит попыток исчерпан.
func (t *restartTracker) recordFailure(id string, p RestartPolicy, now time.Time) (allow bool, wait time.Duration, attempt int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if p.MaxAttempts <= 0 {
		return false, 0, 0
	}

	st := t.items[id]
	if st == nil {
		st = &restartState{}
		t.items[id] = st
	}

	// Уже запланирован следующий слот и время не пришло — ждём.
	if !st.nextAt.IsZero() && now.Before(st.nextAt) {
		return false, st.nextAt.Sub(now), st.attempts
	}

	// nextAt наступил — разрешаем запланированный рестарт.
	if !st.nextAt.IsZero() {
		attempt = st.attempts
		st.nextAt = time.Time{}
		return true, 0, attempt
	}

	// Новый failure (nextAt пуст): если лимит уже выбран — больше не планируем.
	if st.attempts >= p.MaxAttempts {
		return false, 0, st.attempts
	}

	st.attempts++
	st.lastFailAt = now
	wait = p.backoffFor(st.attempts)
	st.nextAt = now.Add(wait)
	return false, wait, st.attempts
}

// resetAfterSuccess сбрасывает backoff после успешного running.
func (t *restartTracker) resetAfterSuccess(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.items, id)
}

// clear забывает состояние (например, desired=stopped).
func (t *restartTracker) clear(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.items, id)
}

// exhausted — лимит попыток уже достигнут (для last_error).
func (t *restartTracker) exhausted(id string, p RestartPolicy) bool {
	if p.MaxAttempts <= 0 {
		return true // «как exhausted» — рестартов не будет
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	st := t.items[id]
	if st == nil {
		return false
	}
	return st.attempts >= p.MaxAttempts && st.nextAt.IsZero()
}
