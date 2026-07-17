// Точка входа CLI ctl: обёртка над операциями store (создание ботов,
// смена desired state и т.п.). Не смешивается с демоном agent.
//
// Phase 0.4: при обычном запуске Load + wiring STORE=memory (подтверждение
// в логе). Подкоманды bots* — Phase 1.
package main

import (
	"fmt"
	"log/slog"
	"os"

	"mvp-manager/internal/config"
	"mvp-manager/internal/storeopen"
)

// version — строка версии бинарника; позже можно подставлять через -ldflags.
const version = "0.0.0-dev"

// helpText — краткая справка для пользователя CLI (без бизнес-логики).
const helpText = `mvp-manager ctl — CLI управления ботами (обёртка над store)

Использование:
  ctl [-h|--help]
  ctl [-v|--version]
  ctl

При запуске без флагов читает конфиг (NODE_ID, STORE) и создаёт store.
Подкоманды bots create/start/stop появятся в Phase 1.
Хранилище на ранних этапах: STORE=memory.
`

func main() {
	// Help / version без NODE_ID — как у agent.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-v", "--version", "version":
			fmt.Printf("ctl %s\n", version)
			return
		case "-h", "--help", "help":
			fmt.Print(helpText)
			return
		}
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ctl: конфиг: %v\n", err)
		os.Exit(1)
	}

	_, storeKind, err := storeopen.Open(cfg.Store)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ctl: store: %v\n", err)
		os.Exit(1)
	}

	slog.Info("ctl: store готов",
		"node_id", cfg.NodeID,
		"store", storeKind,
	)
}
