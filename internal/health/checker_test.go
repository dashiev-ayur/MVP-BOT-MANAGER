package health_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mvp-manager/internal/health"
	"mvp-manager/internal/launch"
	"mvp-manager/internal/store"
	"mvp-manager/internal/store/memory"
)

// TestTickMarksUnhealthy — серия фейлов → actual=failed с префиксом healthcheck:.
func TestTickMarksUnhealthy(t *testing.T) {
	// Сервер всегда 503.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	// Достаём порт из URL тестового сервера.
	port := portFromURL(t, srv.URL)

	ctx := context.Background()
	st := memory.New()
	nodeID := "node-hc"
	nodeRef := nodeID
	rtID := "rt-hc"
	_, _ = st.Runtimes.Create(ctx, store.Runtime{
		ID: rtID, Kind: store.RuntimeKindBotRunner, Name: "br",
		StartCommand: "x", DesiredState: store.DesiredRunning, AssignedNodeID: &nodeRef,
	})
	b, err := st.Bots.Create(ctx, store.Bot{
		Name: "w", BotType: store.BotTypeDefault,
		Channel: store.BotChannelTelegram, RunMode: store.BotRunModeWebhook,
		Port: port, TokenRef: "t", RuntimeID: &rtID,
		DesiredState: store.DesiredRunning, ActualState: store.ActualRunning,
		AssignedNodeID: &nodeRef,
	})
	if err != nil {
		t.Fatal(err)
	}

	chk := health.New(nodeID, st.Bots)
	chk.FailureThreshold = 2
	chk.HTTPTimeout = time.Second
	// Подменяем клиент на тот, что ходит на httptest (порт уже в bot.Port).
	// checker ходит на 127.0.0.1:port — httptest слушает тот же порт. OK.

	_ = chk.Tick(ctx)
	_ = chk.Tick(ctx)

	b2, _ := st.Bots.ByID(ctx, b.ID)
	if b2.ActualState != store.ActualFailed {
		t.Fatalf("want failed, got %s", b2.ActualState)
	}
	if b2.LastError == nil || !strings.HasPrefix(*b2.LastError, launch.HealthcheckErrPrefix) {
		t.Fatalf("want last_error prefix %q, got %v", launch.HealthcheckErrPrefix, b2.LastError)
	}
}

// TestTickIgnoresPolling — polling не опрашивается.
func TestTickIgnoresPolling(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	nodeID := "node-p"
	nodeRef := nodeID
	b, err := st.Bots.Create(ctx, store.Bot{
		Name: "p", BotType: store.BotTypeDefault,
		Channel: store.BotChannelTelegram, RunMode: store.BotRunModePolling,
		Port: 19999, TokenRef: "t",
		DesiredState: store.DesiredRunning, ActualState: store.ActualRunning,
		AssignedNodeID: &nodeRef,
	})
	if err != nil {
		t.Fatal(err)
	}

	chk := health.New(nodeID, st.Bots)
	chk.FailureThreshold = 1
	if err := chk.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	b2, _ := st.Bots.ByID(ctx, b.ID)
	if b2.ActualState != store.ActualRunning {
		t.Fatalf("polling не должен меняться healthcheck’ом, got %s", b2.ActualState)
	}
}

func portFromURL(t *testing.T, raw string) int {
	t.Helper()
	// http://127.0.0.1:PORT
	i := strings.LastIndex(raw, ":")
	if i < 0 {
		t.Fatalf("bad url %s", raw)
	}
	var port int
	for _, c := range raw[i+1:] {
		if c < '0' || c > '9' {
			break
		}
		port = port*10 + int(c-'0')
	}
	if port == 0 {
		t.Fatalf("no port in %s", raw)
	}
	return port
}
