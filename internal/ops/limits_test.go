package ops_test

import (
	"context"
	"errors"
	"testing"

	"mvp-manager/internal/ops"
	"mvp-manager/internal/store"
	"mvp-manager/internal/store/memory"
)

func TestCheckBotLimit(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	node := "n1"
	_, _ = st.Nodes.Upsert(ctx, store.Node{ID: node, Hostname: "h", Status: store.NodeStatusOnline})

	create := func(name string, port int) {
		t.Helper()
		rt, err := st.Runtimes.Create(ctx, store.Runtime{
			Kind: store.RuntimeKindCustomBot, Name: "rt-" + name,
			StartCommand: "true", DesiredState: store.DesiredStopped,
			AssignedNodeID: &node,
		})
		if err != nil {
			t.Fatal(err)
		}
		cn := name
		_, err = st.Bots.Create(ctx, store.Bot{
			Name: name, BotType: store.BotTypeCustom, CustomName: &cn,
			Channel: store.BotChannelTelegram, RunMode: store.BotRunModePolling,
			Port: port, TokenRef: "t", RuntimeID: &rt.ID,
			DesiredState: store.DesiredStopped, AssignedNodeID: &node,
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	if err := ops.CheckBotLimit(ctx, st.Bots, node, 2, true); err != nil {
		t.Fatal(err)
	}
	create("a", 19001)
	create("b", 19002)
	err := ops.CheckBotLimit(ctx, st.Bots, node, 2, true)
	if !errors.Is(err, store.ErrLimitExceeded) {
		t.Fatalf("want ErrLimitExceeded, got %v", err)
	}
	// start существующих при count==max — ок
	if err := ops.CheckBotLimit(ctx, st.Bots, node, 2, false); err != nil {
		t.Fatal(err)
	}
	// 0 = без лимита
	if err := ops.CheckBotLimit(ctx, st.Bots, node, 0, true); err != nil {
		t.Fatal(err)
	}
}
