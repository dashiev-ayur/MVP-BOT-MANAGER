package postgres

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"mvp-manager/internal/store"
)

// EventRepo реализует store.EventRepository.
//
// В SQL id — BIGSERIAL; в доменной модели ID — string (десятичная запись номера).
type EventRepo struct{ pool *pgxpool.Pool }

// Append добавляет событие аудита.
func (e *EventRepo) Append(ctx context.Context, ev store.BotEvent) (store.BotEvent, error) {
	if strings.TrimSpace(ev.BotID) == "" || strings.TrimSpace(ev.Type) == "" {
		return store.BotEvent{}, store.ErrInvalidArgument
	}
	payload, err := jsonMap(ev.Meta)
	if err != nil {
		return store.BotEvent{}, err
	}
	at := ev.At
	if at.IsZero() {
		at = time.Now().UTC()
	} else {
		at = at.UTC()
	}

	var id int64
	var created time.Time
	var msg *string
	if ev.Message != "" {
		msg = &ev.Message
	}
	err = e.pool.QueryRow(ctx, `
INSERT INTO bot_events (bot_id, event_type, message, payload, created_at)
VALUES ($1::uuid, $2, $3, $4::jsonb, $5)
RETURNING id, created_at`,
		ev.BotID, ev.Type, msg, payload, at,
	).Scan(&id, &created)
	if err != nil {
		return store.BotEvent{}, mapError(err)
	}
	out := store.BotEvent{
		ID:      strconv.FormatInt(id, 10),
		BotID:   ev.BotID,
		Type:    ev.Type,
		Message: ev.Message,
		At:      created,
		Meta:    ev.Meta,
	}
	if out.Meta == nil {
		out.Meta = map[string]any{}
	}
	return out, nil
}

// ListByBot возвращает события бота от старых к новым.
func (e *EventRepo) ListByBot(ctx context.Context, botID string) ([]store.BotEvent, error) {
	rows, err := e.pool.Query(ctx, `
SELECT id, bot_id, event_type, COALESCE(message, ''), payload, created_at
FROM bot_events WHERE bot_id = $1::uuid ORDER BY id ASC`, botID)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()

	out := make([]store.BotEvent, 0)
	for rows.Next() {
		var id int64
		var ev store.BotEvent
		var payload []byte
		if err := rows.Scan(&id, &ev.BotID, &ev.Type, &ev.Message, &payload, &ev.At); err != nil {
			return nil, mapError(err)
		}
		ev.ID = strconv.FormatInt(id, 10)
		ev.Meta, err = decodeJSONMap(payload)
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}
