package api

import (
	"time"

	"mvp-manager/internal/store"
)

// Ответные DTO control-api в каноническом snake_case (docs/frontend.md §5.5).
// Отделены от store.*: memory persist и SQL-маппинг продолжают работать
// с доменными типами без json-тегов (PascalCase в snapshot-файле).

// nodeDTO — JSON-представление store.Node.
type nodeDTO struct {
	ID           string           `json:"id"`
	Hostname     string           `json:"hostname"`
	Status       store.NodeStatus `json:"status"`
	LastSeenAt   time.Time        `json:"last_seen_at"`
	AgentVersion *string          `json:"agent_version"`
	Meta         map[string]any   `json:"meta"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
}

// runtimeDTO — JSON-представление store.Runtime.
type runtimeDTO struct {
	ID             string             `json:"id"`
	Kind           store.RuntimeKind  `json:"kind"`
	Name           string             `json:"name"`
	StartCommand   string             `json:"start_command"`
	Workdir        *string            `json:"workdir"`
	Env            map[string]any     `json:"env"`
	DesiredState   store.DesiredState `json:"desired_state"`
	ActualState    store.ActualState  `json:"actual_state"`
	AssignedNodeID *string            `json:"assigned_node_id"`
	LeaseOwner     *string            `json:"lease_owner"`
	LeaseUntil     *time.Time         `json:"lease_until"`
	PID            *int               `json:"pid"`
	ExitCode       *int               `json:"exit_code"`
	LastError      *string            `json:"last_error"`
	ConfigVersion  int64              `json:"config_version"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
}

// botDTO — JSON-представление store.Bot (token_ref уже должен быть замаскирован).
type botDTO struct {
	ID             string             `json:"id"`
	ClientID       *string            `json:"client_id"`
	Name           string             `json:"name"`
	BotType        store.BotType      `json:"bot_type"`
	CustomName     *string            `json:"custom_name"`
	Channel        store.BotChannel   `json:"channel"`
	RunMode        store.BotRunMode   `json:"run_mode"`
	Port           int                `json:"port"`
	TokenRef       string             `json:"token_ref"`
	RuntimeID      *string            `json:"runtime_id"`
	ArtifactPath   *string            `json:"artifact_path"`
	RepoURL        *string            `json:"repo_url"`
	StartCommand   *string            `json:"start_command"`
	DesiredState   store.DesiredState `json:"desired_state"`
	ActualState    store.ActualState  `json:"actual_state"`
	AssignedNodeID *string            `json:"assigned_node_id"`
	LastError      *string            `json:"last_error"`
	ConfigVersion  int64              `json:"config_version"`
	ScenarioConfig map[string]any     `json:"scenario_config"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
}

// botEventDTO — JSON-представление store.BotEvent.
type botEventDTO struct {
	ID      string         `json:"id"`
	BotID   string         `json:"bot_id"`
	Type    string         `json:"type"`
	Message string         `json:"message"`
	At      time.Time      `json:"at"`
	Meta    map[string]any `json:"meta"`
}

func toNodeDTO(n store.Node) nodeDTO {
	return nodeDTO{
		ID:           n.ID,
		Hostname:     n.Hostname,
		Status:       n.Status,
		LastSeenAt:   n.LastSeenAt,
		AgentVersion: n.AgentVersion,
		Meta:         n.Meta,
		CreatedAt:    n.CreatedAt,
		UpdatedAt:    n.UpdatedAt,
	}
}

func toNodeDTOs(list []store.Node) []nodeDTO {
	out := make([]nodeDTO, len(list))
	for i, n := range list {
		out[i] = toNodeDTO(n)
	}
	return out
}

func toRuntimeDTO(rt store.Runtime) runtimeDTO {
	return runtimeDTO{
		ID:             rt.ID,
		Kind:           rt.Kind,
		Name:           rt.Name,
		StartCommand:   rt.StartCommand,
		Workdir:        rt.Workdir,
		Env:            rt.Env,
		DesiredState:   rt.DesiredState,
		ActualState:    rt.ActualState,
		AssignedNodeID: rt.AssignedNodeID,
		LeaseOwner:     rt.LeaseOwner,
		LeaseUntil:     rt.LeaseUntil,
		PID:            rt.PID,
		ExitCode:       rt.ExitCode,
		LastError:      rt.LastError,
		ConfigVersion:  rt.ConfigVersion,
		CreatedAt:      rt.CreatedAt,
		UpdatedAt:      rt.UpdatedAt,
	}
}

func toRuntimeDTOs(list []store.Runtime) []runtimeDTO {
	out := make([]runtimeDTO, len(list))
	for i, rt := range list {
		out[i] = toRuntimeDTO(rt)
	}
	return out
}

func toBotDTO(b store.Bot) botDTO {
	return botDTO{
		ID:             b.ID,
		ClientID:       b.ClientID,
		Name:           b.Name,
		BotType:        b.BotType,
		CustomName:     b.CustomName,
		Channel:        b.Channel,
		RunMode:        b.RunMode,
		Port:           b.Port,
		TokenRef:       b.TokenRef,
		RuntimeID:      b.RuntimeID,
		ArtifactPath:   b.ArtifactPath,
		RepoURL:        b.RepoURL,
		StartCommand:   b.StartCommand,
		DesiredState:   b.DesiredState,
		ActualState:    b.ActualState,
		AssignedNodeID: b.AssignedNodeID,
		LastError:      b.LastError,
		ConfigVersion:  b.ConfigVersion,
		ScenarioConfig: b.ScenarioConfig,
		CreatedAt:      b.CreatedAt,
		UpdatedAt:      b.UpdatedAt,
	}
}

func toBotDTOs(list []store.Bot) []botDTO {
	out := make([]botDTO, len(list))
	for i, b := range list {
		out[i] = toBotDTO(b)
	}
	return out
}

func toBotEventDTO(ev store.BotEvent) botEventDTO {
	return botEventDTO{
		ID:      ev.ID,
		BotID:   ev.BotID,
		Type:    ev.Type,
		Message: ev.Message,
		At:      ev.At,
		Meta:    ev.Meta,
	}
}

func toBotEventDTOs(list []store.BotEvent) []botEventDTO {
	out := make([]botEventDTO, len(list))
	for i, ev := range list {
		out[i] = toBotEventDTO(ev)
	}
	return out
}
