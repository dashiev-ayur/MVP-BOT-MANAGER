package config

import (
	"fmt"
	"os"
	"strconv"
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

	// Bot-runner (агент стартует процесс; runner читает тот же STORE).
	EnvBotRunnerCommand    = "BOT_RUNNER_COMMAND"
	EnvBotRunnerWorkdir    = "BOT_RUNNER_WORKDIR"
	EnvBotRunnerHealthPort = "BOT_RUNNER_HEALTH_PORT" // служебный /healthz самого runner (опционально)
	EnvRuntimeID           = "RUNTIME_ID"             // какой runtime обслуживает этот процесс runner

	// Healthcheck (отдельный cmd; интервал независим от reconcile).
	EnvCheckInterval     = "CHECK_INTERVAL"
	EnvHTTPTimeout       = "HTTP_TIMEOUT"
	EnvFailureThreshold  = "FAILURE_THRESHOLD"
	EnvHealthcheckAllNodes = "HEALTHCHECK_ALL_NODES" // true → опрос всех нод, иначе только NODE_ID
)

// Допустимые значения STORE и значение по умолчанию.
const (
	StoreMemory   = "memory"
	StorePostgres = "postgres"
	DefaultStore  = StoreMemory

	// DefaultMemoryStorePath — общий файл для agent, ctl, bot-runner, healthcheck.
	// Без общего файла процессы не видят записи друг друга (критично для E2E).
	DefaultMemoryStorePath = ".mvp-manager/store.json"

	DefaultReconcileInterval = 3 * time.Second
	DefaultHeartbeatInterval = 5 * time.Second
	DefaultShutdownGrace     = 10 * time.Second

	DefaultCheckInterval    = 10 * time.Second
	DefaultHTTPTimeout      = 2 * time.Second
	DefaultFailureThreshold = 3
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

	// BotRunnerCommand — команда запуска multi-tenant bot-runner (ТЗ §10.1).
	// Пустая строка: агент не сможет стартовать runner (ошибка в reconcile).
	BotRunnerCommand string
	// BotRunnerWorkdir — опциональный workdir процесса runner.
	BotRunnerWorkdir string
	// BotRunnerHealthPort — опциональный порт служебного /healthz runner’а (строка для ENV).
	BotRunnerHealthPort string

	// RuntimeID — идентификатор runtime для процесса bot-runner (ENV RUNTIME_ID).
	// У agent/ctl обычно пуст; у runner обязателен (или выводится из store по имени).
	RuntimeID string

	// CheckInterval / HTTPTimeout / FailureThreshold — конфиг cmd/healthcheck.
	CheckInterval    time.Duration
	HTTPTimeout      time.Duration
	FailureThreshold int
	// HealthcheckAllNodes — опрашивать webhook-ботов всех нод (по умолчанию только NODE_ID).
	HealthcheckAllNodes bool
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

	checkInterval, err := durationFromEnv(EnvCheckInterval, DefaultCheckInterval)
	if err != nil {
		return Config{}, err
	}
	httpTimeout, err := durationFromEnv(EnvHTTPTimeout, DefaultHTTPTimeout)
	if err != nil {
		return Config{}, err
	}
	failureThreshold, err := intFromEnv(EnvFailureThreshold, DefaultFailureThreshold)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		NodeID:              nodeID,
		Store:               store,
		DatabaseURL:         strings.TrimSpace(os.Getenv(EnvDatabaseURL)),
		MemoryStorePath:     memoryPath,
		ReconcileInterval:   reconcileInterval,
		HeartbeatInterval:   heartbeatInterval,
		ShutdownGrace:       shutdownGrace,
		PublicURL:           strings.TrimSpace(os.Getenv(EnvPublicURL)),
		BotRunnerCommand:    strings.TrimSpace(os.Getenv(EnvBotRunnerCommand)),
		BotRunnerWorkdir:    strings.TrimSpace(os.Getenv(EnvBotRunnerWorkdir)),
		BotRunnerHealthPort: strings.TrimSpace(os.Getenv(EnvBotRunnerHealthPort)),
		RuntimeID:           strings.TrimSpace(os.Getenv(EnvRuntimeID)),
		CheckInterval:       checkInterval,
		HTTPTimeout:         httpTimeout,
		FailureThreshold:    failureThreshold,
		HealthcheckAllNodes: boolFromEnv(EnvHealthcheckAllNodes),
	}
	return cfg, nil
}

// intFromEnv читает целое > 0; пустая строка → def.
func intFromEnv(key string, def int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s=%q: %w", key, raw, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("%s=%q: must be > 0", key, raw)
	}
	return n, nil
}

// boolFromEnv — true для "1", "true", "yes", "on" (без учёта регистра).
func boolFromEnv(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
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
