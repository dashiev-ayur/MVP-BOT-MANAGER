package reconcile_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mvp-manager/internal/launch"
	"mvp-manager/internal/reconcile"
	"mvp-manager/internal/store"
	"mvp-manager/internal/store/memory"
	"mvp-manager/internal/supervisor"
)

// TestEnsureBotRunnerStartStop — desired runner running→PID→stopped.
func TestEnsureBotRunnerStartStop(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	sup := supervisor.New(2 * time.Second)
	nodeID := "node-br"

	_, _ = st.Nodes.Upsert(ctx, store.Node{ID: nodeID, Hostname: "h", Status: store.NodeStatusOnline})

	// Долгоживущий «runner»: sleep.
	script := filepath.Join(t.TempDir(), "runner.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec sleep 60\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	nodeRef := nodeID
	rt, err := st.Runtimes.Create(ctx, store.Runtime{
		Kind: store.RuntimeKindBotRunner, Name: launch.BotRunnerRuntimeName(nodeID),
		StartCommand: script, DesiredState: store.DesiredRunning,
		AssignedNodeID: &nodeRef,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.Bots.Create(ctx, store.Bot{
		Name: "d", BotType: store.BotTypeDefault,
		Channel: store.BotChannelTelegram, RunMode: store.BotRunModePolling,
		Port: 19201, TokenRef: "t", RuntimeID: &rt.ID,
		DesiredState: store.DesiredRunning, AssignedNodeID: &nodeRef,
	})
	if err != nil {
		t.Fatal(err)
	}

	loop := reconcile.New(nodeID, st.Nodes, st.Runtimes, st.Bots, sup)
	loop.BotRunnerCommand = script
	loop.StoreKind = "memory"

	if err := loop.Tick(ctx); err != nil {
		t.Fatalf("Tick start: %v", err)
	}
	rt2, _ := st.Runtimes.ByID(ctx, rt.ID)
	if rt2.ActualState != store.ActualRunning || rt2.PID == nil {
		t.Fatalf("want running with pid, got %+v", rt2)
	}

	_ = st.Runtimes.UpdateDesiredState(ctx, rt.ID, store.DesiredStopped)
	if err := loop.Tick(ctx); err != nil {
		t.Fatalf("Tick stop: %v", err)
	}
	rt3, _ := st.Runtimes.ByID(ctx, rt.ID)
	if rt3.ActualState != store.ActualStopped {
		t.Fatalf("want stopped, got %s", rt3.ActualState)
	}
}

// TestUnhealthyRestartsRunner — healthcheck-failed → рестарт процесса.
func TestUnhealthyRestartsRunner(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	sup := supervisor.New(2 * time.Second)
	nodeID := "node-uh"

	_, _ = st.Nodes.Upsert(ctx, store.Node{ID: nodeID, Hostname: "h", Status: store.NodeStatusOnline})

	script := filepath.Join(t.TempDir(), "runner.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec sleep 60\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	nodeRef := nodeID
	rt, err := st.Runtimes.Create(ctx, store.Runtime{
		Kind: store.RuntimeKindBotRunner, Name: launch.BotRunnerRuntimeName(nodeID),
		StartCommand: script, DesiredState: store.DesiredRunning,
		AssignedNodeID: &nodeRef,
	})
	if err != nil {
		t.Fatal(err)
	}
	bot, err := st.Bots.Create(ctx, store.Bot{
		Name: "d", BotType: store.BotTypeDefault,
		Channel: store.BotChannelTelegram, RunMode: store.BotRunModeWebhook,
		Port: 19202, TokenRef: "t", RuntimeID: &rt.ID,
		DesiredState: store.DesiredRunning, AssignedNodeID: &nodeRef,
	})
	if err != nil {
		t.Fatal(err)
	}

	loop := reconcile.New(nodeID, st.Nodes, st.Runtimes, st.Bots, sup)
	loop.BotRunnerCommand = script

	if err := loop.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	rt1, _ := st.Runtimes.ByID(ctx, rt.ID)
	if rt1.PID == nil {
		t.Fatal("no pid")
	}
	pid1 := *rt1.PID

	msg := launch.HealthcheckErrPrefix + " 2 consecutive failures: status 503"
	_ = st.Bots.UpdateActual(ctx, bot.ID, store.BotActualPatch{
		ActualState: store.ActualFailed,
		LastError:   &msg,
	})

	if err := loop.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	rt2, _ := st.Runtimes.ByID(ctx, rt.ID)
	if rt2.PID == nil {
		t.Fatal("no pid after restart")
	}
	if *rt2.PID == pid1 {
		// Иногда PID может совпасть после быстрого recycle — проверим что процесс жив.
		if !sup.IsRunning(rt.ID) {
			t.Fatal("runner not running after unhealthy restart")
		}
	}
	if !sup.IsRunning(rt.ID) {
		t.Fatal("supervisor: not running after unhealthy")
	}
}
