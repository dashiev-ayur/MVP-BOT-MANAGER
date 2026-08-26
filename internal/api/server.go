// Package api — HTTP handlers для cmd/control-api (ТЗ §11).
//
// Auth: Bearer token из конфига. Без токена / с неверным — 401.
// Не импортирует pgx: только store-интерфейсы и ops.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"mvp-manager/internal/config"
	"mvp-manager/internal/launch"
	"mvp-manager/internal/metrics"
	"mvp-manager/internal/ops"
	"mvp-manager/internal/store"
)

// Server — зависимости HTTP API.
type Server struct {
	Token string
	Cfg   config.Config
	Repos ops.Repos
	Log   *slog.Logger
}

// New создаёт Server.
func New(cfg config.Config, repos ops.Repos) *Server {
	return &Server{
		Token: cfg.ControlAPIToken,
		Cfg:   cfg,
		Repos: repos,
		Log:   slog.Default(),
	}
}

// Handler возвращает корневой mux со всеми маршрутами §11.
// Обёртка CORS: UI на другом origin (Vite :5173 → API :8080) иначе
// браузер блокирует fetch, и логин показывает «API недоступен».
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /metrics", s.handleMetrics)
	mux.HandleFunc("GET /v1/nodes", s.auth(s.handleListNodes))
	mux.HandleFunc("GET /v1/bots", s.auth(s.handleListBots))
	mux.HandleFunc("POST /v1/bots", s.auth(s.handleCreateBot))
	mux.HandleFunc("PATCH /v1/bots/{id}", s.auth(s.handlePatchBot))
	mux.HandleFunc("POST /v1/bots/{id}/start", s.auth(s.handleStartBot))
	mux.HandleFunc("POST /v1/bots/{id}/stop", s.auth(s.handleStopBot))
	mux.HandleFunc("POST /v1/bots/{id}/migrate", s.auth(s.handleMigrateBot))
	mux.HandleFunc("GET /v1/runtimes", s.auth(s.handleListRuntimes))
	mux.HandleFunc("GET /v1/bots/{id}/events", s.auth(s.handleListEvents))
	return withCORS(mux)
}

// withCORS отражает Origin запроса (админка + Bearer). Preflight OPTIONS — 204.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.Token == "" {
			writeErr(w, http.StatusUnauthorized, "CONTROL_API_TOKEN не задан")
			return
		}
		h := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(h, prefix) || strings.TrimSpace(strings.TrimPrefix(h, prefix)) != s.Token {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r)
	}
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

// handleMetrics — простой text exposition счётчиков (без Prometheus scrape-обязательности).
func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(metrics.Default.Text()))
}

func (s *Server) handleListNodes(w http.ResponseWriter, r *http.Request) {
	list, err := s.Repos.Nodes.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Ответ в snake_case через DTO — store-типы без json-тегов не трогаем.
	writeJSON(w, http.StatusOK, toNodeDTOs(list))
}

func (s *Server) handleListBots(w http.ResponseWriter, r *http.Request) {
	clientID := strings.TrimSpace(r.URL.Query().Get("client_id"))

	var list []store.Bot
	var err error
	if clientID == "" {
		list, err = s.Repos.Bots.List(r.Context())
	} else {
		// client_id в схеме — UUID; иначе postgres даст 500 на ::uuid.
		if !validUUID(clientID) {
			writeErr(w, http.StatusBadRequest, "client_id должен быть UUID")
			return
		}
		list, err = s.Repos.Bots.ListByClientID(r.Context(), clientID)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toBotDTOs(maskBots(list)))
}

func (s *Server) handleListRuntimes(w http.ResponseWriter, r *http.Request) {
	list, err := s.Repos.Runtimes.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toRuntimeDTOs(list))
}

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.Repos.Events == nil {
		writeJSON(w, http.StatusOK, []botEventDTO{})
		return
	}
	list, err := s.Repos.Events.ListByBot(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toBotEventDTOs(list))
}

func (s *Server) handleStartBot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := ops.StartWithLimit(r.Context(), s.Repos, id, s.Cfg.MaxBotsPerNode); err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "desired": "running", "bot_id": id})
}

func (s *Server) handleStopBot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := ops.Stop(r.Context(), s.Repos, id); err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "desired": "stopped", "bot_id": id})
}

type migrateBody struct {
	ToNodeID string `json:"to_node_id"`
}

