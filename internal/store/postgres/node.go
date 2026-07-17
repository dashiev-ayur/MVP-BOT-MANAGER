package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"mvp-manager/internal/store"
)

// NodeRepo реализует store.NodeRepository.
type NodeRepo struct{ pool *pgxpool.Pool }

// Upsert создаёт или обновляет ноду.
func (r *NodeRepo) Upsert(ctx context.Context, node store.Node) (store.Node, error) {
	if node.ID == "" {
		return store.Node{}, fmt.Errorf("upsert node: empty id: %w", store.ErrInvalidArgument)
	}
	meta, err := jsonMap(node.Meta)
	if err != nil {
		return store.Node{}, err
	}

	// LastSeenAt.IsZero → now() на стороне SQL.
	const q = `
INSERT INTO nodes (id, hostname, status, last_seen_at, agent_version, meta, created_at, updated_at)
VALUES (
  $1, $2, $3,
  CASE WHEN $4::boolean THEN now() ELSE $5 END,
  $6, $7::jsonb, now(), now()
)
ON CONFLICT (id) DO UPDATE SET
  hostname = EXCLUDED.hostname,
  status = EXCLUDED.status,
  agent_version = EXCLUDED.agent_version,
  meta = CASE WHEN $8::boolean THEN EXCLUDED.meta ELSE nodes.meta END,
  last_seen_at = CASE WHEN $4::boolean THEN now() ELSE EXCLUDED.last_seen_at END,
  updated_at = now()
RETURNING id, hostname, status, last_seen_at, agent_version, meta, created_at, updated_at`

	lastSeenZero := node.LastSeenAt.IsZero()
	lastSeen := node.LastSeenAt.UTC()
	metaProvided := node.Meta != nil

	row := r.pool.QueryRow(ctx, q,
		node.ID, node.Hostname, string(node.Status),
		lastSeenZero, lastSeen,
		node.AgentVersion, meta, metaProvided,
	)
	out, err := scanNode(row)
	if err != nil {
		return store.Node{}, mapError(err)
	}
	return out, nil
}

// ByID возвращает ноду или ErrNotFound.
func (r *NodeRepo) ByID(ctx context.Context, id string) (store.Node, error) {
	row := r.pool.QueryRow(ctx, `
SELECT id, hostname, status, last_seen_at, agent_version, meta, created_at, updated_at
FROM nodes WHERE id = $1`, id)
	out, err := scanNode(row)
	if err != nil {
		return store.Node{}, mapError(err)
	}
	return out, nil
}

// List возвращает все ноды.
func (r *NodeRepo) List(ctx context.Context) ([]store.Node, error) {
	rows, err := r.pool.Query(ctx, `
SELECT id, hostname, status, last_seen_at, agent_version, meta, created_at, updated_at
FROM nodes`)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()
	var out []store.Node
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, mapError(err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// Heartbeat обновляет last_seen_at и status.
func (r *NodeRepo) Heartbeat(ctx context.Context, id string, at time.Time, status store.NodeStatus) error {
	tag, err := r.pool.Exec(ctx, `
UPDATE nodes SET last_seen_at = $2, status = $3, updated_at = now() WHERE id = $1`,
		id, at.UTC(), string(status))
	if err != nil {
		return mapError(err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanNode(row scannable) (store.Node, error) {
	var n store.Node
	var status string
	var meta []byte
	err := row.Scan(
		&n.ID, &n.Hostname, &status, &n.LastSeenAt, &n.AgentVersion, &meta, &n.CreatedAt, &n.UpdatedAt,
	)
	if err != nil {
		return store.Node{}, err
	}
	n.Status = store.NodeStatus(status)
	n.Meta, err = decodeJSONMap(meta)
	if err != nil {
		return store.Node{}, err
	}
	return n, nil
}

// jsonMap сериализует map в JSON-байты для jsonb; nil → "{}".
func jsonMap(m map[string]any) ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("json map: %w", err)
	}
	return b, nil
}

func decodeJSONMap(b []byte) (map[string]any, error) {
	if len(b) == 0 || string(b) == "null" {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("decode json map: %w", err)
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}
