package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"mvp-manager/internal/api"
	"mvp-manager/internal/config"
	"mvp-manager/internal/ops"
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
