package runner_test

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"testing"
	"time"

	"mvp-manager/internal/runner"
	"mvp-manager/internal/store"
	"mvp-manager/internal/store/memory"
)

// TestSyncTwoWebhooks — два default webhook в одном Manager, оба /healthz.
func TestSyncTwoWebhooks(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	nodeID := "node-r"
	rtID := "runtime-r"

	_, _ = st.Nodes.Upsert(ctx, store.Node{ID: nodeID, Hostname: "h", Status: store.NodeStatusOnline})
	nodeRef := nodeID
	_, err := st.Runtimes.Create(ctx, store.Runtime{
		ID: rtID, Kind: store.RuntimeKindBotRunner, Name: "bot-runner-node-r",
		StartCommand: "bot-runner", DesiredState: store.DesiredRunning,
		AssignedNodeID: &nodeRef,
	})
	if err != nil {
		t.Fatal(err)
	}

	ports := []int{19191, 19192}
	var botIDs []string
	for i, port := range ports {
		b, err := st.Bots.Create(ctx, store.Bot{
			Name: "d", BotType: store.BotTypeDefault,
			Channel: store.BotChannelTelegram, RunMode: store.BotRunModeWebhook,
			Port: port, TokenRef: "t", RuntimeID: &rtID,
			DesiredState: store.DesiredRunning, AssignedNodeID: &nodeRef,
		})
		if err != nil {
			t.Fatalf("create bot %d: %v", i, err)
		}
		botIDs = append(botIDs, b.ID)
	}

	mgr := runner.New(nodeID, rtID, st.Bots)
	if err := mgr.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Sync с пустым desired снимет инстансы.
		for _, id := range botIDs {
			_ = st.Bots.UpdateDesiredState(ctx, id, store.DesiredStopped)
		}
		_ = mgr.Sync(ctx)
	})

	for _, port := range ports {
		waitHTTP(t, port)
	}

	for _, id := range botIDs {
		b, _ := st.Bots.ByID(ctx, id)
		if b.ActualState != store.ActualRunning {
			t.Fatalf("bot %s: want running, got %s", id, b.ActualState)
		}
	}
}

// TestSyncRemoveOnStop — desired=stopped снимает listen.
func TestSyncRemoveOnStop(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	nodeID := "node-rm"
	rtID := "runtime-rm"
	nodeRef := nodeID
	port := 19193

	_, _ = st.Runtimes.Create(ctx, store.Runtime{
		ID: rtID, Kind: store.RuntimeKindBotRunner, Name: "bot-runner-rm",
		StartCommand: "x", DesiredState: store.DesiredRunning, AssignedNodeID: &nodeRef,
	})
	b, err := st.Bots.Create(ctx, store.Bot{
		Name: "d", BotType: store.BotTypeDefault,
		Channel: store.BotChannelTelegram, RunMode: store.BotRunModeWebhook,
		Port: port, TokenRef: "t", RuntimeID: &rtID,
		DesiredState: store.DesiredRunning, AssignedNodeID: &nodeRef,
	})
	if err != nil {
		t.Fatal(err)
	}

	mgr := runner.New(nodeID, rtID, st.Bots)
	if err := mgr.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	waitHTTP(t, port)

	_ = st.Bots.UpdateDesiredState(ctx, b.ID, store.DesiredStopped)
	if err := mgr.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get("http://127.0.0.1:19193/healthz")
	if err == nil {
		resp.Body.Close()
		t.Fatal("ожидали отказ соединения после remove")
	}
	b2, _ := st.Bots.ByID(ctx, b.ID)
	if b2.ActualState != store.ActualStopped {
		t.Fatalf("want stopped, got %s", b2.ActualState)
	}
}

func waitHTTP(t *testing.T, port int) {
	t.Helper()
	url := "http://127.0.0.1:" + strconv.Itoa(port) + "/healthz"
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == 200 && string(body) == "ok\n" {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting healthz on :%d", port)
}

// TestSyncMaxAndTelegram — channel=max и telegram в одном Manager, оба /healthz.
func TestSyncMaxAndTelegram(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	nodeID := "node-ch"
	rtID := "runtime-ch"
	nodeRef := nodeID

	_, err := st.Runtimes.Create(ctx, store.Runtime{
		ID: rtID, Kind: store.RuntimeKindBotRunner, Name: "bot-runner-ch",
		StartCommand: "x", DesiredState: store.DesiredRunning, AssignedNodeID: &nodeRef,
	})
	if err != nil {
		t.Fatal(err)
	}

	tg, err := st.Bots.Create(ctx, store.Bot{
		Name: "tg", BotType: store.BotTypeDefault,
		Channel: store.BotChannelTelegram, RunMode: store.BotRunModeWebhook,
		Port: 19194, TokenRef: "t-tg", RuntimeID: &rtID,
		DesiredState: store.DesiredRunning, AssignedNodeID: &nodeRef,
	})
	if err != nil {
		t.Fatal(err)
	}
	mx, err := st.Bots.Create(ctx, store.Bot{
		Name: "mx", BotType: store.BotTypeDefault,
		Channel: store.BotChannelMax, RunMode: store.BotRunModeWebhook,
		Port: 19195, TokenRef: "t-mx", RuntimeID: &rtID,
		DesiredState: store.DesiredRunning, AssignedNodeID: &nodeRef,
	})
	if err != nil {
		t.Fatal(err)
	}

	mgr := runner.New(nodeID, rtID, st.Bots)
	if err := mgr.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = st.Bots.UpdateDesiredState(ctx, tg.ID, store.DesiredStopped)
		_ = st.Bots.UpdateDesiredState(ctx, mx.ID, store.DesiredStopped)
		_ = mgr.Sync(ctx)
	})

	waitHTTP(t, 19194)
	waitHTTP(t, 19195)
}
