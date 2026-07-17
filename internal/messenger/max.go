package messenger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Дефолтный endpoint Max Bot API (dev.max.ru/docs-api, platform-api2.max.ru).
const defaultMaxAPI = "https://platform-api2.max.ru"

// Max — тонкий HTTP-клиент к Max Bot API.
//
// Авторизация: заголовок Authorization: <access_token> (query-токен устарел).
// Live-проверке нужен реальный токен бота на https://dev.max.ru.
type Max struct {
	token  string
	client *http.Client
	base   string
}

// NewMax создаёт клиент. httpClient/baseURL могут быть nil/"" (дефолты).
func NewMax(token string, httpClient *http.Client, baseURL string) *Max {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 100 * time.Second}
	}
	if baseURL == "" {
		baseURL = defaultMaxAPI
	}
	return &Max{token: token, client: httpClient, base: stringsTrimRightSlash(baseURL)}
}

// SendText — POST /messages?chat_id=… с телом {"text":…}.
func (m *Max) SendText(ctx context.Context, chatID int64, text string) error {
	q := url.Values{}
	q.Set("chat_id", strconv.FormatInt(chatID, 10))
	body, err := json.Marshal(map[string]any{"text": text})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.base+"/messages?"+q.Encode(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", m.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("max sendMessage: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return fmt.Errorf("max sendMessage status=%d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// RunPolling — GET /updates (long poll) до отмены ctx.
func (m *Max) RunPolling(ctx context.Context, handle func(context.Context, Incoming) error) error {
	var marker *int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		updates, next, err := m.getUpdates(ctx, marker, 30)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
				continue
			}
		}
		for _, u := range updates {
			if in, ok := maxIncoming(u); ok {
				_ = handle(ctx, in)
			}
		}
		marker = next
	}
}

// ServeWebhook принимает Max Update JSON (как в webhook/subscriptions).
func (m *Max) ServeWebhook(w http.ResponseWriter, r *http.Request, handle func(context.Context, Incoming) error) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()
	var u maxUpdate
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&u); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if in, ok := maxIncoming(u); ok {
		if err := handle(r.Context(), in); err != nil {
			http.Error(w, "handler error", http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (m *Max) getUpdates(ctx context.Context, marker *int64, timeoutSec int) ([]maxUpdate, *int64, error) {
	q := url.Values{}
	q.Set("timeout", strconv.Itoa(timeoutSec))
	q.Set("limit", "100")
	if marker != nil {
		q.Set("marker", strconv.FormatInt(*marker, 10))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.base+"/updates?"+q.Encode(), nil)
	if err != nil {
		return nil, marker, err
	}
	req.Header.Set("Authorization", m.token)
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, marker, fmt.Errorf("max getUpdates: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, marker, err
	}
	if resp.StatusCode >= 300 {
		return nil, marker, fmt.Errorf("max getUpdates status=%d: %s", resp.StatusCode, string(body))
	}
	var out struct {
		Updates []maxUpdate `json:"updates"`
		Marker  *int64      `json:"marker"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, marker, fmt.Errorf("max getUpdates decode: %w", err)
	}
	next := out.Marker
	if next == nil {
		next = marker
	}
	return out.Updates, next, nil
}

// maxUpdate — минимальные поля Update Max (message_created / bot_started).
type maxUpdate struct {
	UpdateType string `json:"update_type"`
	ChatID     int64  `json:"chat_id"`
	Message    *struct {
		Body *struct {
			Text string `json:"text"`
		} `json:"body"`
		Recipient *struct {
			ChatID int64 `json:"chat_id"`
			UserID int64 `json:"user_id"`
		} `json:"recipient"`
	} `json:"message"`
}

func maxIncoming(u maxUpdate) (Incoming, bool) {
	switch u.UpdateType {
	case "bot_started":
		// Эквивалент Telegram /start: пользователь начал диалог с ботом.
		if u.ChatID == 0 {
			return Incoming{}, false
		}
		return Incoming{ChatID: u.ChatID, IsStart: true}, true
	case "message_created":
		chatID := u.ChatID
		text := ""
		if u.Message != nil {
			if u.Message.Body != nil {
				text = u.Message.Body.Text
			}
			if chatID == 0 && u.Message.Recipient != nil && u.Message.Recipient.ChatID != 0 {
				chatID = u.Message.Recipient.ChatID
			}
		}
		if chatID == 0 {
			return Incoming{}, false
		}
		return Incoming{ChatID: chatID, Text: text}, true
	default:
		return Incoming{}, false
	}
}
