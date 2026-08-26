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

	var out store.Bot
	err = r.s.doWrite(ctx, func() error {
		if _, exists := r.s.bots[b.ID]; exists {
			return fmt.Errorf("bot id %q: %w", b.ID, store.ErrConflict)
		}
		if owner, taken := r.s.byPort[b.Port]; taken {
			return fmt.Errorf("port %d already used by bot %q: %w", b.Port, owner, store.ErrConflict)
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
		out = cloneBot(stored)
		return nil
	})
	return out, err
}

// ByID возвращает бота или ErrNotFound.
func (r *BotRepo) ByID(ctx context.Context, id string) (store.Bot, error) {
	var out store.Bot
	err := r.s.doRead(ctx, func() error {
		b, ok := r.s.bots[id]
		if !ok {
			return store.ErrNotFound
		}
		out = cloneBot(b)
		return nil
	})
	return out, err
}

// List возвращает всех ботов.
func (r *BotRepo) List(ctx context.Context) ([]store.Bot, error) {
	var out []store.Bot
	err := r.s.doRead(ctx, func() error {
		out = make([]store.Bot, 0, len(r.s.bots))
		for _, b := range r.s.bots {
			out = append(out, cloneBot(b))
		}
		return nil
	})
	return out, err
}

// ListByNode — боты с assigned_node_id = nodeID.
func (r *BotRepo) ListByNode(ctx context.Context, nodeID string) ([]store.Bot, error) {
	var out []store.Bot
	err := r.s.doRead(ctx, func() error {
		out = make([]store.Bot, 0)
		for _, b := range r.s.bots {
			if b.AssignedNodeID != nil && *b.AssignedNodeID == nodeID {
				out = append(out, cloneBot(b))
			}
		}
		return nil
	})
	return out, err
}

// ListByRuntime — боты, привязанные к runtime_id.
func (r *BotRepo) ListByRuntime(ctx context.Context, runtimeID string) ([]store.Bot, error) {
	var out []store.Bot
	err := r.s.doRead(ctx, func() error {
		out = make([]store.Bot, 0)
		for _, b := range r.s.bots {
			if b.RuntimeID != nil && *b.RuntimeID == runtimeID {
				out = append(out, cloneBot(b))
			}
		}
		return nil
	})
	return out, err
}

// ListByClientID — боты с client_id = clientID (сравнение без учёта регистра UUID).
func (r *BotRepo) ListByClientID(ctx context.Context, clientID string) ([]store.Bot, error) {
	var out []store.Bot
	err := r.s.doRead(ctx, func() error {
		out = make([]store.Bot, 0)
		for _, b := range r.s.bots {
			if b.ClientID != nil && strings.EqualFold(*b.ClientID, clientID) {
				out = append(out, cloneBot(b))
			}
		}
		return nil
	})
	return out, err
}

// Update полностью заменяет изменяемые поля бота (кроме CreatedAt).
func (r *BotRepo) Update(ctx context.Context, b store.Bot) (store.Bot, error) {
	if b.ID == "" {
		return store.Bot{}, fmt.Errorf("bot update: empty id: %w", store.ErrInvalidArgument)
	}

	normalized, err := validateCustomName(b.BotType, b.CustomName)
	if err != nil {
		return store.Bot{}, err
	}
	b.CustomName = normalized

	var out store.Bot
	err = r.s.doWrite(ctx, func() error {
		existing, ok := r.s.bots[b.ID]
		if !ok {
			return store.ErrNotFound
		}

		// Конфликт порта: другое id уже занимает этот port.
		if owner, taken := r.s.byPort[b.Port]; taken && owner != b.ID {
			return fmt.Errorf("port %d already used by bot %q: %w", b.Port, owner, store.ErrConflict)
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
		out = cloneBot(stored)
		return nil
	})
	return out, err
}

// UpdateDesiredState меняет только desired_state (ctl start/stop).
func (r *BotRepo) UpdateDesiredState(ctx context.Context, id string, desired store.DesiredState) error {
	return r.s.doWrite(ctx, func() error {
		b, ok := r.s.bots[id]
		if !ok {
			return store.ErrNotFound
		}
		b.DesiredState = desired
		b.UpdatedAt = now()
		r.s.bots[id] = b
		return nil
	})
}

// UpdateActual меняет фактическое состояние бота (runner/agent после reconcile).
func (r *BotRepo) UpdateActual(ctx context.Context, id string, patch store.BotActualPatch) error {
	return r.s.doWrite(ctx, func() error {
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
	})
}

// Delete удаляет бота по id. Нет записи → ErrNotFound.
func (r *BotRepo) Delete(ctx context.Context, id string) error {
	return r.s.doWrite(ctx, func() error {
		b, ok := r.s.bots[id]
		if !ok {
			return store.ErrNotFound
		}
		delete(r.s.byPort, b.Port)
		delete(r.s.bots, id)
		return nil
	})
}
