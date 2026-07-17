package memory

import (
	"context"
	"fmt"
	"time"

	"mvp-manager/internal/store"
)

// Upsert создаёт ноду или обновляет известные поля существующей
// (hostname, status, agent_version, meta). LastSeenAt при zero → now().
func (r *NodeRepo) Upsert(ctx context.Context, node store.Node) (store.Node, error) {
	if err := checkCtx(ctx); err != nil {
		return store.Node{}, err
	}
	if node.ID == "" {
		return store.Node{}, fmt.Errorf("upsert node: empty id: %w", store.ErrInvalidArgument)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	ts := now()
	existing, ok := r.s.nodes[node.ID]
	if !ok {
		if node.CreatedAt.IsZero() {
			node.CreatedAt = ts
		}
		node.UpdatedAt = ts
		if node.LastSeenAt.IsZero() {
			node.LastSeenAt = ts
		}
		if node.Meta == nil {
			node.Meta = map[string]any{}
		} else {
			node.Meta = cloneStringMap(node.Meta)
		}
		stored := cloneNode(node)
		r.s.nodes[node.ID] = stored
		return cloneNode(stored), nil
	}

	existing.Hostname = node.Hostname
	existing.Status = node.Status
	if node.AgentVersion != nil {
		v := *node.AgentVersion
		existing.AgentVersion = &v
	} else {
		existing.AgentVersion = nil
	}
	if node.Meta != nil {
		existing.Meta = cloneStringMap(node.Meta)
	}
	if node.LastSeenAt.IsZero() {
		existing.LastSeenAt = ts
	} else {
		existing.LastSeenAt = node.LastSeenAt
	}
	existing.UpdatedAt = ts
	r.s.nodes[node.ID] = existing
	return cloneNode(existing), nil
}

// ByID возвращает ноду или ErrNotFound.
func (r *NodeRepo) ByID(ctx context.Context, id string) (store.Node, error) {
	if err := checkCtx(ctx); err != nil {
		return store.Node{}, err
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	n, ok := r.s.nodes[id]
	if !ok {
		return store.Node{}, store.ErrNotFound
	}
	return cloneNode(n), nil
}

// List возвращает все ноды (порядок не гарантирован).
func (r *NodeRepo) List(ctx context.Context) ([]store.Node, error) {
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	out := make([]store.Node, 0, len(r.s.nodes))
	for _, n := range r.s.nodes {
		out = append(out, cloneNode(n))
	}
	return out, nil
}

// Heartbeat обновляет last_seen_at и status живой ноды.
// Если ноды нет — ErrNotFound (агент должен сначала Upsert).
func (r *NodeRepo) Heartbeat(ctx context.Context, id string, at time.Time, status store.NodeStatus) error {
	if err := checkCtx(ctx); err != nil {
		return err
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	n, ok := r.s.nodes[id]
	if !ok {
		return store.ErrNotFound
	}
	n.LastSeenAt = at
	n.Status = status
	n.UpdatedAt = now()
	r.s.nodes[id] = n
	return nil
}
