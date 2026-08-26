package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mvp-manager/internal/store"
)

// BotRepo реализует store.BotRepository.
type BotRepo struct{ pool *pgxpool.Pool }

const botCols = `id, client_id, name, bot_type, custom_name, channel, run_mode,
  port, token_ref, runtime_id, artifact_path, repo_url, start_command,
  desired_state, actual_state, assigned_node_id, last_error, config_version,
  scenario_config, created_at, updated_at`

// validateCustomName — инвариант ТЗ §6.3 (как в memory).
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
	if b.DesiredState == "" {
		b.DesiredState = store.DesiredStopped
	}
	if b.ActualState == "" {
		b.ActualState = store.ActualUnknown
	}
	if b.ConfigVersion == 0 {
		b.ConfigVersion = 1
	}
	sc, err := jsonMap(b.ScenarioConfig)
	if err != nil {
		return store.Bot{}, err
	}

	row := r.pool.QueryRow(ctx, `
INSERT INTO bots (
  id, client_id, name, bot_type, custom_name, channel, run_mode,
  port, token_ref, runtime_id, artifact_path, repo_url, start_command,
  desired_state, actual_state, assigned_node_id, last_error, config_version,
  scenario_config, created_at, updated_at
) VALUES (
  $1::uuid, $2::uuid, $3, $4::bot_type, $5, $6::bot_channel, $7::bot_run_mode,
  $8, $9, $10::uuid, $11, $12, $13,
  $14::desired_state, $15::actual_state, $16, $17, $18,
  $19::jsonb, now(), now()
)
RETURNING `+botCols,
		b.ID, b.ClientID, b.Name, string(b.BotType), b.CustomName, string(b.Channel), string(b.RunMode),
		b.Port, b.TokenRef, b.RuntimeID, b.ArtifactPath, b.RepoURL, b.StartCommand,
		string(b.DesiredState), string(b.ActualState), b.AssignedNodeID, b.LastError, b.ConfigVersion,
		sc,
	)
	out, err := scanBot(row)
	if err != nil {
		return store.Bot{}, mapError(err)
	}
	return out, nil
}

// ByID возвращает бота или ErrNotFound.
func (r *BotRepo) ByID(ctx context.Context, id string) (store.Bot, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+botCols+` FROM bots WHERE id = $1::uuid`, id)
	out, err := scanBot(row)
	if err != nil {
		return store.Bot{}, mapError(err)
	}
	return out, nil
}

// List возвращает всех ботов.
func (r *BotRepo) List(ctx context.Context) ([]store.Bot, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+botCols+` FROM bots`)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()
	return collectBots(rows)
}

// ListByNode — боты с assigned_node_id = nodeID.
func (r *BotRepo) ListByNode(ctx context.Context, nodeID string) ([]store.Bot, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+botCols+` FROM bots WHERE assigned_node_id = $1`, nodeID)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()
	return collectBots(rows)
}

// ListByRuntime — боты, привязанные к runtime_id.
func (r *BotRepo) ListByRuntime(ctx context.Context, runtimeID string) ([]store.Bot, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+botCols+` FROM bots WHERE runtime_id = $1::uuid`, runtimeID)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()
	return collectBots(rows)
}

// ListByClientID — боты с client_id = clientID.
func (r *BotRepo) ListByClientID(ctx context.Context, clientID string) ([]store.Bot, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+botCols+` FROM bots WHERE client_id = $1::uuid`, clientID)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()
	return collectBots(rows)
}

// Update полностью заменяет изменяемые поля (кроме CreatedAt).
func (r *BotRepo) Update(ctx context.Context, b store.Bot) (store.Bot, error) {
	if b.ID == "" {
		return store.Bot{}, fmt.Errorf("bot update: empty id: %w", store.ErrInvalidArgument)
	}
	normalized, err := validateCustomName(b.BotType, b.CustomName)
	if err != nil {
		return store.Bot{}, err
	}
	b.CustomName = normalized
	sc, err := jsonMap(b.ScenarioConfig)
	if err != nil {
		return store.Bot{}, err
	}

	row := r.pool.QueryRow(ctx, `
UPDATE bots SET
  client_id = $2::uuid, name = $3, bot_type = $4::bot_type, custom_name = $5,
  channel = $6::bot_channel, run_mode = $7::bot_run_mode, port = $8, token_ref = $9,
  runtime_id = $10::uuid, artifact_path = $11, repo_url = $12, start_command = $13,
  desired_state = $14::desired_state, actual_state = $15::actual_state,
  assigned_node_id = $16, last_error = $17, config_version = $18,
  scenario_config = $19::jsonb, updated_at = now()
WHERE id = $1::uuid
RETURNING `+botCols,
		b.ID, b.ClientID, b.Name, string(b.BotType), b.CustomName,
		string(b.Channel), string(b.RunMode), b.Port, b.TokenRef,
		b.RuntimeID, b.ArtifactPath, b.RepoURL, b.StartCommand,
		string(b.DesiredState), string(b.ActualState), b.AssignedNodeID, b.LastError, b.ConfigVersion,
		sc,
	)
	out, err := scanBot(row)
	if err != nil {
		return store.Bot{}, mapError(err)
	}
	return out, nil
}

// UpdateDesiredState меняет только desired_state.
func (r *BotRepo) UpdateDesiredState(ctx context.Context, id string, desired store.DesiredState) error {
	tag, err := r.pool.Exec(ctx, `
UPDATE bots SET desired_state = $2::desired_state, updated_at = now() WHERE id = $1::uuid`,
		id, string(desired))
	if err != nil {
		return mapError(err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

// UpdateActual меняет actual-поля бота.
func (r *BotRepo) UpdateActual(ctx context.Context, id string, patch store.BotActualPatch) error {
	tag, err := r.pool.Exec(ctx, `
UPDATE bots SET actual_state = $2::actual_state, last_error = $3, updated_at = now()
WHERE id = $1::uuid`,
		id, string(patch.ActualState), patch.LastError)
	if err != nil {
		return mapError(err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

// Delete удаляет бота по id.
func (r *BotRepo) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM bots WHERE id = $1::uuid`, id)
	if err != nil {
		return mapError(err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

func collectBots(rows pgx.Rows) ([]store.Bot, error) {
	var out []store.Bot
	for rows.Next() {
		b, err := scanBot(rows)
		if err != nil {
			return nil, mapError(err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func scanBot(row scannable) (store.Bot, error) {
	var b store.Bot
	var botType, channel, runMode, desired, actual string
	var sc []byte
	err := row.Scan(
		&b.ID, &b.ClientID, &b.Name, &botType, &b.CustomName, &channel, &runMode,
		&b.Port, &b.TokenRef, &b.RuntimeID, &b.ArtifactPath, &b.RepoURL, &b.StartCommand,
		&desired, &actual, &b.AssignedNodeID, &b.LastError, &b.ConfigVersion,
		&sc, &b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		return store.Bot{}, err
	}
	b.BotType = store.BotType(botType)
	b.Channel = store.BotChannel(channel)
	b.RunMode = store.BotRunMode(runMode)
	b.DesiredState = store.DesiredState(desired)
	b.ActualState = store.ActualState(actual)
	b.ScenarioConfig, err = decodeJSONMap(sc)
	if err != nil {
		return store.Bot{}, err
	}
	return b, nil
}
