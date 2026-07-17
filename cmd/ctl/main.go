// Точка входа CLI ctl: обёртка над операциями store (создание ботов,
// смена desired state и т.п.). Не смешивается с демоном agent.
//
// Сейчас — заглушка Phase 0.1: печатает version/help и завершается.
// Реальные подкоманды появятся в следующих фазах.
package main

import (
	"fmt"
	"os"
)

// version — строка версии бинарника; позже можно подставлять через -ldflags.
const version = "0.0.0-dev"

// helpText — краткая справка для пользователя CLI (без бизнес-логики).
const helpText = `mvp-manager ctl — CLI управления ботами (обёртка над store)

Использование:
  ctl [-h|--help]
  ctl [-v|--version]

Пока это заглушка каркаса (Phase 0.1). Подкоманды появятся позже.
Хранилище на ранних этапах: STORE=memory.
`

func main() {
	// Минимальный разбор флагов: help / version; остальное — тоже help,
	// чтобы запуск без аргументов и с неизвестными флагами не паниковал.
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

	// Запуск без аргументов: показываем help (ожидаемое поведение заглушки).
	fmt.Print(helpText)
}
