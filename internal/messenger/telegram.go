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

// Дефолтный endpoint Telegram Bot API (официальная документация core.telegram.org/bots/api).
const defaultTelegramAPI = "https://api.telegram.org"

// Telegram — тонкий клиент Bot API через net/http (без стороннего SDK).
type Telegram struct {
	token  string
	client *http.Client
	base   string // например https://api.telegram.org или httptest URL в тестах
}

// NewTelegram создаёт клиент. httpClient/baseURL могут быть nil/"" (дефолты).
func NewTelegram(token string, httpClient *http.Client, baseURL string) *Telegram {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 45 * time.Second}
	}
	if baseURL == "" {
		baseURL = defaultTelegramAPI
	}
	return &Telegram{token: token, client: httpClient, base: stringsTrimRightSlash(baseURL)}
}

func (t *Telegram) apiURL(method string) string {
	return t.base + "/bot" + t.token + "/" + method
}

// SendText — метод sendMessage.
func (t *Telegram) SendText(ctx context.Context, chatID int64, text string) error {
	body, err := json.Marshal(map[string]any{
		"chat_id": chatID,
		"text":    text,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.apiURL("sendMessage"), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram sendMessage: %w", err)
	}
	defer resp.Body.Close()
	return decodeTelegramOK(resp)
}

// RunPolling — long poll getUpdates до отмены ctx.
func (t *Telegram) RunPolling(ctx context.Context, handle func(context.Context, Incoming) error) error {
	var offset int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		updates, err := t.getUpdates(ctx, offset, 25)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// Сетевой сбой — короткая пауза и повтор (не валим инстанс).
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
				continue
			}
		}
		for _, u := range updates {
			if in, ok := telegramIncoming(u); ok {
				if err := handle(ctx, in); err != nil {
					// Ошибка обработки одного апдейта не останавливает polling.
					_ = err
				}
			}
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
			}
		}
	}
}

// ServeWebhook принимает Telegram Update JSON (POST). Без setWebhook к Telegram:
// полный исходящий webhook требует HTTPS PUBLIC_URL — здесь только приём на порту бота.
func (t *Telegram) ServeWebhook(w http.ResponseWriter, r *http.Request, handle func(context.Context, Incoming) error) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()
	var u telegramUpdate
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&u); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if in, ok := telegramIncoming(u); ok {
		if err := handle(r.Context(), in); err != nil {
			http.Error(w, "handler error", http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (t *Telegram) getUpdates(ctx context.Context, offset int64, timeoutSec int) ([]telegramUpdate, error) {
	q := url.Values{}
	q.Set("timeout", strconv.Itoa(timeoutSec))
	if offset > 0 {
		q.Set("offset", strconv.FormatInt(offset, 10))
	}
	// Long poll: HTTP-таймаут клиента должен быть > timeoutSec.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.apiURL("getUpdates")+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("telegram getUpdates: %w", err)
	}
	defer resp.Body.Close()
	var out struct {
		OK          bool             `json:"ok"`
		Result      []telegramUpdate `json:"result"`
		Description string           `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("telegram getUpdates decode: %w", err)
	}
	if resp.StatusCode >= 300 || !out.OK {
		return nil, fmt.Errorf("telegram getUpdates status=%d: %s", resp.StatusCode, out.Description)
	}
	return out.Result, nil
}

type telegramUpdate struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		Text string `json:"text"`
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	} `json:"message"`
}

func telegramIncoming(u telegramUpdate) (Incoming, bool) {
	if u.Message == nil || u.Message.Chat.ID == 0 {
		return Incoming{}, false
	}
	return Incoming{
		ChatID: u.Message.Chat.ID,
		Text:   u.Message.Text,
	}, true
}

func decodeTelegramOK(resp *http.Response) error {
	var out struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = json.Unmarshal(body, &out)
	if resp.StatusCode >= 300 || !out.OK {
		msg := out.Description
		if msg == "" {
			msg = string(body)
		}
		return fmt.Errorf("telegram api status=%d: %s", resp.StatusCode, msg)
	}
	return nil
}

func stringsTrimRightSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
