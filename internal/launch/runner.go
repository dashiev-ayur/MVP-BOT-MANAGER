package launch

import (
	"fmt"
	"os"
	"strconv"

	"mvp-manager/internal/config"
	"mvp-manager/internal/store"
)

// BotRunnerRuntimeName — каноническое UNIQUE name runtime bot_runner на ноде (MVP: один на ноду).
func BotRunnerRuntimeName(nodeID string) string {
	return "bot-runner-" + nodeID
}

// IsDefaultType — вшитый multi-tenant тип (не custom).
func IsDefaultType(t store.BotType) bool {
	return t == store.BotTypeDefault || t == store.BotTypeDefaultExtended
}

// ValidateDefaultCreate проверяет поля ctl bots create для bot_type=default*.
func ValidateDefaultCreate(name string, port int, tokenRef string) error {
	if name == "" {
		return fmt.Errorf("name required: %w", store.ErrInvalidArgument)
	}
	if port <= 0 || port > 65535 {
		return fmt.Errorf("port must be 1..65535: %w", store.ErrInvalidArgument)
	}
	if tokenRef == "" {
		return fmt.Errorf("token required: %w", store.ErrInvalidArgument)
	}
	return nil
}

// BuildRunnerEnv собирает окружение OS-процесса bot-runner.
//
// Критично: тот же STORE / MEMORY_STORE_PATH, что у agent, иначе runner
// не увидит ботов. RUNTIME_ID говорит runner’у, какой runtime он обслуживает.
func BuildRunnerEnv(nodeID, runtimeID, storeKind, memoryPath, healthPort string) []string {
	env := os.Environ()
	env = append(env,
		config.EnvNodeID+"="+nodeID,
		config.EnvRuntimeID+"="+runtimeID,
		config.EnvStore+"="+storeKind,
	)
	// Явно прокидываем путь: даже если agent стартовал с дефолтом без export в Environ.
	if memoryPath != "" {
		env = append(env, config.EnvMemoryStorePath+"="+memoryPath)
	} else {
		// Пустой путь = RAM-only; всё равно выставляем "", чтобы дочерний
		// процесс не подхватил чужой дефолт из оболочки.
		env = append(env, config.EnvMemoryStorePath+"=")
	}
	if healthPort != "" {
		env = append(env, config.EnvBotRunnerHealthPort+"="+healthPort)
	}
	return env
}

// HealthzURL — URL для опроса webhook-бота на localhost.
func HealthzURL(port int) string {
	return "http://127.0.0.1:" + strconv.Itoa(port) + "/healthz"
}

// HealthcheckErrPrefix — префикс bots.last_error от cmd/healthcheck.
// Agent по нему отличает unhealthy webhook от прочих failed и рестартует runner.
const HealthcheckErrPrefix = "healthcheck:"
