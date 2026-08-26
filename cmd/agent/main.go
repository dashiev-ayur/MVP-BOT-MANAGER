// Точка входа демона agent: на каждой ноде следит за desired/actual
// состоянием ботов и управляет процессами.
//
// Phase 1: Load конфига → wiring STORE=memory (file-backed) → регистрация
// ноды → heartbeat + reconcile loop → при SIGINT/SIGTERM остановка
// управляемых процессов (shutdown grace).
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
	"mvp-manager/internal/lease"
	"mvp-manager/internal/reconcile"
	"mvp-manager/internal/store"
	"mvp-manager/internal/storeopen"
	"mvp-manager/internal/supervisor"
)

// version — строка версии бинарника; позже можно подставлять через -ldflags.
const version = "0.0.0-dev"

// helpText — краткая справка для пользователя CLI (без бизнес-логики).
const helpText = `mvp-manager agent — демон управления ботами на ноде

Использование:
  agent [-h|--help]
  agent [-v|--version]
  agent

ENV: NODE_ID (обяз.), STORE=memory, MEMORY_STORE_PATH (общий JSON с ctl/runner/healthcheck),
BOT_RUNNER_COMMAND, BOT_RUNNER_WORKDIR (опц.), BOT_RUNNER_HEALTH_PORT (опц.),
LEASE_TTL (дефолт 15s), RECONCILE_INTERVAL, HEARTBEAT_INTERVAL, SHUTDOWN_GRACE, PUBLIC_URL,
RESTART_MAX_ATTEMPTS, RESTART_BACKOFF_BASE, RESTART_BACKOFF_MAX, MAX_BOTS_PER_NODE.
См. .env.example и README.
`

func main() {
	// Минимальный разбор флагов: help / version не трогают конфиг,
	// чтобы --help работал даже без NODE_ID в окружении.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-v", "--version", "version":
			fmt.Printf("agent %s\n", version)
			return
		case "-h", "--help", "help":
			fmt.Print(helpText)
			return
		}
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "agent: конфиг: %v\n", err)
		os.Exit(1)
	}

	st, storeKind, err := storeopen.Open(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "agent: store: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = st.Close() }()

	slog.Info("конфиг загружен",
		"node_id", cfg.NodeID,
		"store", storeKind,
		"bot_runner_command", cfg.BotRunnerCommand,
		"lease_ttl", cfg.LeaseTTL.String(),
		"reconcile_interval", cfg.ReconcileInterval.String(),
		"heartbeat_interval", cfg.HeartbeatInterval.String(),
		"shutdown_grace", cfg.ShutdownGrace.String(),
		"restart_max_attempts", cfg.RestartMaxAttempts,
		"restart_backoff_base", cfg.RestartBackoffBase.String(),
		"max_bots_per_node", cfg.MaxBotsPerNode,
	)
	if storeKind == config.StoreMemory {
		slog.Info("memory store", "path", cfg.MemoryStorePath)
	}

	// Регистрация ноды в store (Upsert по NODE_ID) — агент «появляется» в реестре.
	hostname, herr := os.Hostname()
	if herr != nil || hostname == "" {
		hostname = cfg.NodeID
	}
	ver := version
	ctx := context.Background()
	if _, err := st.Nodes.Upsert(ctx, store.Node{
		ID:           cfg.NodeID,
		Hostname:     hostname,
		Status:       store.NodeStatusOnline,
		AgentVersion: &ver,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "agent: регистрация ноды: %v\n", err)
		os.Exit(1)
	}
	slog.Info("нода зарегистрирована", "node_id", cfg.NodeID, "hostname", hostname)

	watcher := storeopen.OpenWatcher(cfg, slog.Default())
	defer func() { _ = watcher.Close() }()

	sup := supervisor.New(cfg.ShutdownGrace)
	loop := reconcile.New(cfg.NodeID, st.Nodes, st.Runtimes, st.Bots, sup)
	loop.ReconcileInterval = cfg.ReconcileInterval
	loop.HeartbeatInterval = cfg.HeartbeatInterval
	loop.PublicURL = cfg.PublicURL
	loop.BotRunnerCommand = cfg.BotRunnerCommand
	loop.BotRunnerWorkdir = cfg.BotRunnerWorkdir
	loop.BotRunnerHealthPort = cfg.BotRunnerHealthPort
	loop.StoreKind = storeKind
	loop.MemoryStorePath = cfg.MemoryStorePath
	loop.Lease = lease.New(cfg.NodeID, cfg.LeaseTTL, st.Runtimes)
	loop.Restart = reconcile.RestartPolicy{
		MaxAttempts: cfg.RestartMaxAttempts,
		Base:        cfg.RestartBackoffBase,
		Max:         cfg.RestartBackoffMax,
	}
	loop.MaxBotsPerNode = cfg.MaxBotsPerNode
	loop.Watcher = watcher

	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	slog.Info("agent запущен: heartbeat + reconcile (+ store watcher)")
	runErr := loop.Run(runCtx)
	if runErr != nil && runCtx.Err() == nil {
		fmt.Fprintf(os.Stderr, "agent: reconcile: %v\n", runErr)
		os.Exit(1)
	}

	// Shutdown: остановить управляемые процессы с grace из конфига.
	shutCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownGrace+5*time.Second)
	defer cancel()
	if err := loop.Shutdown(shutCtx); err != nil {
		slog.Warn("shutdown", "err", err)
	}
	slog.Info("agent остановлен")
}
