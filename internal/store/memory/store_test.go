package memory_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"mvp-manager/internal/store"
	"mvp-manager/internal/store/memory"
)

func TestBotUniquePort(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := memory.New()

	_, err := s.Bots.Create(ctx, store.Bot{
		Name: "a", BotType: store.BotTypeDefault, Channel: store.BotChannelTelegram,
		RunMode: store.BotRunModeWebhook, Port: 8080, TokenRef: "t1",
	})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err = s.Bots.Create(ctx, store.Bot{
		Name: "b", BotType: store.BotTypeDefault, Channel: store.BotChannelTelegram,
		RunMode: store.BotRunModeWebhook, Port: 8080, TokenRef: "t2",
	})
	if !errors.Is(err, store.ErrConflict) {
		t.Fatalf("want ErrConflict on duplicate port, got %v", err)
	}
}

func TestBotCustomNameInvariant(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := memory.New()

	t.Run("custom_requires_name", func(t *testing.T) {
		_, err := s.Bots.Create(ctx, store.Bot{
			Name: "c", BotType: store.BotTypeCustom, Channel: store.BotChannelTelegram,
			RunMode: store.BotRunModeWebhook, Port: 9001, TokenRef: "t",
		})
		if !errors.Is(err, store.ErrInvalidArgument) {
			t.Fatalf("want ErrInvalidArgument, got %v", err)
		}
	})

	t.Run("custom_empty_name", func(t *testing.T) {
		empty := "  "
		_, err := s.Bots.Create(ctx, store.Bot{
			Name: "c", BotType: store.BotTypeCustom, CustomName: &empty,
			Channel: store.BotChannelTelegram, RunMode: store.BotRunModeWebhook,
			Port: 9002, TokenRef: "t",
		})
		if !errors.Is(err, store.ErrInvalidArgument) {
			t.Fatalf("want ErrInvalidArgument, got %v", err)
		}
	})

	t.Run("custom_ok", func(t *testing.T) {
		name := "acme"
		b, err := s.Bots.Create(ctx, store.Bot{
			Name: "c", BotType: store.BotTypeCustom, CustomName: &name,
			Channel: store.BotChannelTelegram, RunMode: store.BotRunModeWebhook,
			Port: 9003, TokenRef: "t",
		})
		if err != nil {
			t.Fatalf("create custom: %v", err)
		}
		if b.CustomName == nil || *b.CustomName != "acme" {
			t.Fatalf("custom_name not stored: %+v", b.CustomName)
		}
	})

	t.Run("non_custom_forbids_name", func(t *testing.T) {
		name := "nope"
		_, err := s.Bots.Create(ctx, store.Bot{
			Name: "d", BotType: store.BotTypeDefault, CustomName: &name,
			Channel: store.BotChannelTelegram, RunMode: store.BotRunModeWebhook,
			Port: 9004, TokenRef: "t",
		})
		if !errors.Is(err, store.ErrInvalidArgument) {
			t.Fatalf("want ErrInvalidArgument, got %v", err)
		}
	})

	t.Run("non_custom_nil_ok", func(t *testing.T) {
		_, err := s.Bots.Create(ctx, store.Bot{
			Name: "e", BotType: store.BotTypeDefault,
			Channel: store.BotChannelTelegram, RunMode: store.BotRunModeWebhook,
			Port: 9005, TokenRef: "t",
		})
		if err != nil {
			t.Fatalf("default without custom_name: %v", err)
		}
	})
}

