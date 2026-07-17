// Точка входа демона agent: на каждой ноде следит за desired/actual
// состоянием ботов и управляет процессами (Phase 1+).
//
// Phase 0.2: при обычном запуске загружает конфиг из ENV и пишет
// NODE_ID/STORE в лог (без wiring store). Help/version без изменений по смыслу.
package main

import (
	"fmt"
	"log/slog"
	"os"

	"mvp-manager/internal/config"
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
STORE=memory) и пишет значения в лог. Memory store и reconcile появятся позже.
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

	// Обычный запуск: демонстрируем чтение конфига (Phase 0.2).
	// Store не создаём — wiring memory/postgres в блоках 0.4 / Phase PG.
	cfg, err := config.Load()
	if err != nil {
		// Пользовательское сообщение на stderr; код выхода 1 — не паника.
		fmt.Fprintf(os.Stderr, "agent: конфиг: %v\n", err)
		os.Exit(1)
	}

	slog.Info("конфиг загружен",
		"node_id", cfg.NodeID,
		"store", cfg.Store,
	)
}
