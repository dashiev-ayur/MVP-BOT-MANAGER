package runner_test

import (
	"go/build"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var forbiddenImportPrefixes = []string{
	"database/sql",
	"github.com/jackc/pgx",
	"github.com/lib/pq",
	"github.com/jackc/pgconn",
}

func TestRunnerPackageHasNoSQLImports(t *testing.T) {
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
				t.Errorf("internal/runner must not import %q (DIP)", imp)
			}
		}
	}
}
