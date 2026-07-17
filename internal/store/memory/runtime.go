package memory

import (
	"context"
	"fmt"
	"time"

	"mvp-manager/internal/store"
)

// Create сохраняет новый runtime. Пустой ID → UUID; конфликт name → ErrConflict.
func (r *RuntimeRepo) Create(ctx context.Context, rt store.Runtime) (store.Runtime, error) {
	if rt.Name == "" {
		return store.Runtime{}, fmt.Errorf("runtime name required: %w", store.ErrInvalidArgument)
	}

	if rt.ID == "" {
		id, err := newID()
		if err != nil {
			return store.Runtime{}, err
		}
		rt.ID = id
	}

	var out store.Runtime
	err := r.s.doWrite(ctx, func() error {
		if _, exists := r.s.runtimes[rt.ID]; exists {
			return fmt.Errorf("runtime id %q: %w", rt.ID, store.ErrConflict)
		}
		if owner, taken := r.s.byRuntimeName[rt.Name]; taken {
			return fmt.Errorf("runtime name %q already used by %q: %w", rt.Name, owner, store.ErrConflict)
		}

		ts := now()
		if rt.CreatedAt.IsZero() {
			rt.CreatedAt = ts
		}
		rt.UpdatedAt = ts
		if rt.DesiredState == "" {
			rt.DesiredState = store.DesiredStopped
		}
		if rt.ActualState == "" {
			rt.ActualState = store.ActualUnknown
		}
		if rt.ConfigVersion == 0 {
			rt.ConfigVersion = 1
		}
		if rt.Env == nil {
			rt.Env = map[string]any{}
		} else {
			rt.Env = cloneStringMap(rt.Env)
		}

		stored := cloneRuntime(rt)
		r.s.runtimes[stored.ID] = stored
		r.s.byRuntimeName[stored.Name] = stored.ID
		out = cloneRuntime(stored)
		return nil
	})
	return out, err
}

// ByID возвращает runtime или ErrNotFound.
func (r *RuntimeRepo) ByID(ctx context.Context, id string) (store.Runtime, error) {
	var out store.Runtime
	err := r.s.doRead(ctx, func() error {
		rt, ok := r.s.runtimes[id]
		if !ok {
			return store.ErrNotFound
		}
		out = cloneRuntime(rt)
		return nil
	})
	return out, err
}

// ByName возвращает runtime по уникальному имени или ErrNotFound.
func (r *RuntimeRepo) ByName(ctx context.Context, name string) (store.Runtime, error) {
	var out store.Runtime
	err := r.s.doRead(ctx, func() error {
		id, ok := r.s.byRuntimeName[name]
		if !ok {
			return store.ErrNotFound
		}
		out = cloneRuntime(r.s.runtimes[id])
		return nil
	})
	return out, err
}

// List возвращает все runtimes.
func (r *RuntimeRepo) List(ctx context.Context) ([]store.Runtime, error) {
	var out []store.Runtime
	err := r.s.doRead(ctx, func() error {
		out = make([]store.Runtime, 0, len(r.s.runtimes))
		for _, rt := range r.s.runtimes {
			out = append(out, cloneRuntime(rt))
		}
		return nil
	})
	return out, err
}

// ListByNode — runtimes с assigned_node_id = nodeID.
func (r *RuntimeRepo) ListByNode(ctx context.Context, nodeID string) ([]store.Runtime, error) {
	var out []store.Runtime
	err := r.s.doRead(ctx, func() error {
		out = make([]store.Runtime, 0)
		for _, rt := range r.s.runtimes {
			if rt.AssignedNodeID != nil && *rt.AssignedNodeID == nodeID {
				out = append(out, cloneRuntime(rt))
			}
		}
		return nil
	})
	return out, err
}

// Update полностью заменяет изменяемые поля runtime (кроме CreatedAt).
// Нет записи → ErrNotFound; конфликт name → ErrConflict.
func (r *RuntimeRepo) Update(ctx context.Context, rt store.Runtime) (store.Runtime, error) {
	if rt.ID == "" {
		return store.Runtime{}, fmt.Errorf("runtime update: empty id: %w", store.ErrInvalidArgument)
	}
	if rt.Name == "" {
		return store.Runtime{}, fmt.Errorf("runtime name required: %w", store.ErrInvalidArgument)
	}

	var out store.Runtime
	err := r.s.doWrite(ctx, func() error {
		existing, ok := r.s.runtimes[rt.ID]
		if !ok {
			return store.ErrNotFound
		}

		// Конфликт имени: другое id уже занимает это name.
		if owner, taken := r.s.byRuntimeName[rt.Name]; taken && owner != rt.ID {
			return fmt.Errorf("runtime name %q already used by %q: %w", rt.Name, owner, store.ErrConflict)
		}

		// Обновляем индекс имени при переименовании.
		if existing.Name != rt.Name {
			delete(r.s.byRuntimeName, existing.Name)
			r.s.byRuntimeName[rt.Name] = rt.ID
		}

		rt.CreatedAt = existing.CreatedAt
		rt.UpdatedAt = now()
		if rt.Env == nil {
			rt.Env = map[string]any{}
		} else {
			rt.Env = cloneStringMap(rt.Env)
		}

		stored := cloneRuntime(rt)
		r.s.runtimes[stored.ID] = stored
		out = cloneRuntime(stored)
		return nil
	})
	return out, err
}

