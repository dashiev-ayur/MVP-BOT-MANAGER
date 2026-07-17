package memory

import (
	"context"
	"strings"

	"mvp-manager/internal/store"
)

// EventRepo реализует store.EventRepository над общим shared.
type EventRepo struct{ s *shared }

var _ store.EventRepository = (*EventRepo)(nil)

// Append добавляет событие аудита бота.
func (e *EventRepo) Append(ctx context.Context, ev store.BotEvent) (store.BotEvent, error) {
	var out store.BotEvent
	err := e.s.doWrite(ctx, func() error {
		if strings.TrimSpace(ev.BotID) == "" {
			return store.ErrInvalidArgument
		}
		if strings.TrimSpace(ev.Type) == "" {
			return store.ErrInvalidArgument
		}
		if ev.ID == "" {
			id, err := newID()
			if err != nil {
				return err
			}
			ev.ID = id
		}
		if ev.At.IsZero() {
			ev.At = now()
		} else {
			ev.At = ev.At.UTC()
		}
		stored := cloneEvent(ev)
		e.s.events = append(e.s.events, stored)
		out = cloneEvent(stored)
		return nil
	})
	return out, err
}

// ListByBot возвращает события бота в порядке добавления.
func (e *EventRepo) ListByBot(ctx context.Context, botID string) ([]store.BotEvent, error) {
	var out []store.BotEvent
	err := e.s.doRead(ctx, func() error {
		for _, ev := range e.s.events {
			if ev.BotID != botID {
				continue
			}
			out = append(out, cloneEvent(ev))
		}
		return nil
	})
	if out == nil {
		out = []store.BotEvent{}
	}
	return out, err
}

func cloneEvent(ev store.BotEvent) store.BotEvent {
	ev.Meta = cloneStringMap(ev.Meta)
	return ev
}
