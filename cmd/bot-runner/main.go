// Точка входа multi-tenant bot-runner: один OS-процесс — N default*-инстансов.
//
// Phase 2: lifecycle без реального Telegram/Max. Общий MEMORY_STORE_PATH с agent.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mvp-manager/internal/config"
	"mvp-manager/internal/runner"
	"mvp-manager/internal/storeopen"
)

const version = "0.0.0-dev"

const helpText = `mvp-manager bot-runner — multi-tenant runtime вшитых default*-ботов

Использование:
  bot-runner [-h|--help]
  bot-runner [-v|--version]
  bot-runner

ENV: NODE_ID, RUNTIME_ID (обяз. для привязки к runtime), STORE=memory,
MEMORY_STORE_PATH (общий JSON с agent/ctl), BOT_RUNNER_HEALTH_PORT (опц.).
`

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-v", "--version", "version":
			fmt.Printf("bot-runner %s\n", version)
			return
		case "-h", "--help", "help":
			fmt.Print(helpText)
			return
		}
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "bot-runner: конфиг: %v\n", err)
		os.Exit(1)
	}
	if cfg.RuntimeID == "" {
		fmt.Fprintf(os.Stderr, "bot-runner: %s обязателен (агент прокидывает при Start)\n", config.EnvRuntimeID)
		os.Exit(1)
	}

	st, storeKind, err := storeopen.Open(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bot-runner: store: %v\n", err)
		os.Exit(1)
	}

	slog.Info("bot-runner старт",
		"node_id", cfg.NodeID,
		"runtime_id", cfg.RuntimeID,
		"store", storeKind,
		"memory_store_path", cfg.MemoryStorePath,
	)

	mgr := runner.New(cfg.NodeID, cfg.RuntimeID, st.Bots)
	mgr.SyncInterval = time.Second
	if cfg.BotRunnerHealthPort != "" {
		mgr.HealthAddr = ":" + cfg.BotRunnerHealthPort
	}

	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := mgr.Run(runCtx); err != nil && runCtx.Err() == nil {
		fmt.Fprintf(os.Stderr, "bot-runner: %v\n", err)
		os.Exit(1)
	}
	slog.Info("bot-runner остановлен")
}
