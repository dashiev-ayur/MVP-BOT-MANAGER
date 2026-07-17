package lease_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"mvp-manager/internal/lease"
	"mvp-manager/internal/store"
	"mvp-manager/internal/store/memory"
)

// TestAcquireRace_TwoNodesSharedFile — два агента с разными NODE_ID и общим
// MEMORY_STORE_PATH: второй не захватывает чужой runtime.
func TestAcquireRace_TwoNodesSharedFile(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "store.json")

	stA, err := memory.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	stB, err := memory.Open(path)
	if err != nil {
		t.Fatal(err)
	}

	nodeA := "node-a"
	nodeB := "node-b"
	_, _ = stA.Nodes.Upsert(ctx, store.Node{ID: nodeA, Hostname: "a", Status: store.NodeStatusOnline})
	_, _ = stA.Nodes.Upsert(ctx, store.Node{ID: nodeB, Hostname: "b", Status: store.NodeStatusOnline})

	nodeRef := nodeA
	rt, err := stA.Runtimes.Create(ctx, store.Runtime{
		Kind: store.RuntimeKindCustomBot, Name: "custom-race",
		StartCommand: "./bot", DesiredState: store.DesiredRunning,
		AssignedNodeID: &nodeRef,
	})
	if err != nil {
		t.Fatal(err)
	}

	ttl := 15 * time.Second
	leaseA := lease.New(nodeA, ttl, stA.Runtimes)
	leaseB := lease.New(nodeB, ttl, stB.Runtimes)

	if err := leaseA.Acquire(ctx, rt.ID); err != nil {
		t.Fatalf("node-a acquire: %v", err)
	}

	err = leaseB.Acquire(ctx, rt.ID)
	if !lease.IsHeld(err) {
		t.Fatalf("node-b должен получить ErrLeaseHeld, got %v", err)
	}

	holdsA, err := leaseA.Holds(ctx, rt.ID)
	if err != nil || !holdsA {
		t.Fatalf("node-a must hold: holds=%v err=%v", holdsA, err)
	}
	holdsB, err := leaseB.Holds(ctx, rt.ID)
	if err != nil || holdsB {
		t.Fatalf("node-b must not hold: holds=%v err=%v", holdsB, err)
	}
}

// TestAcquireRace_ConcurrentSameStore — параллельный Acquire в одном процессе.
func TestAcquireRace_ConcurrentSameStore(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	nodeRef := "node-x"
	rt, err := st.Runtimes.Create(ctx, store.Runtime{
		Kind: store.RuntimeKindBotRunner, Name: "runner-x",
		StartCommand: "bot-runner", AssignedNodeID: &nodeRef,
	})
	if err != nil {
		t.Fatal(err)
	}

	const n = 20
	var wg sync.WaitGroup
	wins := make(chan string, n)
	owners := []string{"node-a", "node-b", "node-c"}
	for i := 0; i < n; i++ {
		wg.Add(1)
		owner := owners[i%3]
		go func(owner string) {
			defer wg.Done()
			m := lease.New(owner, time.Minute, st.Runtimes)
			if err := m.Acquire(ctx, rt.ID); err == nil {
				wins <- owner
			} else if !lease.IsHeld(err) && !errors.Is(err, store.ErrLeaseHeld) {
				t.Errorf("unexpected: %v", err)
			}
		}(owner)
	}
	wg.Wait()
	close(wins)

	seen := map[string]bool{}
	for w := range wins {
		seen[w] = true
	}
	if len(seen) != 1 {
		t.Fatalf("ожидался ровно один победитель lease, got %v", seen)
	}
}

func TestRenewAndRelease(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	nodeRef := "n1"
	rt, _ := st.Runtimes.Create(ctx, store.Runtime{
		Kind: store.RuntimeKindCustomBot, Name: "c1",
		StartCommand: "x", AssignedNodeID: &nodeRef,
	})
	m := lease.New("n1", time.Second, st.Runtimes)
	other := lease.New("n2", time.Second, st.Runtimes)

	if err := m.Acquire(ctx, rt.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.Renew(ctx, rt.ID); err != nil {
		t.Fatal(err)
	}
	if err := other.Renew(ctx, rt.ID); !lease.IsHeld(err) {
		t.Fatalf("other renew: %v", err)
	}
	if err := m.Release(ctx, rt.ID); err != nil {
		t.Fatal(err)
	}
	// После release другой может захватить.
	if err := other.Acquire(ctx, rt.ID); err != nil {
		t.Fatal(err)
	}
}