func (s *Server) handleMigrateBot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body migrateBody
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.ToNodeID == "" {
		writeErr(w, http.StatusBadRequest, "to_node_id обязателен")
		return
	}
	opt := ops.MigrateOptsFromConfig(s.Cfg, body.ToNodeID)
	if err := ops.Migrate(r.Context(), s.Repos, id, opt); err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok", "bot_id": id, "to_node_id": body.ToNodeID,
	})
}

type createBotBody struct {
	Name           string             `json:"name"`
	BotType        store.BotType      `json:"bot_type"`
	CustomName     *string            `json:"custom_name"`
	Channel        store.BotChannel   `json:"channel"`
	RunMode        store.BotRunMode   `json:"run_mode"`
	Port           int                `json:"port"`
	TokenRef       string             `json:"token_ref"`
	ClientID       *string            `json:"client_id"`
	AssignedNodeID *string            `json:"assigned_node_id"`
	DesiredState   store.DesiredState `json:"desired_state"`
	ArtifactPath   *string            `json:"artifact_path"`
	StartCommand   *string            `json:"start_command"`
	Workdir        *string            `json:"workdir"`
}

func (s *Server) handleCreateBot(w http.ResponseWriter, r *http.Request) {
	var body createBotBody
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Channel == "" {
		body.Channel = store.BotChannelTelegram
	}
	if body.RunMode == "" {
		body.RunMode = store.BotRunModeWebhook
	}
	clientID, err := parseOptionalClientID(body.ClientID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	body.ClientID = clientID
	nodeID := s.Cfg.NodeID
	if body.AssignedNodeID != nil && *body.AssignedNodeID != "" {
		nodeID = *body.AssignedNodeID
	}

	ctx := r.Context()
	var bot store.Bot

	switch body.BotType {
	case store.BotTypeCustom:
		bot, err = s.createCustom(ctx, body, nodeID)
	case store.BotTypeDefault, store.BotTypeDefaultExtended:
		bot, err = s.createDefault(ctx, body, nodeID)
	default:
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("неизвестный bot_type %q", body.BotType))
		return
	}
	if err != nil {
		writeStoreErr(w, err)
		return
	}

	if body.DesiredState == store.DesiredRunning {
		if err := ops.StartWithLimit(ctx, s.Repos, bot.ID, s.Cfg.MaxBotsPerNode); err != nil {
			writeStoreErr(w, err)
			return
		}
		bot, _ = s.Repos.Bots.ByID(ctx, bot.ID)
	}
	writeJSON(w, http.StatusCreated, toBotDTO(maskBot(bot)))
}

func (s *Server) createCustom(ctx context.Context, body createBotBody, nodeID string) (store.Bot, error) {
	if err := ops.CheckBotLimit(ctx, s.Repos.Bots, nodeID, s.Cfg.MaxBotsPerNode, true); err != nil {
		return store.Bot{}, err
	}
	cn := ""
	if body.CustomName != nil {
		cn = *body.CustomName
	}
	startCmd := ""
	if body.StartCommand != nil {
		startCmd = *body.StartCommand
	}
	if err := launch.ValidateCustomCreate(body.Name, cn, startCmd, body.Port, body.TokenRef); err != nil {
		return store.Bot{}, err
	}
	var wd *string
	if body.Workdir != nil {
		wd = body.Workdir
	} else if body.ArtifactPath != nil {
		wd = body.ArtifactPath
	}
	rt, err := s.Repos.Runtimes.Create(ctx, store.Runtime{
		Kind:           store.RuntimeKindCustomBot,
		Name:           "custom-" + cn,
		StartCommand:   startCmd,
		Workdir:        wd,
		DesiredState:   store.DesiredStopped,
		ActualState:    store.ActualUnknown,
		AssignedNodeID: &nodeID,
	})
	if err != nil {
		return store.Bot{}, err
	}
	sc := startCmd
	bot, err := s.Repos.Bots.Create(ctx, store.Bot{
		ClientID:       body.ClientID,
		Name:           body.Name,
		BotType:        store.BotTypeCustom,
		CustomName:     &cn,
		Channel:        body.Channel,
		RunMode:        body.RunMode,
		Port:           body.Port,
		TokenRef:       body.TokenRef,
		RuntimeID:      &rt.ID,
		ArtifactPath:   body.ArtifactPath,
		StartCommand:   &sc,
		DesiredState:   store.DesiredStopped,
		ActualState:    store.ActualUnknown,
		AssignedNodeID: &nodeID,
	})
	if err != nil {
		return store.Bot{}, err
	}
	launch.WarnIfPlaintextTokenRef(s.Log, bot.ID, body.TokenRef)
	return bot, nil
}