func TestRuntimeUniqueNameAndCRUD(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := memory.New()

	rt, err := s.Runtimes.Create(ctx, store.Runtime{
		Kind: store.RuntimeKindCustomBot, Name: "runner-1", StartCommand: "./bot",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if rt.ID == "" {
		t.Fatal("expected generated id")
	}

	_, err = s.Runtimes.Create(ctx, store.Runtime{
		Kind: store.RuntimeKindCustomBot, Name: "runner-1", StartCommand: "./bot2",
	})
	if !errors.Is(err, store.ErrConflict) {
		t.Fatalf("want ErrConflict on duplicate name, got %v", err)
	}

	got, err := s.Runtimes.ByName(ctx, "runner-1")
	if err != nil {
		t.Fatalf("ByName: %v", err)
	}
	if got.ID != rt.ID {
		t.Fatalf("ByName id mismatch: %s vs %s", got.ID, rt.ID)
	}

	if err := s.Runtimes.UpdateDesiredState(ctx, rt.ID, store.DesiredRunning); err != nil {
		t.Fatalf("UpdateDesiredState: %v", err)
	}
	pid := 42
	if err := s.Runtimes.UpdateActual(ctx, rt.ID, store.RuntimeActualPatch{
		ActualState: store.ActualRunning, PID: &pid,
	}); err != nil {
		t.Fatalf("UpdateActual: %v", err)
	}
	got, err = s.Runtimes.ByID(ctx, rt.ID)
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if got.DesiredState != store.DesiredRunning || got.ActualState != store.ActualRunning {
		t.Fatalf("desired/actual not updated: %+v", got)
	}
	if got.PID == nil || *got.PID != 42 {
		t.Fatalf("pid not set: %+v", got.PID)
	}
}

func TestNodeUpsertHeartbeat(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := memory.New()

	n, err := s.Nodes.Upsert(ctx, store.Node{
		ID: "node-1", Hostname: "host", Status: store.NodeStatusOnline,
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if n.LastSeenAt.IsZero() {
		t.Fatal("LastSeenAt should be set on upsert")
	}

	at := time.Now().UTC().Add(time.Minute)
	if err := s.Nodes.Heartbeat(ctx, "node-1", at, store.NodeStatusOnline); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	got, err := s.Nodes.ByID(ctx, "node-1")
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if !got.LastSeenAt.Equal(at) {
		t.Fatalf("LastSeenAt=%v want %v", got.LastSeenAt, at)
	}

	err = s.Nodes.Heartbeat(ctx, "missing", at, store.NodeStatusOnline)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestBotCRUDDesiredActual(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := memory.New()

	b, err := s.Bots.Create(ctx, store.Bot{
		Name: "b1", BotType: store.BotTypeDefault, Channel: store.BotChannelMax,
		RunMode: store.BotRunModePolling, Port: 7001, TokenRef: "tok",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.Bots.UpdateDesiredState(ctx, b.ID, store.DesiredRunning); err != nil {
		t.Fatalf("UpdateDesiredState: %v", err)
	}
	msg := "boom"
	if err := s.Bots.UpdateActual(ctx, b.ID, store.BotActualPatch{
		ActualState: store.ActualFailed, LastError: &msg,
	}); err != nil {
		t.Fatalf("UpdateActual: %v", err)
	}

	got, err := s.Bots.ByID(ctx, b.ID)
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if got.DesiredState != store.DesiredRunning || got.ActualState != store.ActualFailed {
		t.Fatalf("states: %+v", got)
	}
	if got.LastError == nil || *got.LastError != "boom" {
		t.Fatalf("last_error: %+v", got.LastError)
	}

	if err := s.Bots.Delete(ctx, b.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = s.Bots.ByID(ctx, b.ID)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("want ErrNotFound after delete, got %v", err)
	}

	// Порт освобождён — повторный create на том же port должен пройти.
	_, err = s.Bots.Create(ctx, store.Bot{
		Name: "b2", BotType: store.BotTypeDefault, Channel: store.BotChannelMax,
		RunMode: store.BotRunModePolling, Port: 7001, TokenRef: "tok2",
	})
	if err != nil {
		t.Fatalf("reuse port after delete: %v", err)
	}
}

func TestBotListByClientID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := memory.New()

	clientA := "11111111-1111-4111-8111-111111111111"
	clientB := "22222222-2222-4222-8222-222222222222"

	a1, err := s.Bots.Create(ctx, store.Bot{
		ClientID: &clientA, Name: "a1", BotType: store.BotTypeDefault,
		Channel: store.BotChannelTelegram, RunMode: store.BotRunModePolling,
		Port: 7101, TokenRef: "t-a1",
	})
	if err != nil {
		t.Fatalf("create a1: %v", err)
	}
	if _, err := s.Bots.Create(ctx, store.Bot{
		ClientID: &clientA, Name: "a2", BotType: store.BotTypeDefault,
		Channel: store.BotChannelTelegram, RunMode: store.BotRunModePolling,
		Port: 7102, TokenRef: "t-a2",
	}); err != nil {
		t.Fatalf("create a2: %v", err)
	}
	if _, err := s.Bots.Create(ctx, store.Bot{
		ClientID: &clientB, Name: "b1", BotType: store.BotTypeDefault,
		Channel: store.BotChannelTelegram, RunMode: store.BotRunModePolling,
		Port: 7103, TokenRef: "t-b1",
	}); err != nil {
		t.Fatalf("create b1: %v", err)
	}
	if _, err := s.Bots.Create(ctx, store.Bot{
		Name: "orphan", BotType: store.BotTypeDefault,
		Channel: store.BotChannelTelegram, RunMode: store.BotRunModePolling,
		Port: 7104, TokenRef: "t-orphan",
	}); err != nil {
		t.Fatalf("create orphan: %v", err)
	}

	got, err := s.Bots.ListByClientID(ctx, clientA)
	if err != nil {
		t.Fatalf("ListByClientID A: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("client A: want 2 bots, got %d", len(got))
	}
	ids := map[string]bool{}
	for _, b := range got {
		ids[b.ID] = true
	}
	if !ids[a1.ID] {
		t.Fatalf("client A list missing a1")
	}

	// UUID в другом регистре должен совпасть.
	gotFold, err := s.Bots.ListByClientID(ctx, strings.ToUpper(clientA))
	if err != nil {
		t.Fatalf("ListByClientID fold: %v", err)
	}
	if len(gotFold) != 2 {
		t.Fatalf("client A (upper): want 2, got %d", len(gotFold))
	}

	gotB, err := s.Bots.ListByClientID(ctx, clientB)
	if err != nil {
		t.Fatalf("ListByClientID B: %v", err)
	}
	if len(gotB) != 1 || gotB[0].Name != "b1" {
		t.Fatalf("client B: %+v", gotB)
	}

	empty, err := s.Bots.ListByClientID(ctx, "33333333-3333-4333-8333-333333333333")
	if err != nil {
		t.Fatalf("ListByClientID unknown: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("unknown client: want empty, got %d", len(empty))
	}
}

func TestConcurrentSafetySmoke(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := memory.New()

	if _, err := s.Nodes.Upsert(ctx, store.Node{
		ID: "n", Hostname: "h", Status: store.NodeStatusOnline,
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	const workers = 16
	const perWorker = 50
	var wg sync.WaitGroup
	errCh := make(chan error, workers*3)

	wg.Add(workers)
	for i := 0; i < workers; i++ {
		i := i
		go func() {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				port := 10000 + i*perWorker + j
				_, err := s.Bots.Create(ctx, store.Bot{
					Name:    fmt.Sprintf("bot-%d-%d", i, j),
					BotType: store.BotTypeDefault, Channel: store.BotChannelTelegram,
					RunMode: store.BotRunModeWebhook, Port: port, TokenRef: "t",
				})
				if err != nil {
					errCh <- err
					return
				}
			}
		}()
	}

	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				_ = s.Nodes.Heartbeat(ctx, "n", time.Now().UTC(), store.NodeStatusOnline)
				_, _ = s.Bots.List(ctx)
				_, _ = s.Nodes.ByID(ctx, "n")
			}
		}()
	}

	wg.Add(workers)
	for i := 0; i < workers; i++ {
		i := i
		go func() {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				name := fmt.Sprintf("rt-%d-%d", i, j)
				_, err := s.Runtimes.Create(ctx, store.Runtime{
					Kind: store.RuntimeKindBotRunner, Name: name, StartCommand: "./r",
				})
				if err != nil {
					errCh <- err
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent op: %v", err)
	}

	bots, err := s.Bots.List(ctx)
	if err != nil {
		t.Fatalf("List bots: %v", err)
	}
	if want := workers * perWorker; len(bots) != want {
		t.Fatalf("bots count=%d want %d", len(bots), want)
	}
	runtimes, err := s.Runtimes.List(ctx)
	if err != nil {
		t.Fatalf("List runtimes: %v", err)
	}
	if want := workers * perWorker; len(runtimes) != want {
		t.Fatalf("runtimes count=%d want %d", len(runtimes), want)
	}
}
