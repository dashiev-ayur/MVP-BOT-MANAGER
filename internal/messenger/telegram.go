package messenger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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
		// Без общего Client.Timeout: иначе long poll getUpdates легко
		// упирается в суммарный лимит (dial+TLS+ожидание body). Дедлайны —
		// на каждый запрос через context (см. getUpdates / SendText).
		httpClient = &http.Client{}
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
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, t.apiURL("sendMessage"), bytes.NewReader(body))
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
//
// Перед первым getUpdates снимаем webhook (deleteWebhook + drop_pending_updates):
// иначе Telegram отвечает Conflict или отдаёт только старый webhook-фильтр.
func (t *Telegram) RunPolling(ctx context.Context, handle func(context.Context, Incoming) error) error {
	if err := t.deleteWebhook(ctx); err != nil {
		slog.Warn("telegram deleteWebhook failed", "err", err)
	} else {
		slog.Info("telegram webhook cleared (polling mode)")
	}

	var offset int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		// 10s long poll — чаще видно «getUpdates ok» в логе при ручной проверке.
		updates, err := t.getUpdates(ctx, offset, 10)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			slog.Warn("telegram getUpdates failed; retry", "err", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
				continue
			}
		}
		for _, u := range updates {
			if in, ok := telegramIncoming(u); ok {
				slog.Info("telegram incoming", "chat_id", in.ChatID, "text", in.Text)
				if err := handle(ctx, in); err != nil {
					slog.Warn("telegram update handler failed",
						"chat_id", in.ChatID,
						"err", err,
					)
				}
			}
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
			}
		}
	}
}

// deleteWebhook снимает исходящий webhook и сбрасывает очередь pending updates.
func (t *Telegram) deleteWebhook(ctx context.Context) error {
	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	u := t.apiURL("deleteWebhook") + "?drop_pending_updates=true"
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram deleteWebhook: %w", err)
	}
	defer resp.Body.Close()
	return decodeTelegramOK(resp)
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
	// Дедлайн запроса = long-poll timeout + запас на сеть (не Client.Timeout).
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec+15)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, t.apiURL("getUpdates")+"?"+q.Encode(), nil)
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
