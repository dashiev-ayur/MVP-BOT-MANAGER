// Точка входа healthcheck: опрос GET /healthz webhook-ботов → запись в store.
//
// Процессы НЕ рестартует — это делает agent по reconcile (ТЗ §5.3).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"mvp-manager/internal/config"
	"mvp-manager/internal/health"
	"mvp-manager/internal/storeopen"
)

const version = "0.0.0-dev"

const helpText = `mvp-manager healthcheck — опрос /healthz webhook-ботов

Использование:
  healthcheck [-h|--help]
  healthcheck [-v|--version]
  healthcheck

ENV: NODE_ID, STORE, MEMORY_STORE_PATH (общий с agent),
CHECK_INTERVAL, HTTP_TIMEOUT, FAILURE_THRESHOLD,
HEALTHCHECK_ALL_NODES (по умолчанию только локальная нода).

Не рестартует процессы — только пишет actual_state/last_error в store.
`

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-v", "--version", "version":
			fmt.Printf("healthcheck %s\n", version)
			return
		case "-h", "--help", "help":
			fmt.Print(helpText)
			return
		}
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: конфиг: %v\n", err)
		os.Exit(1)
	}

	st, storeKind, err := storeopen.Open(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: store: %v\n", err)
		os.Exit(1)
	}

	slog.Info("healthcheck старт",
		"node_id", cfg.NodeID,
		"store", storeKind,
		"memory_store_path", cfg.MemoryStorePath,
		"check_interval", cfg.CheckInterval.String(),
		"failure_threshold", cfg.FailureThreshold,
		"all_nodes", cfg.HealthcheckAllNodes,
	)

	chk := health.New(cfg.NodeID, st.Bots)
	chk.AllNodes = cfg.HealthcheckAllNodes
	chk.Interval = cfg.CheckInterval
	chk.HTTPTimeout = cfg.HTTPTimeout
	chk.FailureThreshold = cfg.FailureThreshold

	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := chk.Run(runCtx); err != nil && runCtx.Err() == nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		os.Exit(1)
	}
	slog.Info("healthcheck остановлен")
}
