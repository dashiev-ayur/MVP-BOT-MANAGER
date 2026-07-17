package config

import (
	"fmt"
	"os"
	"strings"
)

// Имена переменных окружения, из которых Load читает конфигурацию.
const (
	EnvNodeID      = "NODE_ID"
	EnvStore       = "STORE"
	EnvDatabaseURL = "DATABASE_URL" // зарезервировано под Phase PG; сейчас не обязательно
)

// Допустимые значения STORE и значение по умолчанию.
const (
	StoreMemory   = "memory"
	StorePostgres = "postgres"
	DefaultStore  = StoreMemory
)

// Config — снимок конфигурации процесса после чтения ENV.
//
// Поля DatabaseURL пока только читаются (для будущего STORE=postgres);
// на этапе Phase 0.2 подключение к БД не выполняется.
type Config struct {
	// NodeID — идентификатор ноды агента (обязателен, непустой).
	NodeID string
	// Store — тип хранилища: memory (сейчас) или postgres (Phase PG).
	Store string
	// DatabaseURL — DSN PostgreSQL; используется только при STORE=postgres (позже).
	DatabaseURL string
}

// Load читает конфигурацию из переменных окружения процесса.
//
// Правила:
//   - NODE_ID обязателен (после TrimSpace не должен быть пустым);
//   - STORE по умолчанию — memory, если переменная не задана или пустая;
//   - неизвестный STORE → ошибка с перечислением допустимых значений (не паника);
//   - postgres принимается как известное значение конфига, но wiring/БД — не здесь.
//
// Файл .env библиотекой не загружается: переменные задаёт окружение
// (export, set -a; source .env и т.п.) — см. .env.example и README.
func Load() (Config, error) {
	nodeID := strings.TrimSpace(os.Getenv(EnvNodeID))
	if nodeID == "" {
		return Config{}, fmt.Errorf(
			"%s обязателен: задайте непустой идентификатор ноды в переменной окружения %s",
			EnvNodeID, EnvNodeID,
		)
	}

	store := strings.TrimSpace(os.Getenv(EnvStore))
	if store == "" {
		store = DefaultStore
	}

	// Явная проверка допустимых значений: неизвестный бэкенд — понятная ошибка,
	// а не «тихий» fallback и не паника.
	switch store {
	case StoreMemory, StorePostgres:
		// ok
	default:
		return Config{}, fmt.Errorf(
			"неизвестный %s=%q: допустимы %q и %q (%s — позже, Phase PG)",
			EnvStore, store, StoreMemory, StorePostgres, StorePostgres,
		)
	}

	cfg := Config{
		NodeID:      nodeID,
		Store:       store,
		DatabaseURL: strings.TrimSpace(os.Getenv(EnvDatabaseURL)),
	}
	return cfg, nil
}
