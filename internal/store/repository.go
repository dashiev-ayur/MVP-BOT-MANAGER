package store

import (
	"context"
	"time"
)

// NodeRepository — регистрация ноды агента и heartbeat (ТЗ §6.1).
//
// Потребители: agent при старте и в цикле reconcile (обновление last_seen_at).
// Реализации не должны импортироваться бизнес-пакетами напрямую (DIP).
type NodeRepository interface {
	// Upsert создаёт ноду или обновляет известные поля существующей (hostname, status, agent_version, meta).
	// LastSeenAt при upsert обычно выставляется реализацией в «сейчас», если не задан явно.
	Upsert(ctx context.Context, node Node) (Node, error)

	// ByID возвращает ноду или ErrNotFound.
	ByID(ctx context.Context, id string) (Node, error)

	// List возвращает все известные ноды (порядок не гарантирован).
	List(ctx context.Context) ([]Node, error)

	// Heartbeat обновляет last_seen_at и status живой ноды.
	// Если ноды нет — ErrNotFound (агент должен сначала Upsert).
	Heartbeat(ctx context.Context, id string, at time.Time, status NodeStatus) error
}

// RuntimeRepository — OS-процессы bot_runner / custom_bot (ТЗ §6.2).
//
// Потребители: reconcile/supervisor (actual, pid, lease), ctl (desired).
type RuntimeRepository interface {
	// Create сохраняет новый runtime.
	// Если r.ID пуст — реализация генерирует UUID-строку.
	// Конфликт UNIQUE name → ErrConflict.
	Create(ctx context.Context, r Runtime) (Runtime, error)

	// ByID возвращает runtime или ErrNotFound.
	ByID(ctx context.Context, id string) (Runtime, error)

	// ByName возвращает runtime по уникальному имени или ErrNotFound.
	ByName(ctx context.Context, name string) (Runtime, error)

	// List возвращает все runtimes.
	List(ctx context.Context) ([]Runtime, error)

	// ListByNode — runtimes с assigned_node_id = nodeID (для reconcile на ноде).
	ListByNode(ctx context.Context, nodeID string) ([]Runtime, error)

	// Update полностью заменяет изменяемые поля runtime (кроме CreatedAt).
	// Нет записи → ErrNotFound; конфликт name → ErrConflict.
	Update(ctx context.Context, r Runtime) (Runtime, error)

	// UpdateDesiredState меняет только desired_state (типичный путь ctl start/stop).
	UpdateDesiredState(ctx context.Context, id string, desired DesiredState) error

	// UpdateActual меняет фактическое состояние после действий supervisor
	// (actual_state, pid, exit_code, last_error).
	UpdateActual(ctx context.Context, id string, patch RuntimeActualPatch) error

	// UpdateLease обновляет lease_owner / lease_until.
	// owner/until = nil означает сброс соответствующего поля в NULL.
	UpdateLease(ctx context.Context, id string, owner *string, until *time.Time) error

	// TryAcquireLease атомарно захватывает lease для owner до until.
	// Успех, если lease свободен, истёк (lease_until < now) или уже принадлежит owner.
	// Иначе ErrLeaseHeld. Нет runtime → ErrNotFound.
	TryAcquireLease(ctx context.Context, id string, owner string, until time.Time) error

	// RenewLease продлевает lease_until только если текущий lease_owner == owner
	// и lease ещё не истёк (или уже наш). Иначе ErrLeaseHeld / ErrNotFound.
	RenewLease(ctx context.Context, id string, owner string, until time.Time) error

	// ReleaseLease сбрасывает lease, только если owner совпадает (или lease уже пуст/истёк).
	// Чужой валидный lease → ErrLeaseHeld.
	ReleaseLease(ctx context.Context, id string, owner string) error
}

// BotRepository — логические боты клиентов (ТЗ §6.3).
//
// Потребители: ctl (create/list/desired), reconcile/runner (actual, привязка runtime).
type BotRepository interface {
	// Create сохраняет нового бота.
	// Если b.ID пуст — реализация генерирует UUID-строку.
	// UNIQUE port → ErrConflict; нарушение custom_name ↔ bot_type → ErrInvalidArgument.
	Create(ctx context.Context, b Bot) (Bot, error)

	// ByID возвращает бота или ErrNotFound.
	ByID(ctx context.Context, id string) (Bot, error)

	// List возвращает всех ботов.
	List(ctx context.Context) ([]Bot, error)

	// ListByNode — боты с assigned_node_id = nodeID.
	ListByNode(ctx context.Context, nodeID string) ([]Bot, error)

	// ListByRuntime — боты, привязанные к runtime_id (набор инстансов runner’а).
	ListByRuntime(ctx context.Context, runtimeID string) ([]Bot, error)

	// ListByClientID — боты с client_id = clientID (UUID-строка).
	// Боты без client_id не входят в выборку. Нет совпадений → пустой слайс, не ErrNotFound.
	ListByClientID(ctx context.Context, clientID string) ([]Bot, error)

	// Update полностью заменяет изменяемые поля бота (кроме CreatedAt).
	Update(ctx context.Context, b Bot) (Bot, error)

	// UpdateDesiredState меняет только desired_state (ctl start/stop).
	UpdateDesiredState(ctx context.Context, id string, desired DesiredState) error

	// UpdateActual меняет фактическое состояние бота (runner/agent после reconcile).
	UpdateActual(ctx context.Context, id string, patch BotActualPatch) error

	// Delete удаляет бота по id. Нет записи → ErrNotFound.
	Delete(ctx context.Context, id string) error
}

// EventRepository — аудит действий над ботом (ТЗ §11 GET …/events).
//
// Потребители: ctl / control-api при start/stop/migrate; API для чтения.
type EventRepository interface {
	// Append добавляет событие. Пустой ID → генерируется UUID.
	Append(ctx context.Context, ev BotEvent) (BotEvent, error)

	// ListByBot возвращает события бота (порядок: от старых к новым).
	ListByBot(ctx context.Context, botID string) ([]BotEvent, error)
}
