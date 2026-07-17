package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mvp-manager/internal/store"
)

// RuntimeRepo реализует store.RuntimeRepository (включая lease CAS).
type RuntimeRepo struct{ pool *pgxpool.Pool }

const runtimeCols = `id, kind, name, start_command, workdir, env,
  desired_state, actual_state, assigned_node_id, lease_owner, lease_until,
  pid, exit_code, last_error, config_version, created_at, updated_at`

// Create сохраняет новый runtime.
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
	if rt.DesiredState == "" {
		rt.DesiredState = store.DesiredStopped
	}
	if rt.ActualState == "" {
		rt.ActualState = store.ActualUnknown
	}
	if rt.ConfigVersion == 0 {
		rt.ConfigVersion = 1
	}
	env, err := jsonMap(rt.Env)
	if err != nil {
		return store.Runtime{}, err
	}

	row := r.pool.QueryRow(ctx, `
INSERT INTO runtimes (
  id, kind, name, start_command, workdir, env,
  desired_state, actual_state, assigned_node_id, lease_owner, lease_until,
  pid, exit_code, last_error, config_version, created_at, updated_at
) VALUES (
  $1::uuid, $2::runtime_kind, $3, $4, $5, $6::jsonb,
  $7::desired_state, $8::actual_state, $9, $10, $11,
  $12, $13, $14, $15, now(), now()
)
RETURNING `+runtimeCols,
		rt.ID, string(rt.Kind), rt.Name, rt.StartCommand, rt.Workdir, env,
		string(rt.DesiredState), string(rt.ActualState), rt.AssignedNodeID, rt.LeaseOwner, rt.LeaseUntil,
		rt.PID, rt.ExitCode, rt.LastError, rt.ConfigVersion,
	)
	out, err := scanRuntime(row)
	if err != nil {
		return store.Runtime{}, mapError(err)
	}
	return out, nil
}

// ByID возвращает runtime или ErrNotFound.
func (r *RuntimeRepo) ByID(ctx context.Context, id string) (store.Runtime, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+runtimeCols+` FROM runtimes WHERE id = $1::uuid`, id)
	out, err := scanRuntime(row)
	if err != nil {
		return store.Runtime{}, mapError(err)
	}
	return out, nil
}

// ByName возвращает runtime по имени или ErrNotFound.
func (r *RuntimeRepo) ByName(ctx context.Context, name string) (store.Runtime, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+runtimeCols+` FROM runtimes WHERE name = $1`, name)
	out, err := scanRuntime(row)
	if err != nil {
		return store.Runtime{}, mapError(err)
	}
	return out, nil
}

// List возвращает все runtimes.
func (r *RuntimeRepo) List(ctx context.Context) ([]store.Runtime, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+runtimeCols+` FROM runtimes`)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()
	return collectRuntimes(rows)
}

// ListByNode — runtimes с assigned_node_id = nodeID.
func (r *RuntimeRepo) ListByNode(ctx context.Context, nodeID string) ([]store.Runtime, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+runtimeCols+` FROM runtimes WHERE assigned_node_id = $1`, nodeID)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()
	return collectRuntimes(rows)
}

// Update полностью заменяет изменяемые поля (кроме CreatedAt).
func (r *RuntimeRepo) Update(ctx context.Context, rt store.Runtime) (store.Runtime, error) {
	if rt.ID == "" {
		return store.Runtime{}, fmt.Errorf("runtime update: empty id: %w", store.ErrInvalidArgument)
	}
	if rt.Name == "" {
		return store.Runtime{}, fmt.Errorf("runtime name required: %w", store.ErrInvalidArgument)
	}
	env, err := jsonMap(rt.Env)
	if err != nil {
		return store.Runtime{}, err
	}

	row := r.pool.QueryRow(ctx, `
UPDATE runtimes SET
  kind = $2::runtime_kind, name = $3, start_command = $4, workdir = $5, env = $6::jsonb,
  desired_state = $7::desired_state, actual_state = $8::actual_state,
  assigned_node_id = $9, lease_owner = $10, lease_until = $11,
  pid = $12, exit_code = $13, last_error = $14, config_version = $15,
  updated_at = now()
WHERE id = $1::uuid
RETURNING `+runtimeCols,
		rt.ID, string(rt.Kind), rt.Name, rt.StartCommand, rt.Workdir, env,
		string(rt.DesiredState), string(rt.ActualState), rt.AssignedNodeID, rt.LeaseOwner, rt.LeaseUntil,
		rt.PID, rt.ExitCode, rt.LastError, rt.ConfigVersion,
	)
	out, err := scanRuntime(row)
	if err != nil {
		return store.Runtime{}, mapError(err)
	}
	return out, nil
}

