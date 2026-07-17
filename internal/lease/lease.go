// Package lease — захват / продление / освобождение lease на runtime (ТЗ §10.3).
//
// Старт OS-процесса (custom и bot_runner) разрешён только после успешного Acquire
// текущим NODE_ID. Два агента с общим store не могут одновременно держать
// валидный lease одного runtime.
package lease

import (
	"context"
	"errors"
	"fmt"
	"time"

	"mvp-manager/internal/store"
)

// ErrHeld — удобный алиас store.ErrLeaseHeld для потребителей reconcile.
var ErrHeld = store.ErrLeaseHeld

// Manager — операции lease от имени одной ноды.
type Manager struct {
	NodeID   string
	TTL      time.Duration
	Runtimes store.RuntimeRepository

	// Now — для тестов; nil → time.Now().UTC().
	Now func() time.Time
}

// New создаёт Manager с TTL (если ≤0 — 15s).
func New(nodeID string, ttl time.Duration, runtimes store.RuntimeRepository) *Manager {
	if ttl <= 0 {
		ttl = 15 * time.Second
	}
	return &Manager{NodeID: nodeID, TTL: ttl, Runtimes: runtimes}
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now().UTC()
	}
	return time.Now().UTC()
}

func (m *Manager) until() time.Time {
	return m.now().Add(m.TTL)
}

// Acquire пытается захватить lease runtime для NodeID.
func (m *Manager) Acquire(ctx context.Context, runtimeID string) error {
	if err := m.Runtimes.TryAcquireLease(ctx, runtimeID, m.NodeID, m.until()); err != nil {
		return fmt.Errorf("acquire lease runtime %s node %s: %w", runtimeID, m.NodeID, err)
	}
	return nil
}

// Renew продлевает lease; чужой/отсутствующий → ErrHeld.
func (m *Manager) Renew(ctx context.Context, runtimeID string) error {
	if err := m.Runtimes.RenewLease(ctx, runtimeID, m.NodeID, m.until()); err != nil {
		return fmt.Errorf("renew lease runtime %s node %s: %w", runtimeID, m.NodeID, err)
	}
	return nil
}

// Release освобождает lease, если он наш.
func (m *Manager) Release(ctx context.Context, runtimeID string) error {
	if err := m.Runtimes.ReleaseLease(ctx, runtimeID, m.NodeID); err != nil {
		return fmt.Errorf("release lease runtime %s node %s: %w", runtimeID, m.NodeID, err)
	}
	return nil
}

// Holds сообщает, принадлежит ли валидный lease текущей ноде.
func (m *Manager) Holds(ctx context.Context, runtimeID string) (bool, error) {
	rt, err := m.Runtimes.ByID(ctx, runtimeID)
	if err != nil {
		return false, err
	}
	if rt.LeaseOwner == nil || *rt.LeaseOwner != m.NodeID {
		return false, nil
	}
	if rt.LeaseUntil == nil || !rt.LeaseUntil.After(m.now()) {
		return false, nil
	}
	return true, nil
}

// IsHeld возвращает true, если ошибка — чужой lease.
func IsHeld(err error) bool {
	return errors.Is(err, ErrHeld)
}
