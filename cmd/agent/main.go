// Точка входа демона agent: на каждой ноде следит за desired/actual
// состоянием ботов и управляет процессами (Phase 1+).
//
// Phase 0.4: Load конфига → wiring STORE=memory → регистрация ноды →
// ожидание SIGINT/SIGTERM и тихий выход. Без reconcile loop.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"mvp-manager/internal/config"
	"mvp-manager/internal/store"
	"mvp-manager/internal/storeopen"
)

// version — строка версии бинарника; позже можно подставлять через -ldflags.
const version = "0.0.0-dev"

// helpText — краткая справка для пользователя CLI (без бизнес-логики).
const helpText = `mvp-manager agent — демон управления ботами на ноде

Использование:
  agent [-h|--help]
  agent [-v|--version]
  agent

При запуске без флагов читает конфиг из ENV (NODE_ID, STORE; по умолчанию
STORE=memory), создаёт store, регистрирует ноду и ждёт SIGINT/SIGTERM.
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

	st, storeKind, err := storeopen.Open(cfg.Store)
	if err != nil {
		fmt.Fprintf(os.Stderr, "agent: store: %v\n", err)
		os.Exit(1)
	}

	slog.Info("конфиг загружен",
		"node_id", cfg.NodeID,
		"store", storeKind,
	)

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

	// Ждём SIGINT/SIGTERM: context отменяется → тихий выход без reconcile (Phase 1).
	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	slog.Info("agent запущен, ожидание сигнала завершения")
	<-runCtx.Done()
	// Тихий выход: без лишнего Fatal/паники; defer stop снимет обработчики сигналов.
}