// UpdateDesiredState меняет только desired_state.
func (r *RuntimeRepo) UpdateDesiredState(ctx context.Context, id string, desired store.DesiredState) error {
	tag, err := r.pool.Exec(ctx, `
UPDATE runtimes SET desired_state = $2::desired_state, updated_at = now() WHERE id = $1::uuid`,
		id, string(desired))
	if err != nil {
		return mapError(err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

// UpdateActual меняет actual-поля после supervisor.
func (r *RuntimeRepo) UpdateActual(ctx context.Context, id string, patch store.RuntimeActualPatch) error {
	tag, err := r.pool.Exec(ctx, `
UPDATE runtimes SET
  actual_state = $2::actual_state,
  pid = $3, exit_code = $4, last_error = $5,
  updated_at = now()
WHERE id = $1::uuid`,
		id, string(patch.ActualState), patch.PID, patch.ExitCode, patch.LastError)
	if err != nil {
		return mapError(err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

// UpdateLease обновляет lease_owner / lease_until (nil = NULL).
func (r *RuntimeRepo) UpdateLease(ctx context.Context, id string, owner *string, until *time.Time) error {
	tag, err := r.pool.Exec(ctx, `
UPDATE runtimes SET lease_owner = $2, lease_until = $3, updated_at = now() WHERE id = $1::uuid`,
		id, owner, until)
	if err != nil {
		return mapError(err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

// TryAcquireLease атомарно захватывает lease (свободен / истёк / уже наш).
func (r *RuntimeRepo) TryAcquireLease(ctx context.Context, id string, owner string, until time.Time) error {
	return r.leaseCAS(ctx, id, owner, until, `
UPDATE runtimes SET lease_owner = $2, lease_until = $3, updated_at = now()
WHERE id = $1::uuid
  AND (
    lease_owner IS NULL OR lease_owner = ''
    OR lease_until IS NULL OR lease_until < now()
    OR lease_owner = $2
  )`)
}

// RenewLease продлевает lease только для текущего владельца (или истёкшего «нашего»).
func (r *RuntimeRepo) RenewLease(ctx context.Context, id string, owner string, until time.Time) error {
	return r.leaseCAS(ctx, id, owner, until, `
UPDATE runtimes SET lease_owner = $2, lease_until = $3, updated_at = now()
WHERE id = $1::uuid
  AND lease_owner = $2`)
}

// ReleaseLease сбрасывает lease, если он наш или уже свободен/истёк.
func (r *RuntimeRepo) ReleaseLease(ctx context.Context, id string, owner string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return mapError(err)
	}
	defer tx.Rollback(ctx)

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM runtimes WHERE id = $1::uuid)`, id).Scan(&exists); err != nil {
		return mapError(err)
	}
	if !exists {
		return store.ErrNotFound
	}

	tag, err := tx.Exec(ctx, `
UPDATE runtimes SET lease_owner = NULL, lease_until = NULL, updated_at = now()
WHERE id = $1::uuid
  AND (
    lease_owner IS NULL OR lease_owner = '' OR lease_owner = $2
    OR lease_until IS NULL OR lease_until < now()
  )`, id, owner)
	if err != nil {
		return mapError(err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrLeaseHeld
	}
	return mapError(tx.Commit(ctx))
}

// leaseCAS — общий путь Acquire/Renew: отличить NotFound от LeaseHeld.
func (r *RuntimeRepo) leaseCAS(ctx context.Context, id, owner string, until time.Time, updateSQL string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return mapError(err)
	}
	defer tx.Rollback(ctx)

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM runtimes WHERE id = $1::uuid)`, id).Scan(&exists); err != nil {
		return mapError(err)
	}
	if !exists {
		return store.ErrNotFound
	}

	tag, err := tx.Exec(ctx, updateSQL, id, owner, until.UTC())
	if err != nil {
		return mapError(err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrLeaseHeld
	}
	return mapError(tx.Commit(ctx))
}

func collectRuntimes(rows pgx.Rows) ([]store.Runtime, error) {
	var out []store.Runtime
	for rows.Next() {
		rt, err := scanRuntime(rows)
		if err != nil {
			return nil, mapError(err)
		}
		out = append(out, rt)
	}
	return out, rows.Err()
}

func scanRuntime(row scannable) (store.Runtime, error) {
	var rt store.Runtime
	var kind, desired, actual string
	var env []byte
	err := row.Scan(
		&rt.ID, &kind, &rt.Name, &rt.StartCommand, &rt.Workdir, &env,
		&desired, &actual, &rt.AssignedNodeID, &rt.LeaseOwner, &rt.LeaseUntil,
		&rt.PID, &rt.ExitCode, &rt.LastError, &rt.ConfigVersion, &rt.CreatedAt, &rt.UpdatedAt,
	)
	if err != nil {
		return store.Runtime{}, err
	}
	rt.Kind = store.RuntimeKind(kind)
	rt.DesiredState = store.DesiredState(desired)
	rt.ActualState = store.ActualState(actual)
	rt.Env, err = decodeJSONMap(env)
	if err != nil {
		return store.Runtime{}, err
	}
	return rt, nil
}
