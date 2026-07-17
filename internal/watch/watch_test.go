package watch_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"mvp-manager/internal/watch"
)

func TestFileWatcher_DetectsChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "store.json")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	w := watch.NewFileWatcher(path, 50*time.Millisecond, nil)
	defer w.Close()

	// Дождаться начального snapshot mtime.
	time.Sleep(80 * time.Millisecond)

	if err := os.WriteFile(path, []byte(`{"bots":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case <-w.Events():
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for file change event")
	}
}

func TestOpenForStore_PostgresIsNop(t *testing.T) {
	w := watch.OpenForStore("postgres", "/tmp/x", nil)
	defer w.Close()
	select {
	case <-w.Events():
		t.Fatal("nop should not fire")
	case <-time.After(50 * time.Millisecond):
	}
}
