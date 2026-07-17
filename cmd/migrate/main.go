// cmd/migrate — применение SQL-миграций (goose) и ручные сиды.
//
// Миграции НЕ запускаются при docker compose up — только этой командой.
//
// DSN:
//
//	по умолчанию DATABASE_URL (dev, mvp_manager);
//	--e2e или MIGRATE_TARGET=e2e → DATABASE_URL_E2E (mvp_manager_e2e);
//	--database-url DSN → явный DSN.
//
// seed по умолчанию пишет только в dev (DATABASE_URL), не в e2e
// (чтобы e2e-прогоны не загрязняли демо-данные).
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"mvp-manager/internal/store/postgres"
	"mvp-manager/migrations"
)

const (
	envDatabaseURL    = "DATABASE_URL"
	envDatabaseURLE2E = "DATABASE_URL_E2E"
	envMigrateTarget  = "MIGRATE_TARGET" // "e2e" → DATABASE_URL_E2E
)

func main() {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	e2e := fs.Bool("e2e", false, "использовать DATABASE_URL_E2E вместо DATABASE_URL")
	dsnFlag := fs.String("database-url", "", "явный DSN (перекрывает ENV и --e2e)")
	_ = fs.Parse(os.Args[1:])

	args := fs.Args()
	if len(args) == 0 {
		printUsage()
		os.Exit(2)
	}

	cmd := args[0]
	switch cmd {
	case "up", "down", "status", "version":
		dsn, label, err := resolveDSN(*dsnFlag, *e2e, false)
		if err != nil {
			fatal(err)
		}
		if err := runGoose(dsn, label, cmd); err != nil {
			fatal(err)
		}
	case "seed":
		// Сиды по умолчанию — только dev. Явный --e2e / --database-url разрешены,
		// но без --e2e никогда не берём DATABASE_URL_E2E.
		dsn, label, err := resolveDSN(*dsnFlag, *e2e, true)
		if err != nil {
			fatal(err)
		}
		if err := runSeed(dsn, label); err != nil {
			fatal(err)
		}
	case "-h", "--help", "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "migrate: неизвестная команда %q\n\n", cmd)
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Print(`mvp-manager migrate — goose-миграции и ручные сиды

Использование:
  migrate [--e2e|--database-url DSN] up|down|status|version
  migrate [--database-url DSN] seed          # по умолчанию DATABASE_URL (dev)
  migrate --e2e seed                        # явно в e2e-БД (редко нужно)

ENV:
  DATABASE_URL       — dev (mvp_manager)
  DATABASE_URL_E2E   — e2e (mvp_manager_e2e)
  MIGRATE_TARGET=e2e — то же, что --e2e

Порядок локального старта:
  docker compose up -d
  go run ./cmd/migrate up
  go run ./cmd/migrate seed
  STORE=postgres ./bin/agent
`)
}

// resolveDSN выбирает DSN.
// seedOnlyDev: без --e2e и без явного DSN всегда DATABASE_URL (даже если MIGRATE_TARGET=e2e —
// для seed игнорируем MIGRATE_TARGET, чтобы случайно не засеять e2e).
func resolveDSN(explicit string, e2eFlag, seedOnlyDev bool) (dsn, label string, err error) {
	if strings.TrimSpace(explicit) != "" {
		return strings.TrimSpace(explicit), "flag", nil
	}
	useE2E := e2eFlag
	if !seedOnlyDev && !useE2E {
		useE2E = strings.EqualFold(strings.TrimSpace(os.Getenv(envMigrateTarget)), "e2e")
	}
	if useE2E {
		dsn = strings.TrimSpace(os.Getenv(envDatabaseURLE2E))
		if dsn == "" {
			return "", "", fmt.Errorf("%s не задан (нужен для --e2e)", envDatabaseURLE2E)
		}
		return dsn, "e2e", nil
	}
	dsn = strings.TrimSpace(os.Getenv(envDatabaseURL))
	if dsn == "" {
		return "", "", fmt.Errorf("%s не задан", envDatabaseURL)
	}
	return dsn, "dev", nil
}

func runGoose(dsn, label, cmd string) error {
	fmt.Printf("migrate %s (%s)\n", cmd, label)

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("sql open: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping: %w", err)
	}

	// goose + embed.FS (см. Context7 /pressly/goose).
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("goose dialect: %w", err)
	}

	// Каталог внутри embed.FS — корень пакета migrations (файлы *.sql рядом).
	const dir = "."
	switch cmd {
	case "up":
		return goose.UpContext(ctx, db, dir)
	case "down":
		return goose.DownContext(ctx, db, dir)
	case "status":
		return goose.StatusContext(ctx, db, dir)
	case "version":
		v, err := goose.GetDBVersionContext(ctx, db)
		if err != nil {
			return err
		}
		fmt.Println(v)
		return nil
	default:
		return fmt.Errorf("unknown goose cmd %q", cmd)
	}
}

func runSeed(dsn, label string) error {
	fmt.Printf("migrate seed (%s)\n", label)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st, err := postgres.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer st.Close()

	if err := postgres.SeedDemo(ctx, st); err != nil {
		return err
	}
	fmt.Println("seed OK: client A (2 bots) + client B (1 bot), desired=stopped")
	fmt.Printf("  client_a=%s\n  client_b=%s\n", postgres.SeedClientA, postgres.SeedClientB)
	return nil
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
	os.Exit(1)
}
