package memory

import (
	"context"
	"fmt"
	"time"

	"mvp-manager/internal/store"
)

// Create сохраняет новый runtime. Пустой ID → UUID; конфликт name → ErrConflict.
func (r *RuntimeRepo) Create(ctx context.Context, rt store.Runtime) (store.Runtime, error) {
	if err := checkCtx(ctx); err != nil {
		return store.Runtime{}, err
	}
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

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	if _, exists := r.s.runtimes[rt.ID]; exists {
		return store.Runtime{}, fmt.Errorf("runtime id %q: %w", rt.ID, store.ErrConflict)
	}
	if owner, taken := r.s.byRuntimeName[rt.Name]; taken {
		return store.Runtime{}, fmt.Errorf("runtime name %q already used by %q: %w", rt.Name, owner, store.ErrConflict)
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
	return cloneRuntime(stored), nil
}

// ByID возвращает runtime или ErrNotFound.
func (r *RuntimeRepo) ByID(ctx context.Context, id string) (store.Runtime, error) {
	if err := checkCtx(ctx); err != nil {
		return store.Runtime{}, err
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	rt, ok := r.s.runtimes[id]
	if !ok {
		return store.Runtime{}, store.ErrNotFound
	}
	return cloneRuntime(rt), nil
}

// ByName возвращает runtime по уникальному имени или ErrNotFound.
func (r *RuntimeRepo) ByName(ctx context.Context, name string) (store.Runtime, error) {
	if err := checkCtx(ctx); err != nil {
		return store.Runtime{}, err
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	id, ok := r.s.byRuntimeName[name]
	if !ok {
		return store.Runtime{}, store.ErrNotFound
	}
	return cloneRuntime(r.s.runtimes[id]), nil
}

// List возвращает все runtimes.
func (r *RuntimeRepo) List(ctx context.Context) ([]store.Runtime, error) {
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	out := make([]store.Runtime, 0, len(r.s.runtimes))
	for _, rt := range r.s.runtimes {
		out = append(out, cloneRuntime(rt))
	}
	return out, nil
}

// ListByNode — runtimes с assigned_node_id = nodeID.
func (r *RuntimeRepo) ListByNode(ctx context.Context, nodeID string) ([]store.Runtime, error) {
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	out := make([]store.Runtime, 0)
	for _, rt := range r.s.runtimes {
		if rt.AssignedNodeID != nil && *rt.AssignedNodeID == nodeID {
			out = append(out, cloneRuntime(rt))
		}
	}
	return out, nil
}

// Update полностью заменяет изменяемые поля runtime (кроме CreatedAt).
// Нет записи → ErrNotFound; конфликт name → ErrConflict.
func (r *RuntimeRepo) Update(ctx context.Context, rt store.Runtime) (store.Runtime, error) {
	if err := checkCtx(ctx); err != nil {
		return store.Runtime{}, err
	}
	if rt.ID == "" {
		return store.Runtime{}, fmt.Errorf("runtime update: empty id: %w", store.ErrInvalidArgument)
	}
	if rt.Name == "" {
		return store.Runtime{}, fmt.Errorf("runtime name required: %w", store.ErrInvalidArgument)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	existing, ok := r.s.runtimes[rt.ID]
	if !ok {
		return store.Runtime{}, store.ErrNotFound
	}

	// Конфликт имени: другое id уже занимает это name.
	if owner, taken := r.s.byRuntimeName[rt.Name]; taken && owner != rt.ID {
		return store.Runtime{}, fmt.Errorf("runtime name %q already used by %q: %w", rt.Name, owner, store.ErrConflict)
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
	return cloneRuntime(stored), nil
}

// UpdateDesiredState меняет только desired_state (типичный путь ctl start/stop).
func (r *RuntimeRepo) UpdateDesiredState(ctx context.Context, id string, desired store.DesiredState) error {
	if err := checkCtx(ctx); err != nil {
		return err
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	rt, ok := r.s.runtimes[id]
	if !ok {
		return store.ErrNotFound
	}
	rt.DesiredState = desired
	rt.UpdatedAt = now()
	r.s.runtimes[id] = rt
	return nil
}

// UpdateActual меняет фактическое состояние после действий supervisor.
func (r *RuntimeRepo) UpdateActual(ctx context.Context, id string, patch store.RuntimeActualPatch) error {
	if err := checkCtx(ctx); err != nil {
		return err
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

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
}

// UpdateLease обновляет lease_owner / lease_until.
// owner/until = nil означает сброс соответствующего поля в NULL.
func (r *RuntimeRepo) UpdateLease(ctx context.Context, id string, owner *string, until *time.Time) error {
	if err := checkCtx(ctx); err != nil {
		return err
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

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
}
