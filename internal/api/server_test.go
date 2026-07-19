package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mvp-manager/internal/api"
	"mvp-manager/internal/config"
	"mvp-manager/internal/ops"
	"mvp-manager/internal/store"
	"mvp-manager/internal/store/memory"
)

func TestHealthzAndAuth(t *testing.T) {
	st := memory.New()
	cfg := config.Config{
		NodeID:          "node-api",
		ControlAPIToken: "secret",
		APIAddr:         "127.0.0.1:0",
	}
	srv := api.New(cfg, ops.Repos{
		Nodes: st.Nodes, Runtimes: st.Runtimes, Bots: st.Bots, Events: st.Events,
	})
	h := srv.Handler()

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("healthz: %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/bots", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("без токена: want 401, got %d", rr.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/bots", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("с токеном: want 200, got %d body=%s", rr.Code, rr.Body.String())
	}
}

// TestEntityJSONSnakeCase проверяет канонический snake_case в ответах сущностей
// (UI-0a / docs/frontend.md §5.5): bot_type, last_seen_at, а не PascalCase.
func TestEntityJSONSnakeCase(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	cfg := config.Config{
		NodeID:          "node-api",
		ControlAPIToken: "secret",
		APIAddr:         "127.0.0.1:0",
	}
	srv := api.New(cfg, ops.Repos{
		Nodes: st.Nodes, Runtimes: st.Runtimes, Bots: st.Bots, Events: st.Events,
	})
	h := srv.Handler()

	_, err := st.Nodes.Upsert(ctx, store.Node{
		ID: "node-1", Hostname: "host-1", Status: store.NodeStatusOnline,
	})
	if err != nil {
		t.Fatalf("Upsert node: %v", err)
	}
	rt, err := st.Runtimes.Create(ctx, store.Runtime{
		Kind: store.RuntimeKindBotRunner, Name: "runner-node-1",
		StartCommand: "bot-runner", DesiredState: store.DesiredStopped, ActualState: store.ActualUnknown,
	})
	if err != nil {
		t.Fatalf("Create runtime: %v", err)
	}
	nodeID := "node-1"
	bot, err := st.Bots.Create(ctx, store.Bot{
		Name: "b1", BotType: store.BotTypeDefault, Channel: store.BotChannelTelegram,
		RunMode: store.BotRunModeWebhook, Port: 18001, TokenRef: "tok-secret",
		RuntimeID: &rt.ID, AssignedNodeID: &nodeID,
		DesiredState: store.DesiredStopped, ActualState: store.ActualUnknown,
	})
	if err != nil {
		t.Fatalf("Create bot: %v", err)
	}
	if _, err := st.Events.Append(ctx, store.BotEvent{
		BotID: bot.ID, Type: "created", Message: "bot created", At: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Append event: %v", err)
	}

	authGet := func(path string) string {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer secret")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s: want 200, got %d body=%s", path, rr.Code, rr.Body.String())
		}
		return rr.Body.String()
	}

	nodesBody := authGet("/v1/nodes")
	assertSnakeKeys(t, nodesBody, []string{"last_seen_at", "agent_version", "created_at"}, []string{"LastSeenAt", "AgentVersion", "CreatedAt"})

	botsBody := authGet("/v1/bots")
	assertSnakeKeys(t, botsBody, []string{"bot_type", "token_ref", "assigned_node_id", "desired_state"}, []string{"BotType", "TokenRef", "AssignedNodeID", "DesiredState"})

	runtimesBody := authGet("/v1/runtimes")
	assertSnakeKeys(t, runtimesBody, []string{"start_command", "desired_state", "assigned_node_id", "lease_until"}, []string{"StartCommand", "DesiredState", "AssignedNodeID", "LeaseUntil"})

	eventsBody := authGet("/v1/bots/" + bot.ID + "/events")
	assertSnakeKeys(t, eventsBody, []string{"bot_id", "type", "message", "at"}, []string{"BotID", "Type", "Message", "At"})
}

// TestCreateBotResponseSnakeCase — POST принимает snake_case и отдаёт snake_case.
func TestCreateBotResponseSnakeCase(t *testing.T) {
	st := memory.New()
	cfg := config.Config{
		NodeID:           "node-api",
		ControlAPIToken:  "secret",
		APIAddr:          "127.0.0.1:0",
		BotRunnerCommand: "bot-runner",
	}
	srv := api.New(cfg, ops.Repos{
		Nodes: st.Nodes, Runtimes: st.Runtimes, Bots: st.Bots, Events: st.Events,
	})
	h := srv.Handler()

	body := `{
		"name": "client-42",
		"bot_type": "default",
		"channel": "telegram",
		"run_mode": "webhook",
		"port": 18042,
		"token_ref": "secret:bot-42",
		"assigned_node_id": "node-api",
		"desired_state": "stopped"
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/bots", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d body=%s", rr.Code, rr.Body.String())
	}
	assertSnakeKeys(t, rr.Body.String(), []string{"bot_type", "token_ref", "assigned_node_id"}, []string{"BotType", "TokenRef"})

	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["bot_type"] != "default" {
		t.Fatalf("bot_type: got %#v", got["bot_type"])
	}
}

// assertSnakeKeys проверяет наличие канонических ключей и отсутствие PascalCase.
func assertSnakeKeys(t *testing.T, body string, want []string, forbid []string) {
	t.Helper()
	for _, k := range want {
		if !strings.Contains(body, `"`+k+`"`) {
			t.Fatalf("ожидали ключ %q в JSON, body=%s", k, body)
		}
	}
	for _, k := range forbid {
		if strings.Contains(body, `"`+k+`"`) {
			t.Fatalf("неожиданный PascalCase ключ %q в JSON, body=%s", k, body)
		}
	}
}
