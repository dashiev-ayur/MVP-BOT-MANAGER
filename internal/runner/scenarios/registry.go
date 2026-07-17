// Package scenarios — каталог вшитых сценариев bot-runner (ТЗ §6.3 / Phase 3.1).
//
// Новый bot_type добавляется явной Register, без раздувания монолитного switch
// во всём runner. default и default_extended регистрируются в Builtin().
package scenarios

import (
	"context"
	"fmt"
	"sync"

	"mvp-manager/internal/messenger"
	"mvp-manager/internal/store"
)

// Handler — узкий контракт сценария: обработать входящее сообщение канала.
//
// Реализации не знают про HTTP/supervisor — только мессенджер и текст ответа.
type Handler interface {
	// Handle обрабатывает Incoming; handled=true, если сценарий «взял» событие.
	Handle(ctx context.Context, ch messenger.Channel, in messenger.Incoming) (handled bool, err error)
}

// Registry — потокобезопасная карта bot_type → Handler.
type Registry struct {
	mu   sync.RWMutex
	byType map[store.BotType]Handler
}

// New создаёт пустой реестр (типы регистрируют вызывающие).
func New() *Registry {
	return &Registry{byType: make(map[store.BotType]Handler)}
}

// Register явно привязывает тип бота к обработчику сценария.
// Повторная регистрация того же типа перезаписывает handler (удобно в тестах).
func (r *Registry) Register(botType store.BotType, h Handler) {
	if r == nil || h == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byType[botType] = h
}

// Get возвращает handler для типа или false, если тип не зарегистрирован.
func (r *Registry) Get(botType store.BotType) (Handler, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.byType[botType]
	return h, ok
}

// MustGet как Get, но с ошибкой для старта инстанса.
func (r *Registry) MustGet(botType store.BotType) (Handler, error) {
	h, ok := r.Get(botType)
	if !ok {
		return nil, fmt.Errorf("сценарий для bot_type %q не зарегистрирован", botType)
	}
	return h, nil
}

// Builtin — канонический набор MVP: default + default_extended.
func Builtin() *Registry {
	r := New()
	r.Register(store.BotTypeDefault, Default{})
	r.Register(store.BotTypeDefaultExtended, Extended{})
	return r
}
