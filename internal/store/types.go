package store

import "time"

// Идентификаторы:
//   - Node.ID — произвольный стабильный TEXT (часто hostname / NODE_ID из env);
//   - Runtime.ID, Bot.ID, Bot.ClientID, Bot.RuntimeID — UUID в виде строки
//     (канонический текстовый вид RFC 4122). Выбран string, а не отдельный
//     UUID-тип: нулевых внешних зависимостей и единообразие с Node.ID.
// Реализации могут генерировать UUID при Create, если поле пустое.

// ---------------------------------------------------------------------------
// Enum-подобные типы (значения совпадают с CHECK/ENUM в ТЗ §6)
// ---------------------------------------------------------------------------

// NodeStatus — статус ноды в кластере (ТЗ §6.1).
type NodeStatus string

const (
	NodeStatusOnline   NodeStatus = "online"
	NodeStatusOffline  NodeStatus = "offline"
	NodeStatusDraining NodeStatus = "draining"
)

// RuntimeKind — тип OS-процесса, которым управляет агент (ТЗ §6.2).
type RuntimeKind string

const (
	RuntimeKindBotRunner RuntimeKind = "bot_runner"
	RuntimeKindCustomBot RuntimeKind = "custom_bot"
)

// DesiredState — желаемое состояние runtime/bot (ТЗ §6.2 / §6.3).
type DesiredState string

const (
	DesiredRunning DesiredState = "running"
	DesiredStopped DesiredState = "stopped"
)

// ActualState — фактическое состояние runtime/bot (ТЗ §6.2 / §6.3).
type ActualState string

const (
	ActualUnknown   ActualState = "unknown"
	ActualStarting  ActualState = "starting"
	ActualRunning   ActualState = "running"
	ActualStopping  ActualState = "stopping"
	ActualStopped   ActualState = "stopped"
	ActualFailed    ActualState = "failed"
	ActualMigrating ActualState = "migrating"
)

// BotType — логический тип бота (ТЗ §6.3).
type BotType string

const (
	BotTypeCustom          BotType = "custom"
	BotTypeDefault         BotType = "default"
	BotTypeDefaultExtended BotType = "default_extended"
)

// BotChannel — мессенджер-канал бота (ТЗ §6.3).
type BotChannel string

const (
	BotChannelTelegram BotChannel = "telegram"
	BotChannelMax      BotChannel = "max"
)

// BotRunMode — режим доставки апдейтов (ТЗ §6.3).
// webhook — процесс биндит port; polling — port только зарезервирован в store.
type BotRunMode string

const (
	BotRunModeWebhook BotRunMode = "webhook"
	BotRunModePolling BotRunMode = "polling"
)

// ---------------------------------------------------------------------------
// Доменные сущности
// ---------------------------------------------------------------------------

// Node — агент на машине (таблица nodes, ТЗ §6.1).
type Node struct {
	ID           string
	Hostname     string
	Status       NodeStatus
	LastSeenAt   time.Time
	AgentVersion *string        // опционально
	Meta         map[string]any // JSON-объект; nil трактуем как пустой
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Runtime — OS-процесс под управлением агента (таблица runtimes, ТЗ §6.2).
// kind=bot_runner — multi-tenant runner; kind=custom_bot — dedicated процесс.
type Runtime struct {
	ID           string
	Kind         RuntimeKind
	Name         string // UNIQUE во всём store
	StartCommand string
	Workdir      *string
	Env          map[string]any // JSON-объект env; nil = пустой

	DesiredState DesiredState
	ActualState  ActualState

	AssignedNodeID *string
	LeaseOwner     *string
	LeaseUntil     *time.Time

	PID           *int
	ExitCode      *int
	LastError     *string
	ConfigVersion int64

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Bot — логический бот клиента (таблица bots, ТЗ §6.3).
//
// Инварианты (проверяют реализации при Create/Update):
//   - Port уникален глобально;
//   - при BotTypeCustom CustomName обязателен и непустой;
//   - при остальных типах CustomName должен быть nil.
type Bot struct {
	ID       string
	ClientID *string // опциональная связь с будущей таблицей clients

	Name       string
	BotType    BotType
	CustomName *string
	Channel    BotChannel
	RunMode    BotRunMode

	Port     int
	TokenRef string // ссылка/секрет; в MVP допустим сам токен

	// RuntimeID — привязка к runner (default*) или dedicated runtime (custom).
	// nil, если бот stopped / ещё не назначен.
	RuntimeID *string

	ArtifactPath *string
	RepoURL      *string
	StartCommand *string // override; иначе дефолт контракта репо

	DesiredState   DesiredState
	ActualState    ActualState
	AssignedNodeID *string

	LastError      *string
	ConfigVersion  int64
	ScenarioConfig map[string]any // параметры вшитого сценария; nil = пустой

	CreatedAt time.Time
	UpdatedAt time.Time
}

// ---------------------------------------------------------------------------
// Частичные обновления для reconcile (без полного Update сущности)
// ---------------------------------------------------------------------------

// RuntimeActualPatch — снимок actual-полей runtime после start/stop/crash.
//
// ActualState обязателен. PID / ExitCode / LastError: значение указателя
// записывается как есть (nil указатель = NULL в store). Так supervisor
// может и выставить pid, и явно очистить его после остановки процесса.
type RuntimeActualPatch struct {
	ActualState ActualState
	PID         *int
	ExitCode    *int
	LastError   *string
}

// BotActualPatch — снимок actual-полей логического бота.
// LastError: nil указатель = очистить ошибку в store.
type BotActualPatch struct {
	ActualState ActualState
	LastError   *string
}
