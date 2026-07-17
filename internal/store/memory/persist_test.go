package memory_test

import (
	"context"
	"path/filepath"
	"testing"

	"mvp-manager/internal/store"
	"mvp-manager/internal/store/memory"
)

// TestFileBackedSharedAcrossOpen проверяет, что два Open одного пути
// видят одни и те же bots (модель agent ↔ ctl при STORE=memory).
func TestFileBackedSharedAcrossOpen(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "store.json")

	a, err := memory.Open(path)
	if err != nil {
		t.Fatalf("Open a: %v", err)
	}
	name := "acme"
	created, err := a.Bots.Create(ctx, store.Bot{
		Name: "b1", BotType: store.BotTypeCustom, CustomName: &name,
		Channel: store.BotChannelTelegram, RunMode: store.BotRunModeWebhook,
		Port: 18080, TokenRef: "tok",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	b, err := memory.Open(path)
	if err != nil {
		t.Fatalf("Open b: %v", err)
	}
	got, err := b.Bots.ByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("ByID via second Open: %v", err)
	}
	if got.Port != 18080 {
		t.Fatalf("port: got %d, want 18080", got.Port)
	}
}
