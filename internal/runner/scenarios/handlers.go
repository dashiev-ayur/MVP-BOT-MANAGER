package scenarios

import (
	"context"
	"fmt"
	"strings"

	"mvp-manager/internal/messenger"
)

// Default — сценарий bot_type=default: ответ на /start фиксированным текстом.
type Default struct{}

// Handle делегирует messenger.HandleDefaultStart (тот же текст, что в Phase 2).
func (Default) Handle(ctx context.Context, ch messenger.Channel, in messenger.Incoming) (bool, error) {
	return messenger.HandleDefaultStart(ctx, ch, in)
}

// ExtendedStartReply — текст /start для default_extended (отличим от default).
const ExtendedStartReply = "Привет! Сценарий default_extended (mvp-manager) готов."

// ExtendedPingReply — ответ на /ping (доп. команда, отличающая extended).
const ExtendedPingReply = "pong (default_extended)"

// Extended — сценарий bot_type=default_extended:
//   - /start → другой текст, чем у default;
//   - /ping → короткое подтверждение.
type Extended struct{}

// Handle обрабатывает /start и /ping.
func (Extended) Handle(ctx context.Context, ch messenger.Channel, in messenger.Incoming) (bool, error) {
	if isStart(in) {
		if err := ch.SendText(ctx, in.ChatID, ExtendedStartReply); err != nil {
			return true, fmt.Errorf("extended /start chat %d: %w", in.ChatID, err)
		}
		return true, nil
	}
	if isPing(in) {
		if err := ch.SendText(ctx, in.ChatID, ExtendedPingReply); err != nil {
			return true, fmt.Errorf("extended /ping chat %d: %w", in.ChatID, err)
		}
		return true, nil
	}
	return false, nil
}

func isStart(in messenger.Incoming) bool {
	if in.IsStart {
		return true
	}
	cmd := commandName(in.Text)
	return cmd == "/start"
}

func isPing(in messenger.Incoming) bool {
	return commandName(in.Text) == "/ping"
}

// commandName извлекает "/cmd" из "/cmd@Bot args".
func commandName(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	cmd, _, _ := strings.Cut(text, " ")
	cmd, _, _ = strings.Cut(cmd, "@")
	return cmd
}
