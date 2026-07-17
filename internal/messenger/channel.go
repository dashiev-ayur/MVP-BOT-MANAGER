// Package messenger — адаптеры каналов Telegram / Max для сценария default.
//
// Тонкие HTTP-клиенты к официальным Bot API (без тяжёлых SDK):
//   - Telegram: https://api.telegram.org/bot<token>/… (Bot API getUpdates / sendMessage);
//   - Max:      https://platform-api2.max.ru (Authorization + /updates /messages).
//
// Сценарий default отвечает на /start (и Max bot_started) коротким приветствием.
package messenger

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"mvp-manager/internal/store"
)

// DefaultStartReply — текст ответа на /start (и эквивалент Max bot_started).
const DefaultStartReply = "Привет! Сценарий default (mvp-manager) готов."

// Incoming — нормализованное входящее событие от канала.
type Incoming struct {
	// ChatID — куда слать ответ (Telegram chat.id / Max chat_id).
	ChatID int64
	// Text — текст сообщения; для Max bot_started может быть пустым.
	Text string
	// IsStart — явное «старт диалога» (Max bot_started или Telegram /start).
	IsStart bool
}

// Channel — контракт канала мессенджера для сценария default.
//
// Реализации: Telegram, Max. HTTPClient и BaseURL подменяются в тестах (httptest).
type Channel interface {
	// SendText отправляет текстовое сообщение в чат.
	SendText(ctx context.Context, chatID int64, text string) error
	// RunPolling — long poll до отмены ctx; на каждое Incoming вызывает handle.
	RunPolling(ctx context.Context, handle func(context.Context, Incoming) error) error
	// ServeWebhook принимает один update (POST JSON) и при необходимости вызывает handle.
	// Telegram: тело = Update; Max: тело = Update (как в webhook/subscriptions).
	ServeWebhook(w http.ResponseWriter, r *http.Request, handle func(context.Context, Incoming) error)
}

// NewChannel выбирает адаптер по bots.channel.
//
// httpClient / baseURL опциональны (nil / "" → дефолты продакшена).
func NewChannel(ch store.BotChannel, token string, httpClient *http.Client, baseURL string) (Channel, error) {
	if token == "" {
		return nil, fmt.Errorf("empty messenger token")
	}
	switch ch {
	case store.BotChannelTelegram:
		return NewTelegram(token, httpClient, baseURL), nil
	case store.BotChannelMax:
		return NewMax(token, httpClient, baseURL), nil
	default:
		return nil, fmt.Errorf("unsupported channel %q", ch)
	}
}

// HandleDefaultStart — ядро сценария default: ответить на /start (или IsStart).
//
// Возвращает handled=true, если ответ отправлен (или команда распознана).
func HandleDefaultStart(ctx context.Context, ch Channel, in Incoming) (handled bool, err error) {
	if !isStartCommand(in) {
		return false, nil
	}
	if err := ch.SendText(ctx, in.ChatID, DefaultStartReply); err != nil {
		return true, fmt.Errorf("reply /start to chat %d: %w", in.ChatID, err)
	}
	return true, nil
}

// isStartCommand — /start (с опциональным @botname) или флаг IsStart (Max bot_started).
func isStartCommand(in Incoming) bool {
	if in.IsStart {
		return true
	}
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return false
	}
	// Telegram: "/start" или "/start@MyBot".
	cmd, _, _ := strings.Cut(text, " ")
	cmd, _, _ = strings.Cut(cmd, "@")
	return cmd == "/start"
}
