package store

import "errors"

// Sentinel-ошибки контракта хранилища.
// Реализации (memory, postgres) обязаны возвращать их через errors.Is,
// чтобы бизнес-логика не зависела от текста сообщения или драйвера БД.

var (
	// ErrNotFound — сущность с указанным id/именем не найдена.
	ErrNotFound = errors.New("store: not found")

	// ErrConflict — нарушение уникальности или конфликтующее состояние
	// (например UNIQUE port у bots, UNIQUE name у runtimes).
	ErrConflict = errors.New("store: conflict")

	// ErrInvalidArgument — нарушен инвариант модели до записи
	// (например custom_name обязателен только для bot_type=custom).
	ErrInvalidArgument = errors.New("store: invalid argument")

	// ErrLeaseHeld — runtime уже под валидным lease другого владельца
	// (TryAcquireLease / RenewLease / ReleaseLease).
	ErrLeaseHeld = errors.New("store: lease held by another owner")

	// ErrLimitExceeded — превышен лимит (например MAX_BOTS_PER_NODE).
	ErrLimitExceeded = errors.New("store: limit exceeded")
)
