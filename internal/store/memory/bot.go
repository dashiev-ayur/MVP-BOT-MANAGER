package memory

import (
	"context"
	"fmt"
	"strings"

	"mvp-manager/internal/store"
)

// validateCustomName проверяет инвариант ТЗ §6.3 (bots_custom_name_required):
//   - bot_type=custom → custom_name обязателен и непустой;
//   - иначе → custom_name должен быть nil/пустым (в store пишем nil).
//
// Возвращает нормализованное значение для записи.
func validateCustomName(botType store.BotType, customName *string) (*string, error) {
	trimmed := ""
	if customName != nil {
		trimmed = strings.TrimSpace(*customName)
	}

	if botType == store.BotTypeCustom {
		if trimmed == "" {
			return nil, fmt.Errorf("custom_name required for bot_type=custom: %w", store.ErrInvalidArgument)
		}
		v := trimmed
		return &v, nil
	}

	if trimmed != "" {
		return nil, fmt.Errorf("custom_name must be empty for bot_type=%s: %w", botType, store.ErrInvalidArgument)
	}
	return nil, nil
}

// Create сохраняет нового бота.
// Пустой ID → UUID; UNIQUE port → ErrConflict; custom_name ↔ bot_type → ErrInvalidArgument.
func (r *BotRepo) Create(ctx context.Context, b store.Bot) (store.Bot, error) {
	if err := checkCtx(ctx); err != nil {
		return store.Bot{}, err
	}

	normalized, err := validateCustomName(b.BotType, b.CustomName)
	if err != nil {
		return store.Bot{}, err
	}
	b.CustomName = normalized

	if b.ID == "" {
		id, err := newID()
		if err != nil {
			return store.Bot{}, err
		}
		b.ID = id
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	if _, exists := r.s.bots[b.ID]; exists {
		return store.Bot{}, fmt.Errorf("bot id %q: %w", b.ID, store.ErrConflict)
	}
	if owner, taken := r.s.byPort[b.Port]; taken {
		return store.Bot{}, fmt.Errorf("port %d already used by bot %q: %w", b.Port, owner, store.ErrConflict)
	}

	ts := now()
	if b.CreatedAt.IsZero() {
		b.CreatedAt = ts
	}
	b.UpdatedAt = ts
	if b.DesiredState == "" {
		b.DesiredState = store.DesiredStopped
	}
	if b.ActualState == "" {
		b.ActualState = store.ActualUnknown
	}
	if b.ConfigVersion == 0 {
		b.ConfigVersion = 1
	}
	if b.ScenarioConfig == nil {
		b.ScenarioConfig = map[string]any{}
	} else {
		b.ScenarioConfig = cloneStringMap(b.ScenarioConfig)
	}

	stored := cloneBot(b)
	r.s.bots[stored.ID] = stored
	r.s.byPort[stored.Port] = stored.ID
	return cloneBot(stored), nil
}

// ByID возвращает бота или ErrNotFound.
func (r *BotRepo) ByID(ctx context.Context, id string) (store.Bot, error) {
	if err := checkCtx(ctx); err != nil {
		return store.Bot{}, err
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	b, ok := r.s.bots[id]
	if !ok {
		return store.Bot{}, store.ErrNotFound
	}
	return cloneBot(b), nil
}

// List возвращает всех ботов.
func (r *BotRepo) List(ctx context.Context) ([]store.Bot, error) {
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	out := make([]store.Bot, 0, len(r.s.bots))
	for _, b := range r.s.bots {
		out = append(out, cloneBot(b))
	}
	return out, nil
}

// ListByNode — боты с assigned_node_id = nodeID.
func (r *BotRepo) ListByNode(ctx context.Context, nodeID string) ([]store.Bot, error) {
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	out := make([]store.Bot, 0)
	for _, b := range r.s.bots {
		if b.AssignedNodeID != nil && *b.AssignedNodeID == nodeID {
			out = append(out, cloneBot(b))
		}
	}
	return out, nil
}

// ListByRuntime — боты, привязанные к runtime_id.
func (r *BotRepo) ListByRuntime(ctx context.Context, runtimeID string) ([]store.Bot, error) {
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	out := make([]store.Bot, 0)
	for _, b := range r.s.bots {
		if b.RuntimeID != nil && *b.RuntimeID == runtimeID {
			out = append(out, cloneBot(b))
		}
	}
	return out, nil
}

// Update полностью заменяет изменяемые поля бота (кроме CreatedAt).
func (r *BotRepo) Update(ctx context.Context, b store.Bot) (store.Bot, error) {
	if err := checkCtx(ctx); err != nil {
		return store.Bot{}, err
	}
	if b.ID == "" {
		return store.Bot{}, fmt.Errorf("bot update: empty id: %w", store.ErrInvalidArgument)
	}

	normalized, err := validateCustomName(b.BotType, b.CustomName)
	if err != nil {
		return store.Bot{}, err
	}
	b.CustomName = normalized

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	existing, ok := r.s.bots[b.ID]
	if !ok {
		return store.Bot{}, store.ErrNotFound
	}

	// Конфликт порта: другое id уже занимает этот port.
	if owner, taken := r.s.byPort[b.Port]; taken && owner != b.ID {
		return store.Bot{}, fmt.Errorf("port %d already used by bot %q: %w", b.Port, owner, store.ErrConflict)
	}

	if existing.Port != b.Port {
		delete(r.s.byPort, existing.Port)
		r.s.byPort[b.Port] = b.ID
	}

	b.CreatedAt = existing.CreatedAt
	b.UpdatedAt = now()
	if b.ScenarioConfig == nil {
		b.ScenarioConfig = map[string]any{}
	} else {
		b.ScenarioConfig = cloneStringMap(b.ScenarioConfig)
	}

	stored := cloneBot(b)
	r.s.bots[stored.ID] = stored
	return cloneBot(stored), nil
}

// UpdateDesiredState меняет только desired_state (ctl start/stop).
func (r *BotRepo) UpdateDesiredState(ctx context.Context, id string, desired store.DesiredState) error {
	if err := checkCtx(ctx); err != nil {
		return err
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	b, ok := r.s.bots[id]
	if !ok {
		return store.ErrNotFound
	}
	b.DesiredState = desired
	b.UpdatedAt = now()
	r.s.bots[id] = b
	return nil
}

// UpdateActual меняет фактическое состояние бота (runner/agent после reconcile).
func (r *BotRepo) UpdateActual(ctx context.Context, id string, patch store.BotActualPatch) error {
	if err := checkCtx(ctx); err != nil {
		return err
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	b, ok := r.s.bots[id]
	if !ok {
		return store.ErrNotFound
	}
	b.ActualState = patch.ActualState
	if patch.LastError != nil {
		v := *patch.LastError
		b.LastError = &v
	} else {
		b.LastError = nil
	}
	b.UpdatedAt = now()
	r.s.bots[id] = b
	return nil
}

// Delete удаляет бота по id. Нет записи → ErrNotFound.
func (r *BotRepo) Delete(ctx context.Context, id string) error {
	if err := checkCtx(ctx); err != nil {
		return err
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	b, ok := r.s.bots[id]
	if !ok {
		return store.ErrNotFound
	}
	delete(r.s.byPort, b.Port)
	delete(r.s.bots, id)
	return nil
}
