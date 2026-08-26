package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"mvp-manager/internal/store"
	"mvp-manager/internal/store/postgres"
)

// openTestStore открывает DATABASE_URL_E2E (или DATABASE_URL).
// Без DSN — Skip (юнит-тесты не требуют Docker; e2e-скрипт обязателен).
func openTestStore(t *testing.T) *postgres.Store {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL_E2E")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("DATABASE_URL_E2E/DATABASE_URL не задан — skip postgres unit tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	st, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(st.Close)
	return st
}

func TestPostgresBotPortConflictAndLease(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	// Уникальный порт в широком диапазоне (повторные прогоны не конфликтуют).
	port := 40000 + int(time.Now().UnixNano()%20000)

	nodeID := fmt.Sprintf("pg-test-node-%d", time.Now().UnixNano())
	_, err := st.Nodes.Upsert(ctx, store.Node{
		ID: nodeID, Hostname: "test", Status: store.NodeStatusOnline,
	})
	if err != nil {
		t.Fatalf("upsert node: %v", err)
	}

	rtName := fmt.Sprintf("pg-test-rt-%d", time.Now().UnixNano())
	rt, err := st.Runtimes.Create(ctx, store.Runtime{
		Kind: store.RuntimeKindBotRunner, Name: rtName,
		StartCommand: "./bin/bot-runner", AssignedNodeID: &nodeID,
	})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}

	b1, err := st.Bots.Create(ctx, store.Bot{
		Name: "a", BotType: store.BotTypeDefault, Channel: store.BotChannelTelegram,
		RunMode: store.BotRunModePolling, Port: port, TokenRef: "t1",
		RuntimeID: &rt.ID, AssignedNodeID: &nodeID,
	})
	if err != nil {
		t.Fatalf("create bot: %v", err)
	}
	t.Cleanup(func() { _ = st.Bots.Delete(ctx, b1.ID) })

	_, err = st.Bots.Create(ctx, store.Bot{
		Name: "b", BotType: store.BotTypeDefault, Channel: store.BotChannelTelegram,
		RunMode: store.BotRunModePolling, Port: port, TokenRef: "t2",
	})
	if !errors.Is(err, store.ErrConflict) {
		t.Fatalf("want ErrConflict on duplicate port, got %v", err)
	}

	until := time.Now().UTC().Add(time.Minute)
	// lease_owner REFERENCES nodes(id) — владелец должен существовать в nodes.
	if err := st.Runtimes.TryAcquireLease(ctx, rt.ID, nodeID, until); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	other := nodeID + "-other"
	_, _ = st.Nodes.Upsert(ctx, store.Node{ID: other, Hostname: "other", Status: store.NodeStatusOnline})
	err = st.Runtimes.TryAcquireLease(ctx, rt.ID, other, until)
	if !errors.Is(err, store.ErrLeaseHeld) {
		t.Fatalf("want ErrLeaseHeld, got %v", err)
	}
	if err := st.Runtimes.RenewLease(ctx, rt.ID, nodeID, until.Add(time.Minute)); err != nil {
		t.Fatalf("renew: %v", err)
	}
	if err := st.Runtimes.ReleaseLease(ctx, rt.ID, nodeID); err != nil {
		t.Fatalf("release: %v", err)
	}
}

func TestPostgresListByClientID(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	clientA := "11111111-1111-4111-8111-111111111111"
	clientB := "22222222-2222-4222-8222-222222222222"
	port := 41000 + int(time.Now().UnixNano()%20000)

	a1, err := st.Bots.Create(ctx, store.Bot{
		ClientID: &clientA, Name: "pg-a1", BotType: store.BotTypeDefault,
		Channel: store.BotChannelTelegram, RunMode: store.BotRunModePolling,
		Port: port, TokenRef: "t-a",
	})
	if err != nil {
		t.Fatalf("create a1: %v", err)
	}
	t.Cleanup(func() { _ = st.Bots.Delete(ctx, a1.ID) })

	b1, err := st.Bots.Create(ctx, store.Bot{
		ClientID: &clientB, Name: "pg-b1", BotType: store.BotTypeDefault,
		Channel: store.BotChannelTelegram, RunMode: store.BotRunModePolling,
		Port: port + 1, TokenRef: "t-b",
	})
	if err != nil {
		t.Fatalf("create b1: %v", err)
	}
	t.Cleanup(func() { _ = st.Bots.Delete(ctx, b1.ID) })

	got, err := st.Bots.ListByClientID(ctx, clientA)
	if err != nil {
		t.Fatalf("ListByClientID: %v", err)
	}
	found := false
	for _, b := range got {
		if b.ID == a1.ID {
			found = true
		}
		if b.ID == b1.ID {
			t.Fatalf("client A list contains bot of client B")
		}
	}
	if !found {
		t.Fatalf("client A list missing a1")
	}
}