// UpdateDesiredState меняет только desired_state (типичный путь ctl start/stop).
func (r *RuntimeRepo) UpdateDesiredState(ctx context.Context, id string, desired store.DesiredState) error {
	return r.s.doWrite(ctx, func() error {
		rt, ok := r.s.runtimes[id]
		if !ok {
			return store.ErrNotFound
		}
		rt.DesiredState = desired
		rt.UpdatedAt = now()
		r.s.runtimes[id] = rt
		return nil
	})
}

// UpdateActual меняет фактическое состояние после действий supervisor.
func (r *RuntimeRepo) UpdateActual(ctx context.Context, id string, patch store.RuntimeActualPatch) error {
	return r.s.doWrite(ctx, func() error {
		rt, ok := r.s.runtimes[id]
		if !ok {
			return store.ErrNotFound
		}
		rt.ActualState = patch.ActualState
		// Указатели пишутся «как есть»: nil = NULL в store (очистка pid/exit/error).
		if patch.PID != nil {
			v := *patch.PID
			rt.PID = &v
		} else {
			rt.PID = nil
		}
		if patch.ExitCode != nil {
			v := *patch.ExitCode
			rt.ExitCode = &v
		} else {
			rt.ExitCode = nil
		}
		if patch.LastError != nil {
			v := *patch.LastError
			rt.LastError = &v
		} else {
			rt.LastError = nil
		}
		rt.UpdatedAt = now()
		r.s.runtimes[id] = rt
		return nil
	})
}

// UpdateLease обновляет lease_owner / lease_until.
// owner/until = nil означает сброс соответствующего поля в NULL.
func (r *RuntimeRepo) UpdateLease(ctx context.Context, id string, owner *string, until *time.Time) error {
	return r.s.doWrite(ctx, func() error {
		rt, ok := r.s.runtimes[id]
		if !ok {
			return store.ErrNotFound
		}
		if owner != nil {
			v := *owner
			rt.LeaseOwner = &v
		} else {
			rt.LeaseOwner = nil
		}
		if until != nil {
			v := *until
			rt.LeaseUntil = &v
		} else {
			rt.LeaseUntil = nil
		}
		rt.UpdatedAt = now()
		r.s.runtimes[id] = rt
		return nil
	})
}

// TryAcquireLease — атомарный захват (под flock в file-backed режиме).
func (r *RuntimeRepo) TryAcquireLease(ctx context.Context, id string, owner string, until time.Time) error {
	return r.s.doWrite(ctx, func() error {
		rt, ok := r.s.runtimes[id]
		if !ok {
			return store.ErrNotFound
		}
		if !leaseAcquirable(rt, owner, now()) {
			return store.ErrLeaseHeld
		}
		o := owner
		u := until.UTC()
		rt.LeaseOwner = &o
		rt.LeaseUntil = &u
		rt.UpdatedAt = now()
		r.s.runtimes[id] = rt
		return nil
	})
}

// RenewLease продлевает until только для текущего владельца.
func (r *RuntimeRepo) RenewLease(ctx context.Context, id string, owner string, until time.Time) error {
	return r.s.doWrite(ctx, func() error {
		rt, ok := r.s.runtimes[id]
		if !ok {
			return store.ErrNotFound
		}
		if !leaseOwnedBy(rt, owner, now()) {
			return store.ErrLeaseHeld
		}
		u := until.UTC()
		o := owner
		rt.LeaseOwner = &o
		rt.LeaseUntil = &u
		rt.UpdatedAt = now()
		r.s.runtimes[id] = rt
		return nil
	})
}

// ReleaseLease сбрасывает lease, если он наш (или уже свободен/истёк).
func (r *RuntimeRepo) ReleaseLease(ctx context.Context, id string, owner string) error {
	return r.s.doWrite(ctx, func() error {
		rt, ok := r.s.runtimes[id]
		if !ok {
			return store.ErrNotFound
		}
		// Чужой валидный lease нельзя сбросить.
		if leaseHeldByOther(rt, owner, now()) {
			return store.ErrLeaseHeld
		}
		rt.LeaseOwner = nil
		rt.LeaseUntil = nil
		rt.UpdatedAt = now()
		r.s.runtimes[id] = rt
		return nil
	})
}

// leaseAcquirable — свободен, истёк или уже наш.
func leaseAcquirable(rt store.Runtime, owner string, now time.Time) bool {
	if rt.LeaseOwner == nil || *rt.LeaseOwner == "" {
		return true
	}
	if rt.LeaseUntil == nil || !rt.LeaseUntil.After(now) {
		return true // истёк
	}
	return *rt.LeaseOwner == owner
}

// leaseOwnedBy — валидный lease принадлежит owner (для renew).
func leaseOwnedBy(rt store.Runtime, owner string, now time.Time) bool {
	if rt.LeaseOwner == nil || *rt.LeaseOwner != owner {
		return false
	}
	// Истёкший «наш» lease можно продлить (как re-acquire).
	if rt.LeaseUntil == nil || !rt.LeaseUntil.After(now) {
		return true
	}
	return true
}

// leaseHeldByOther — валидный lease у другого узла.
func leaseHeldByOther(rt store.Runtime, owner string, now time.Time) bool {
	if rt.LeaseOwner == nil || *rt.LeaseOwner == "" {
		return false
	}
	if rt.LeaseUntil == nil || !rt.LeaseUntil.After(now) {
		return false
	}
	return *rt.LeaseOwner != owner
}
