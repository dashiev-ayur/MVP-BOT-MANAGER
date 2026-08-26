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

	// CORS: UI (Vite) ходит с Origin → нужны Allow-Origin и preflight.
	reqCORS := httptest.NewRequest(http.MethodOptions, "/v1/bots", nil)
	reqCORS.Header.Set("Origin", "http://localhost:5173")
	reqCORS.Header.Set("Access-Control-Request-Method", "GET")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, reqCORS)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS: want 204, got %d", rr.Code)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("CORS Allow-Origin: got %q", got)
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

func TestCreateBotWithClientID(t *testing.T) {
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

	const clientID = "11111111-1111-4111-8111-111111111111"
	body := `{
		"name": "with-client",
		"bot_type": "default",
		"channel": "telegram",
		"run_mode": "webhook",
		"port": 18043,
		"token_ref": "secret:bot-43",
		"client_id": "` + clientID + `",
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
	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["client_id"] != clientID {
		t.Fatalf("client_id: got %#v", got["client_id"])
	}

	bad := httptest.NewRequest(http.MethodPost, "/v1/bots", bytes.NewBufferString(`{
		"name": "bad-client",
		"bot_type": "default",
		"channel": "telegram",
		"run_mode": "webhook",
		"port": 18044,
		"token_ref": "secret:bot-44",
		"client_id": "not-a-uuid",
		"desired_state": "stopped"
	}`))
	bad.Header.Set("Authorization", "Bearer secret")
	bad.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, bad)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid client_id: want 400, got %d body=%s", rr.Code, rr.Body.String())
	}

	// PATCH: смена client_id.
	id, _ := got["id"].(string)
	patch := httptest.NewRequest(http.MethodPatch, "/v1/bots/"+id, bytes.NewBufferString(`{
		"client_id": "22222222-2222-4222-8222-222222222222"
	}`))
	patch.Header.Set("Authorization", "Bearer secret")
	patch.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, patch)
	if rr.Code != http.StatusOK {
		t.Fatalf("patch: want 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var patched map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &patched); err != nil {
		t.Fatalf("patch decode: %v", err)
	}
	if patched["client_id"] != "22222222-2222-4222-8222-222222222222" {
		t.Fatalf("patched client_id: got %#v", patched["client_id"])
	}
}

func TestListBotsFilterByClientID(t *testing.T) {
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

	clientA := "11111111-1111-4111-8111-111111111111"
	clientB := "22222222-2222-4222-8222-222222222222"
	if _, err := st.Bots.Create(ctx, store.Bot{
		ClientID: &clientA, Name: "a1", BotType: store.BotTypeDefault,
		Channel: store.BotChannelTelegram, RunMode: store.BotRunModePolling,
		Port: 19101, TokenRef: "tok-a",
		DesiredState: store.DesiredStopped, ActualState: store.ActualUnknown,
	}); err != nil {
		t.Fatalf("create a1: %v", err)
	}
	if _, err := st.Bots.Create(ctx, store.Bot{
		ClientID: &clientB, Name: "b1", BotType: store.BotTypeDefault,
		Channel: store.BotChannelTelegram, RunMode: store.BotRunModePolling,
		Port: 19102, TokenRef: "tok-b",
		DesiredState: store.DesiredStopped, ActualState: store.ActualUnknown,
	}); err != nil {
		t.Fatalf("create b1: %v", err)
	}
	if _, err := st.Bots.Create(ctx, store.Bot{
		Name: "orphan", BotType: store.BotTypeDefault,
		Channel: store.BotChannelTelegram, RunMode: store.BotRunModePolling,
		Port: 19103, TokenRef: "tok-o",
		DesiredState: store.DesiredStopped, ActualState: store.ActualUnknown,
	}); err != nil {
		t.Fatalf("create orphan: %v", err)
	}

	authGet := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer secret")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr
	}

	decodeNames := func(body []byte) []string {
		t.Helper()
		var list []map[string]any
		if err := json.Unmarshal(body, &list); err != nil {
			t.Fatalf("decode: %v body=%s", err, body)
		}
		names := make([]string, 0, len(list))
		for _, b := range list {
			name, _ := b["name"].(string)
			names = append(names, name)
		}
		return names
	}

	all := authGet("/v1/bots")
	if all.Code != http.StatusOK {
		t.Fatalf("all: want 200, got %d body=%s", all.Code, all.Body.String())
	}
	if got := decodeNames(all.Body.Bytes()); len(got) != 3 {
		t.Fatalf("all: want 3 bots, got %v", got)
	}

	filtered := authGet("/v1/bots?client_id=" + clientA)
	if filtered.Code != http.StatusOK {
		t.Fatalf("filter: want 200, got %d body=%s", filtered.Code, filtered.Body.String())
	}
	if got := decodeNames(filtered.Body.Bytes()); len(got) != 1 || got[0] != "a1" {
		t.Fatalf("filter client A: got %v", got)
	}

	unknown := authGet("/v1/bots?client_id=33333333-3333-4333-8333-333333333333")
	if unknown.Code != http.StatusOK {
		t.Fatalf("unknown: want 200, got %d body=%s", unknown.Code, unknown.Body.String())
	}
	if got := decodeNames(unknown.Body.Bytes()); len(got) != 0 {
		t.Fatalf("unknown: want empty, got %v", got)
	}

	bad := authGet("/v1/bots?client_id=not-a-uuid")
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid uuid: want 400, got %d body=%s", bad.Code, bad.Body.String())
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
