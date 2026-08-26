package messenger_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"mvp-manager/internal/messenger"
	"mvp-manager/internal/store"
)

// TestTelegramStartWebhook — входящий /start (mock HTTP) → sendMessage с приветствием.
func TestTelegramStartWebhook(t *testing.T) {
	var (
		mu       sync.Mutex
		sentBody string
		sentPath string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		sentPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		sentBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	t.Cleanup(srv.Close)

	tg := messenger.NewTelegram("TESTTOKEN", srv.Client(), srv.URL)
	rec := httptest.NewRecorder()
	body := `{"update_id":1,"message":{"text":"/start","chat":{"id":42}}}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	tg.ServeWebhook(rec, req, func(ctx context.Context, in messenger.Incoming) error {
		_, err := messenger.HandleDefaultStart(ctx, tg, in)
		return err
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}

	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(sentPath, "/botTESTTOKEN/sendMessage") {
		t.Fatalf("unexpected path %q", sentPath)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(sentBody), &payload); err != nil {
		t.Fatal(err)
	}
	if int64(payload["chat_id"].(float64)) != 42 {
		t.Fatalf("chat_id=%v", payload["chat_id"])
	}
	if payload["text"] != messenger.DefaultStartReply {
		t.Fatalf("text=%v", payload["text"])
	}
}

// TestTelegramStartPolling — deleteWebhook → getUpdates с /start → sendMessage.
func TestTelegramStartPolling(t *testing.T) {
	var (
		mu              sync.Mutex
		getUpdatesCalls int
		deletedWebhook  bool
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "deleteWebhook"):
			mu.Lock()
			deletedWebhook = true
			mu.Unlock()
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
		case strings.Contains(r.URL.Path, "getUpdates"):
			mu.Lock()
			getUpdatesCalls++
			n := getUpdatesCalls
			mu.Unlock()
			if n == 1 {
				_, _ = w.Write([]byte(`{"ok":true,"result":[{"update_id":7,"message":{"text":"/start","chat":{"id":99}}}]}`))
				return
			}
			// Второй запрос — короткий пустой ответ до отмены.
			_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
		case strings.Contains(r.URL.Path, "sendMessage"):
			_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	tg := messenger.NewTelegram("TOK", srv.Client(), srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- tg.RunPolling(ctx, func(ctx context.Context, in messenger.Incoming) error {
			handled, err := messenger.HandleDefaultStart(ctx, tg, in)
			if handled {
				cancel()
			}
			return err
		})
	}()

	err := <-done
	if err != nil && !errorsIsContextCanceled(err) {
		t.Fatalf("polling: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !deletedWebhook {
		t.Fatal("deleteWebhook not called before getUpdates")
	}
	if getUpdatesCalls < 1 {
		t.Fatal("getUpdates not called")
	}
}

func errorsIsContextCanceled(err error) bool {
	return err != nil && (err == context.Canceled || strings.Contains(err.Error(), "context canceled"))
}

// TestMaxBotStarted — Max bot_started → SendText (mock HTTP).
func TestMaxBotStarted(t *testing.T) {
	var (
		mu       sync.Mutex
		sentAuth string
		sentPath string
		sentBody string
		sentQuery string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		sentAuth = r.Header.Get("Authorization")
		sentPath = r.URL.Path
		sentQuery = r.URL.RawQuery
		b, _ := io.ReadAll(r.Body)
		sentBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":{}}`))
	}))
	t.Cleanup(srv.Close)

	mx := messenger.NewMax("max-access-token", srv.Client(), srv.URL)
	rec := httptest.NewRecorder()
	body := `{"update_type":"bot_started","chat_id":555}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	mx.ServeWebhook(rec, req, func(ctx context.Context, in messenger.Incoming) error {
		_, err := messenger.HandleDefaultStart(ctx, mx, in)
		return err
	})
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}

	mu.Lock()
	defer mu.Unlock()
	if sentAuth != "max-access-token" {
		t.Fatalf("Authorization=%q", sentAuth)
	}
	if sentPath != "/messages" {
		t.Fatalf("path=%q", sentPath)
	}
	if !strings.Contains(sentQuery, "chat_id=555") {
		t.Fatalf("query=%q", sentQuery)
	}
	if !strings.Contains(sentBody, messenger.DefaultStartReply) {
		t.Fatalf("body=%q", sentBody)
	}
}

// TestMaxMessageCreatedStart — message_created с текстом /start.
func TestMaxMessageCreatedStart(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	mx := messenger.NewMax("t", srv.Client(), srv.URL)
	handled, err := messenger.HandleDefaultStart(context.Background(), mx, messenger.Incoming{
		ChatID: 1,
		Text:   "/start",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("expected handled")
	}
}

func TestNewChannel_Both(t *testing.T) {
	t.Parallel()
	tg, err := messenger.NewChannel(store.BotChannelTelegram, "t", nil, "")
	if err != nil || tg == nil {
		t.Fatalf("telegram: %v", err)
	}
	mx, err := messenger.NewChannel(store.BotChannelMax, "t", nil, "")
	if err != nil || mx == nil {
		t.Fatalf("max: %v", err)
	}
	_, err = messenger.NewChannel(store.BotChannel("irc"), "t", nil, "")
	if err == nil {
		t.Fatal("expected error for unknown channel")
	}
}

func TestHandleDefaultStart_IgnoresOther(t *testing.T) {
	t.Parallel()
	// nil channel не вызовется — handled=false.
	handled, err := messenger.HandleDefaultStart(context.Background(), nil, messenger.Incoming{
		ChatID: 1,
		Text:   "hello",
	})
	if err != nil || handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
}
