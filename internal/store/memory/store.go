package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"mvp-manager/internal/store"
)

// Store — фасад потокобезопасного in-memory хранилища.
//
// В Go один тип не может реализовать Node/Runtime/Bot репозитории сразу:
// методы ByID, List, Create, Update отличаются только типами возврата.
// Поэтому три тонких адаптера (Nodes / Runtimes / Bots) делят общее
// состояние shared и мьютекс — снаружи это один объект New().
type Store struct {
	Nodes    *NodeRepo
	Runtimes *RuntimeRepo
	Bots     *BotRepo
}

// shared — внутренние maps и индексы UNIQUE под одним RWMutex.
type shared struct {
	mu sync.RWMutex

	nodes    map[string]store.Node
	runtimes map[string]store.Runtime
	bots     map[string]store.Bot

	// byRuntimeName: name → runtime id (UNIQUE name, ТЗ §6.2).
	byRuntimeName map[string]string
	// byPort: port → bot id (UNIQUE port, ТЗ §6.3).
	byPort map[int]string
}

// Compile-time проверка контракта на адаптерах (не на фасаде).
var (
	_ store.NodeRepository    = (*NodeRepo)(nil)
	_ store.RuntimeRepository = (*RuntimeRepo)(nil)
	_ store.BotRepository     = (*BotRepo)(nil)
)

// NodeRepo реализует store.NodeRepository над общим shared.
type NodeRepo struct{ s *shared }

// RuntimeRepo реализует store.RuntimeRepository над общим shared.
type RuntimeRepo struct{ s *shared }

// BotRepo реализует store.BotRepository над общим shared.
type BotRepo struct{ s *shared }

// New создаёт пустое хранилище. Данные не персистятся на диск:
// рестарт процесса = пустое состояние (ок для Phase 0 / memory).
func New() *Store {
	sh := &shared{
		nodes:         make(map[string]store.Node),
		runtimes:      make(map[string]store.Runtime),
		bots:          make(map[string]store.Bot),
		byRuntimeName: make(map[string]string),
		byPort:        make(map[int]string),
	}
	return &Store{
		Nodes:    &NodeRepo{s: sh},
		Runtimes: &RuntimeRepo{s: sh},
		Bots:     &BotRepo{s: sh},
	}
}

// checkCtx — ранняя проверка отмены до захвата мьютекса.
func checkCtx(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("store memory: %w", err)
	}
	return nil
}

// now — единая точка времени для CreatedAt/UpdatedAt/LastSeenAt (UTC).
func now() time.Time {
	return time.Now().UTC()
}

// cloneStringMap копирует map[string]any, чтобы внешний код не мутировал
// внутреннее состояние store через общие ссылки на Meta/Env/ScenarioConfig.
func cloneStringMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneNode(n store.Node) store.Node {
	n.Meta = cloneStringMap(n.Meta)
	if n.AgentVersion != nil {
		v := *n.AgentVersion
		n.AgentVersion = &v
	}
	return n
}

func cloneRuntime(r store.Runtime) store.Runtime {
	r.Env = cloneStringMap(r.Env)
	if r.Workdir != nil {
		v := *r.Workdir
		r.Workdir = &v
	}
	if r.AssignedNodeID != nil {
		v := *r.AssignedNodeID
		r.AssignedNodeID = &v
	}
	if r.LeaseOwner != nil {
		v := *r.LeaseOwner
		r.LeaseOwner = &v
	}
	if r.LeaseUntil != nil {
		v := *r.LeaseUntil
		r.LeaseUntil = &v
	}
	if r.PID != nil {
		v := *r.PID
		r.PID = &v
	}
	if r.ExitCode != nil {
		v := *r.ExitCode
		r.ExitCode = &v
	}
	if r.LastError != nil {
		v := *r.LastError
		r.LastError = &v
	}
	return r
}

func cloneBot(b store.Bot) store.Bot {
	b.ScenarioConfig = cloneStringMap(b.ScenarioConfig)
	if b.ClientID != nil {
		v := *b.ClientID
		b.ClientID = &v
	}
	if b.CustomName != nil {
		v := *b.CustomName
		b.CustomName = &v
	}
	if b.RuntimeID != nil {
		v := *b.RuntimeID
		b.RuntimeID = &v
	}
	if b.ArtifactPath != nil {
		v := *b.ArtifactPath
		b.ArtifactPath = &v
	}
	if b.RepoURL != nil {
		v := *b.RepoURL
		b.RepoURL = &v
	}
	if b.StartCommand != nil {
		v := *b.StartCommand
		b.StartCommand = &v
	}
	if b.AssignedNodeID != nil {
		v := *b.AssignedNodeID
		b.AssignedNodeID = &v
	}
	if b.LastError != nil {
		v := *b.LastError
		b.LastError = &v
	}
	return b
}
