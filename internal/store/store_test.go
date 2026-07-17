package store_test

import (
	"go/build"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"mvp-manager/internal/store"
)

// Запрещённые импорты: порт store не должен тянуть SQL/драйверы (DIP, Phase 0.3).
var forbiddenImportPrefixes = []string{
	"database/sql",
	"github.com/jackc/pgx",
	"github.com/lib/pq",
	"github.com/jackc/pgconn",
}

func TestStorePackageHasNoSQLImports(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(thisFile)

	pkg, err := build.Default.ImportDir(dir, 0)
	if err != nil {
		t.Fatalf("ImportDir(%s): %v", dir, err)
	}

	all := append(append([]string{}, pkg.Imports...), pkg.TestImports...)
	for _, imp := range all {
		for _, bad := range forbiddenImportPrefixes {
			if imp == bad || strings.HasPrefix(imp, bad+"/") {
				t.Errorf("internal/store must not import %q (DIP)", imp)
			}
		}
	}
}

// Compile-only: доменные константы и sentinel доступны снаружи пакета.
func TestDomainConstantsExported(t *testing.T) {
	t.Parallel()

	_ = store.NodeStatusOnline
	_ = store.RuntimeKindBotRunner
	_ = store.DesiredRunning
	_ = store.ActualUnknown
	_ = store.BotTypeCustom
	_ = store.BotChannelTelegram
	_ = store.BotRunModeWebhook

	if store.ErrNotFound == nil || store.ErrConflict == nil || store.ErrInvalidArgument == nil {
		t.Fatal("sentinel errors must be non-nil")
	}
}