func (s *Server) createDefault(ctx context.Context, body createBotBody, nodeID string) (store.Bot, error) {
	if err := ops.CheckBotLimit(ctx, s.Repos.Bots, nodeID, s.Cfg.MaxBotsPerNode, true); err != nil {
		return store.Bot{}, err
	}
	if err := launch.ValidateDefaultCreate(body.Name, body.Port, body.TokenRef); err != nil {
		return store.Bot{}, err
	}
	name := launch.BotRunnerRuntimeName(nodeID)
	rt, err := s.Repos.Runtimes.ByName(ctx, name)
	if errors.Is(err, store.ErrNotFound) {
		cmd := s.Cfg.BotRunnerCommand
		if cmd == "" {
			cmd = "bot-runner"
		}
		var wd *string
		if s.Cfg.BotRunnerWorkdir != "" {
			w := s.Cfg.BotRunnerWorkdir
			wd = &w
		}
		rt, err = s.Repos.Runtimes.Create(ctx, store.Runtime{
			Kind:           store.RuntimeKindBotRunner,
			Name:           name,
			StartCommand:   cmd,
			Workdir:        wd,
			DesiredState:   store.DesiredStopped,
			ActualState:    store.ActualUnknown,
			AssignedNodeID: &nodeID,
		})
	}
	if err != nil {
		return store.Bot{}, err
	}
	bot, err := s.Repos.Bots.Create(ctx, store.Bot{
		ClientID:       body.ClientID,
		Name:           body.Name,
		BotType:        body.BotType,
		Channel:        body.Channel,
		RunMode:        body.RunMode,
		Port:           body.Port,
		TokenRef:       body.TokenRef,
		RuntimeID:      &rt.ID,
		DesiredState:   store.DesiredStopped,
		ActualState:    store.ActualUnknown,
		AssignedNodeID: &nodeID,
	})
	if err != nil {
		return store.Bot{}, err
	}
	launch.WarnIfPlaintextTokenRef(s.Log, bot.ID, body.TokenRef)
	return bot, nil
}

type patchBotBody struct {
	DesiredState   *store.DesiredState `json:"desired_state"`
	TokenRef       *string             `json:"token_ref"`
	ClientID       *string             `json:"client_id"`
	AssignedNodeID *string             `json:"assigned_node_id"`
	ConfigVersion  *int64              `json:"config_version"`
	ScenarioConfig map[string]any      `json:"scenario_config"`
}

func (s *Server) handlePatchBot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body patchBotBody
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	bot, err := s.Repos.Bots.ByID(r.Context(), id)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if body.DesiredState != nil {
		bot.DesiredState = *body.DesiredState
	}
	if body.TokenRef != nil {
		bot.TokenRef = *body.TokenRef
		bot.ConfigVersion++
	}
	if body.ClientID != nil {
		parsed, err := parseOptionalClientID(body.ClientID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		bot.ClientID = parsed
	}
	if body.AssignedNodeID != nil {
		bot.AssignedNodeID = body.AssignedNodeID
	}
	if body.ConfigVersion != nil {
		bot.ConfigVersion = *body.ConfigVersion
	}
	if body.ScenarioConfig != nil {
		bot.ScenarioConfig = body.ScenarioConfig
		bot.ConfigVersion++
	}
	bot, err = s.Repos.Bots.Update(r.Context(), bot)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toBotDTO(maskBot(bot)))
}

// maskBot копирует бота с замаскированным token_ref для HTTP-ответов.
func maskBot(b store.Bot) store.Bot {
	b.TokenRef = launch.MaskTokenRef(b.TokenRef)
	return b
}

func maskBots(list []store.Bot) []store.Bot {
	out := make([]store.Bot, len(list))
	for i, b := range list {
		out[i] = maskBot(b)
	}
	return out
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("json: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func writeStoreErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, err.Error())
	case errors.Is(err, store.ErrConflict), errors.Is(err, store.ErrInvalidArgument):
		writeErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, store.ErrLimitExceeded):
		writeErr(w, http.StatusConflict, err.Error())
	default:
		writeErr(w, http.StatusInternalServerError, err.Error())
	}
}

// uuidRE — канонический UUID с дефисами (RFC 4122), без внешней uuid-библиотеки.
var uuidRE = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func validUUID(s string) bool {
	return uuidRE.MatchString(s)
}

// parseOptionalClientID: omit/пустая строка → nil; иначе UUID или ошибка.
func parseOptionalClientID(raw *string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	s := strings.TrimSpace(*raw)
	if s == "" {
		return nil, nil
	}
	if !validUUID(s) {
		return nil, fmt.Errorf("client_id должен быть UUID")
	}
	return &s, nil
}
