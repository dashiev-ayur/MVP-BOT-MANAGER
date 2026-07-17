package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Имена переменных окружения, из которых Load читает конфигурацию.
const (
	EnvNodeID            = "NODE_ID"
	EnvStore             = "STORE"
	EnvDatabaseURL       = "DATABASE_URL" // зарезервировано под Phase PG; сейчас не обязательно
	EnvMemoryStorePath   = "MEMORY_STORE_PATH"
	EnvReconcileInterval = "RECONCILE_INTERVAL"
	EnvHeartbeatInterval = "HEARTBEAT_INTERVAL"
	EnvShutdownGrace     = "SHUTDOWN_GRACE"
	EnvPublicURL         = "PUBLIC_URL" // опционально, прокидывается в launch contract custom-бота
)

// Допустимые значения STORE и значение по умолчанию.
const (
	StoreMemory   = "memory"
	StorePostgres = "postgres"
	DefaultStore  = StoreMemory

	// DefaultMemoryStorePath — общий файл для agent и ctl при STORE=memory.
	// Без общего файла процессы не видят записи друг друга (критично для E2E Phase 1).
	DefaultMemoryStorePath = ".mvp-manager/store.json"

	DefaultReconcileInterval = 3 * time.Second
	DefaultHeartbeatInterval = 5 * time.Second
	DefaultShutdownGrace     = 10 * time.Second
)

// Config — снимок конфигурации процесса после чтения ENV.
//
// Поля DatabaseURL пока только читаются (для будущего STORE=postgres);
// подключение к БД на Phase 1 не выполняется.
type Config struct {
	// NodeID — идентификатор ноды агента (обязателен, непустой).
	NodeID string
	// Store — тип хранилища: memory (сейчас) или postgres (Phase PG).
	Store string
	// DatabaseURL — DSN PostgreSQL; используется только при STORE=postgres (позже).
	DatabaseURL string

	// MemoryStorePath — JSON-файл общего состояния для STORE=memory.
	// Пустая строка допустима только если явно задать MEMORY_STORE_PATH=""
	// (чистый in-process store без диска; agent и ctl тогда изолированы).
	// При незаданной переменной — DefaultMemoryStorePath.
	MemoryStorePath string

	// ReconcileInterval — период сверки desired↔actual в agent.
	ReconcileInterval time.Duration
	// HeartbeatInterval — период обновления last_seen_at ноды.
	HeartbeatInterval time.Duration
	// ShutdownGrace — сколько ждать SIGTERM дочерним процессам перед SIGKILL.
	ShutdownGrace time.Duration

	// PublicURL — опциональный PUBLIC_URL для launch contract (webhook).
	PublicURL string
}

// Load читает конфигурацию из переменных окружения процесса.
//
// Правила:
//   - NODE_ID обязателен (после TrimSpace не должен быть пустым);
//   - STORE по умолчанию — memory, если переменная не задана или пустая;
//   - неизвестный STORE → ошибка с перечислением допустимых значений (не паника);
//   - postgres принимается как известное значение конфига, но wiring/БД — не здесь;
//   - MEMORY_STORE_PATH: если переменная не задана — DefaultMemoryStorePath;
//     если задана пустой строкой — persistence отключена (только RAM);
//   - интервалы парсятся как time.ParseDuration ("3s", "500ms"); пусто → дефолты.
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

	memoryPath, err := memoryStorePathFromEnv()
	if err != nil {
		return Config{}, err
	}

	reconcileInterval, err := durationFromEnv(EnvReconcileInterval, DefaultReconcileInterval)
	if err != nil {
		return Config{}, err
	}
	heartbeatInterval, err := durationFromEnv(EnvHeartbeatInterval, DefaultHeartbeatInterval)
	if err != nil {
		return Config{}, err
	}
	shutdownGrace, err := durationFromEnv(EnvShutdownGrace, DefaultShutdownGrace)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		NodeID:            nodeID,
		Store:             store,
		DatabaseURL:       strings.TrimSpace(os.Getenv(EnvDatabaseURL)),
		MemoryStorePath:   memoryPath,
		ReconcileInterval: reconcileInterval,
		HeartbeatInterval: heartbeatInterval,
		ShutdownGrace:     shutdownGrace,
		PublicURL:         strings.TrimSpace(os.Getenv(EnvPublicURL)),
	}
	return cfg, nil
}

// memoryStorePathFromEnv различает «переменная не задана» и «задана пустой».
// Не задана → дефолтный файл (agent и ctl делят состояние).
// Задана "" → без файла (изолированный RAM store, удобно для юнит-тестов wiring).
func memoryStorePathFromEnv() (string, error) {
	v, ok := os.LookupEnv(EnvMemoryStorePath)
	if !ok {
		return DefaultMemoryStorePath, nil
	}
	// Явно пустой путь — ок; иначе TrimSpace (пробелы не считаем путём).
	return strings.TrimSpace(v), nil
}

// durationFromEnv читает duration из ENV; пустая строка → def.
func durationFromEnv(key string, def time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s=%q: %w", key, raw, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s=%q: duration must be > 0", key, raw)
	}
	return d, nil
}
