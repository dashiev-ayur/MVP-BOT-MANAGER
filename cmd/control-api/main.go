// Точка входа HTTP control-api: CRUD / start / stop / migrate над тем же store,
// что agent и ctl (ТЗ §11). Не смешивается с демоном agent.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mvp-manager/internal/api"
	"mvp-manager/internal/config"
	"mvp-manager/internal/ops"
	"mvp-manager/internal/storeopen"
)

const version = "0.0.0-dev"

const helpText = `mvp-manager control-api — HTTP API управления ботами (ТЗ §11)

Использование:
  control-api [-h|--help]
  control-api [-v|--version]
  control-api

ENV: NODE_ID (для create default assignment), STORE, MEMORY_STORE_PATH,
API_ADDR (дефолт 127.0.0.1:8080), CONTROL_API_TOKEN (обязателен для /v1/*),
BOT_RUNNER_COMMAND (для create/migrate default*).
`

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-v", "--version", "version":
			fmt.Printf("control-api %s\n", version)
			return
		case "-h", "--help", "help":
			fmt.Print(helpText)
			return
		}
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "control-api: конфиг: %v\n", err)
		os.Exit(1)
	}
	if cfg.ControlAPIToken == "" {
		slog.Warn("CONTROL_API_TOKEN пуст: все /v1/* вернут 401")
	}

	st, storeKind, err := storeopen.Open(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "control-api: store: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = st.Close() }()

	srvAPI := api.New(cfg, ops.Repos{
		Nodes:    st.Nodes,
		Runtimes: st.Runtimes,
		Bots:     st.Bots,
		Events:   st.Events,
	})

	httpSrv := &http.Server{
		Addr:              cfg.APIAddr,
		Handler:           srvAPI.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	slog.Info("control-api слушает",
		"addr", cfg.APIAddr,
		"store", storeKind,
		"memory_store_path", cfg.MemoryStorePath,
		"node_id", cfg.NodeID,
	)

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpSrv.ListenAndServe()
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutCtx)
		slog.Info("control-api остановлен")
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "control-api: %v\n", err)
			os.Exit(1)
		}
	}
}
