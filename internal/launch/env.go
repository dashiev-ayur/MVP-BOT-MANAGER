// Package launch — тонкие хелперы launch contract (ТЗ §9) без дублирования моделей store.
package launch

import (
	"fmt"
	"os"
	"strconv"

	"mvp-manager/internal/store"
)

// Имена ENV launch contract (ТЗ §9).
const (
	EnvPort      = "PORT"
	EnvBotToken  = "BOT_TOKEN"
	EnvBotMode   = "BOT_MODE"
	EnvChannel   = "CHANNEL"
	EnvPublicURL = "PUBLIC_URL"
)

// BuildEnv собирает полное окружение для Start custom-бота:
// os.Environ() + обязательные PORT/BOT_TOKEN/BOT_MODE/CHANNEL и опциональный PUBLIC_URL.
//
// BOT_TOKEN резолвится из bot.TokenRef (см. ResolveTokenRef).
func BuildEnv(bot store.Bot, publicURL string) ([]string, error) {
	token, err := ResolveTokenRef(bot.TokenRef)
	if err != nil {
		return nil, fmt.Errorf("resolve token_ref: %w", err)
	}
	env := os.Environ()
	env = append(env,
		EnvPort+"="+strconv.Itoa(bot.Port),
		EnvBotToken+"="+token,
		EnvBotMode+"="+string(bot.RunMode),
		EnvChannel+"="+string(bot.Channel),
	)
	if publicURL != "" {
		env = append(env, EnvPublicURL+"="+publicURL)
	}
	return env, nil
}

// ValidateCustomCreate проверяет минимальные поля для ctl bots create (custom).
// Не создаёт сущности — только валидация аргументов CLI/API.
func ValidateCustomCreate(name, customName, startCommand string, port int, tokenRef string) error {
	if name == "" {
		return fmt.Errorf("name required: %w", store.ErrInvalidArgument)
	}
	if customName == "" {
		return fmt.Errorf("custom_name required: %w", store.ErrInvalidArgument)
	}
	if startCommand == "" {
		return fmt.Errorf("start_command required: %w", store.ErrInvalidArgument)
	}
	if port <= 0 || port > 65535 {
		return fmt.Errorf("port must be 1..65535: %w", store.ErrInvalidArgument)
	}
	if tokenRef == "" {
		return fmt.Errorf("token required: %w", store.ErrInvalidArgument)
	}
	return nil
}

// WorkdirFromBot выбирает workdir для supervisor: artifact_path бота, иначе nil.
func WorkdirFromBot(bot store.Bot) string {
	if bot.ArtifactPath != nil {
		return *bot.ArtifactPath
	}
	return ""
}

// StartCommand выбирает команду: runtime.StartCommand, иначе bot.StartCommand.
func StartCommand(rt store.Runtime, bot store.Bot) string {
	if rt.StartCommand != "" {
		return rt.StartCommand
	}
	if bot.StartCommand != nil {
		return *bot.StartCommand
	}
	return ""
}
