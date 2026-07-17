package postgres

import (
	"context"
	"errors"
	"fmt"

	"mvp-manager/internal/store"
)

// Стабильные UUID сидов (документированы в README).
// Клиент A — 2 default-бота; клиент B — 1 default-бот.
const (
	SeedClientA = "11111111-1111-4111-8111-111111111111"
	SeedClientB = "22222222-2222-4222-8222-222222222222"

	SeedBotA1 = "a1111111-1111-4111-8111-111111111111"
	SeedBotA2 = "a2222222-2222-4222-8222-222222222222"
	SeedBotB1 = "b1111111-1111-4111-8111-111111111111"

	// Стабильный UUID runtime (только hex-цифры — требование типа UUID в Postgres).
	SeedRuntimeID = "33333333-3333-4333-8333-333333333333"

	// SeedNodeID совпадает с NODE_ID в .env.example.
	SeedNodeID = "node-1"

	seedRuntimeName = "bot-runner-node-1"
)

// SeedDemo вставляет демо-данные (идемпотентно: ON CONFLICT DO NOTHING / Upsert).
//
// По умолчанию вызывается против DATABASE_URL (dev), не e2e.
func SeedDemo(ctx context.Context, st *Store) error {
	if st == nil {
		return fmt.Errorf("seed: nil store")
	}

	nodeID := SeedNodeID
	if _, err := st.Nodes.Upsert(ctx, store.Node{
		ID:       nodeID,
		Hostname: "localhost",
		Status:   store.NodeStatusOnline,
	}); err != nil {
		return fmt.Errorf("seed node: %w", err)
	}

	// Runtime bot_runner: Create или уже есть по стабильному id/name.
	if _, err := st.Runtimes.ByID(ctx, SeedRuntimeID); err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("seed runtime lookup: %w", err)
		}
		assigned := nodeID
		if _, err := st.Runtimes.Create(ctx, store.Runtime{
			ID:             SeedRuntimeID,
			Kind:           store.RuntimeKindBotRunner,
			Name:           seedRuntimeName,
			StartCommand:   "./bin/bot-runner",
			DesiredState:   store.DesiredStopped,
			ActualState:    store.ActualUnknown,
			AssignedNodeID: &assigned,
		}); err != nil {
			if !isConflict(err) {
				return fmt.Errorf("seed runtime: %w", err)
			}
		}
	}

	clientA := SeedClientA
	clientB := SeedClientB
	rtID := SeedRuntimeID
	assigned := nodeID

	bots := []store.Bot{
		{
			ID: SeedBotA1, ClientID: &clientA, Name: "seed-client-a-bot-1",
			BotType: store.BotTypeDefault, Channel: store.BotChannelTelegram,
			RunMode: store.BotRunModePolling, Port: 19001,
			TokenRef: "seed:client-a-bot-1", RuntimeID: &rtID,
			DesiredState: store.DesiredStopped, AssignedNodeID: &assigned,
		},
		{
			ID: SeedBotA2, ClientID: &clientA, Name: "seed-client-a-bot-2",
			BotType: store.BotTypeDefault, Channel: store.BotChannelTelegram,
			RunMode: store.BotRunModePolling, Port: 19002,
			TokenRef: "seed:client-a-bot-2", RuntimeID: &rtID,
			DesiredState: store.DesiredStopped, AssignedNodeID: &assigned,
		},
		{
			ID: SeedBotB1, ClientID: &clientB, Name: "seed-client-b-bot-1",
			BotType: store.BotTypeDefault, Channel: store.BotChannelTelegram,
			RunMode: store.BotRunModePolling, Port: 19003,
			TokenRef: "seed:client-b-bot-1", RuntimeID: &rtID,
			DesiredState: store.DesiredStopped, AssignedNodeID: &assigned,
		},
	}

	for _, b := range bots {
		_, err := st.Bots.ByID(ctx, b.ID)
		if err == nil {
			continue // уже есть — идемпотентность
		}
		if !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("seed bot lookup %s: %w", b.ID, err)
		}
		if _, err := st.Bots.Create(ctx, b); err != nil {
			if isConflict(err) {
				// Порт занят другим ботом — не дублируем бесконтрольно.
				continue
			}
			return fmt.Errorf("seed bot %s: %w", b.ID, err)
		}
	}
	return nil
}

func isConflict(err error) bool {
	return errors.Is(err, store.ErrConflict)
}
