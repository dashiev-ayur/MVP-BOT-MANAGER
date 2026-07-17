package reconcile_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mvp-manager/internal/reconcile"
	"mvp-manager/internal/store"
	"mvp-manager/internal/store/memory"
	"mvp-manager/internal/supervisor"
)

// TestTickStartStop — desired running→процесс→stopped через reconcile.
func TestTickStartStop(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	sup := supervisor.New(2 * time.Second)
	nodeID := "node-test"

	_, err := st.Nodes.Upsert(ctx, store.Node{
		ID: nodeID, Hostname: "h", Status: store.NodeStatusOnline,
	})
	if err != nil {
		t.Fatal(err)
	}

	script := filepath.Join(t.TempDir(), "bot.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec sleep 60\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	nodeRef := nodeID
	rt, err := st.Runtimes.Create(ctx, store.Runtime{
		Kind: store.RuntimeKindCustomBot, Name: "custom-test",
		StartCommand: script, DesiredState: store.DesiredRunning,
		AssignedNodeID: &nodeRef,
	})
	if err != nil {
		t.Fatal(err)
	}
	cname := "testbot"
	_, err = st.Bots.Create(ctx, store.Bot{
		Name: "b", BotType: store.BotTypeCustom, CustomName: &cname,
		Channel: store.BotChannelTelegram, RunMode: store.BotRunModePolling,
		Port: 19091, TokenRef: "t", RuntimeID: &rt.ID,
		DesiredState: store.DesiredRunning, AssignedNodeID: &nodeRef,
	})
	if err != nil {
		t.Fatal(err)
	}

	loop := reconcile.New(nodeID, st.Nodes, st.Runtimes, st.Bots, sup)
	if err := loop.Tick(ctx); err != nil {
		t.Fatalf("Tick start: %v", err)
	}

	rt2, _ := st.Runtimes.ByID(ctx, rt.ID)
	if rt2.ActualState != store.ActualRunning || rt2.PID == nil {
		t.Fatalf("want running with pid, got %+v", rt2)
	}
	if !sup.IsRunning(rt.ID) {
		t.Fatal("supervisor: not running")
	}

	if err := st.Runtimes.UpdateDesiredState(ctx, rt.ID, store.DesiredStopped); err != nil {
		t.Fatal(err)
	}
	if err := loop.Tick(ctx); err != nil {
		t.Fatalf("Tick stop: %v", err)
	}
	rt3, _ := st.Runtimes.ByID(ctx, rt.ID)
	if rt3.ActualState != store.ActualStopped {
		t.Fatalf("want stopped, got %s", rt3.ActualState)
	}
	if sup.IsRunning(rt.ID) {
		t.Fatal("supervisor still running after stop")
	}
}

// TestTickCrashFailed — краш ребёнка → actual=failed, повторный Tick не паникует.
func TestTickCrashFailed(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	sup := supervisor.New(time.Second)
	nodeID := "node-crash"

	_, _ = st.Nodes.Upsert(ctx, store.Node{ID: nodeID, Hostname: "h", Status: store.NodeStatusOnline})

	script := filepath.Join(t.TempDir(), "fail.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	nodeRef := nodeID
	rt, err := st.Runtimes.Create(ctx, store.Runtime{
		Kind: store.RuntimeKindCustomBot, Name: "custom-fail",
		StartCommand: script, DesiredState: store.DesiredRunning,
		AssignedNodeID: &nodeRef,
	})
	if err != nil {
		t.Fatal(err)
	}
	cname := "failbot"
	_, err = st.Bots.Create(ctx, store.Bot{
		Name: "b", BotType: store.BotTypeCustom, CustomName: &cname,
		Channel: store.BotChannelTelegram, RunMode: store.BotRunModePolling,
		Port: 19092, TokenRef: "t", RuntimeID: &rt.ID,
		DesiredState: store.DesiredRunning, AssignedNodeID: &nodeRef,
	})
	if err != nil {
		t.Fatal(err)
	}

	loop := reconcile.New(nodeID, st.Nodes, st.Runtimes, st.Bots, sup)
	_ = loop.Tick(ctx) // Start → процесс сразу выйдет

	// Ждём Wait в supervisor.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !sup.IsRunning(rt.ID) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if err := loop.Tick(ctx); err != nil {
		t.Fatalf("Tick after crash: %v", err)
	}
	rt2, _ := st.Runtimes.ByID(ctx, rt.ID)
	if rt2.ActualState != store.ActualFailed {
		t.Fatalf("want failed, got %s", rt2.ActualState)
	}

	// Повторный tick не должен снова стартовать (остаёмся failed).
	_ = loop.Tick(ctx)
	if sup.IsRunning(rt.ID) {
		t.Fatal("не ожидали auto-restart после failed")
	}
}
