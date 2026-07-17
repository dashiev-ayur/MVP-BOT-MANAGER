package ops

import (
	"context"
	"fmt"

	"mvp-manager/internal/store"
)

// CheckBotLimit проверяет, что на nodeID можно разместить ещё один бот
// (create) или что текущее число не превышает лимит (start/migrate).
//
// maxBots <= 0 → лимит выключен.
// adding=true → create: reject если count >= max;
// adding=false → start существующего: reject только если count > max
// (уже «перелимит» после ручного store).
func CheckBotLimit(ctx context.Context, bots store.BotRepository, nodeID string, maxBots int, adding bool) error {
	if maxBots <= 0 || nodeID == "" {
		return nil
	}
	list, err := bots.ListByNode(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("list bots on %s: %w", nodeID, err)
	}
	n := len(list)
	if adding {
		if n >= maxBots {
			return fmt.Errorf(
				"лимит ботов на ноде %s: уже %d, MAX_BOTS_PER_NODE=%d: %w",
				nodeID, n, maxBots, store.ErrLimitExceeded,
			)
		}
		return nil
	}
	if n > maxBots {
		return fmt.Errorf(
			"лимит ботов на ноде %s превышен: %d > MAX_BOTS_PER_NODE=%d: %w",
			nodeID, n, maxBots, store.ErrLimitExceeded,
		)
	}
	return nil
}
