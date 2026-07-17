// Package ops — операции над ботами в store (start/stop/migrate), общие для ctl и control-api.
//
// Не импортирует HTTP и SQL: только интерфейсы store + launch-хелперы.
package ops

import (
	"context"
	"errors"
	"fmt"
	"time"

	"mvp-manager/internal/config"
	"mvp-manager/internal/launch"
	"mvp-manager/internal/store"
)

// Repos — набор репозиториев, нужных операциям.
type Repos struct {
	Nodes    store.NodeRepository
	Runtimes store.RuntimeRepository
	Bots     store.BotRepository
	Events   store.EventRepository // может быть nil — события не пишем
}

// Start выставляет desired=running боту и связанному runtime.
// maxBots — MAX_BOTS_PER_NODE (0 = без проверки); проверяем assigned ноду бота.
func Start(ctx context.Context, r Repos, botID string) error {
	return StartWithLimit(ctx, r, botID, 0)
}

// StartWithLimit — как Start, но с проверкой MAX_BOTS_PER_NODE.
func StartWithLimit(ctx context.Context, r Repos, botID string, maxBots int) error {
	bot, err := r.Bots.ByID(ctx, botID)
	if err != nil {
		return err
	}
	if bot.RuntimeID == nil {
		return fmt.Errorf("bot %s: нет runtime_id", botID)
	}
	if bot.AssignedNodeID != nil {
		if err := CheckBotLimit(ctx, r.Bots, *bot.AssignedNodeID, maxBots, false); err != nil {
			return err
		}
	}
	rtID := *bot.RuntimeID

	if err := r.Bots.UpdateDesiredState(ctx, botID, store.DesiredRunning); err != nil {
		return err
	}
	if err := r.Runtimes.UpdateDesiredState(ctx, rtID, store.DesiredRunning); err != nil {
		return err
	}

	rt, err := r.Runtimes.ByID(ctx, rtID)
	if err != nil {
		return err
	}
	if rt.ActualState == store.ActualFailed || rt.ActualState == store.ActualStopped {
		_ = r.Runtimes.UpdateActual(ctx, rtID, store.RuntimeActualPatch{
			ActualState: store.ActualUnknown,
		})
	}
	if bot.ActualState == store.ActualFailed || bot.ActualState == store.ActualStopped {
		_ = r.Bots.UpdateActual(ctx, botID, store.BotActualPatch{
			ActualState: store.ActualUnknown,
		})
	}
	appendEvent(ctx, r, botID, "started", "desired=running", nil)
	return nil
}

// Stop выставляет desired=stopped; для custom гасит runtime, для runner —
// только если больше нет desired=running default*.
func Stop(ctx context.Context, r Repos, botID string) error {
	bot, err := r.Bots.ByID(ctx, botID)
	if err != nil {
		return err
	}
	if bot.RuntimeID == nil {
		return fmt.Errorf("bot %s: нет runtime_id", botID)
	}
	rtID := *bot.RuntimeID

	if err := r.Bots.UpdateDesiredState(ctx, botID, store.DesiredStopped); err != nil {
		return err
	}

	rt, err := r.Runtimes.ByID(ctx, rtID)
	if err != nil {
		return err
	}

	if rt.Kind == store.RuntimeKindCustomBot {
		if err := r.Runtimes.UpdateDesiredState(ctx, rtID, store.DesiredStopped); err != nil {
			return err
		}
	} else if rt.Kind == store.RuntimeKindBotRunner {
		still, err := anyDefaultRunning(ctx, r.Bots, rtID, botID)
		if err != nil {
			return err
		}
		if !still {
			if err := r.Runtimes.UpdateDesiredState(ctx, rtID, store.DesiredStopped); err != nil {
				return err
			}
		}
	}
	appendEvent(ctx, r, botID, "stopped", "desired=stopped", nil)
	return nil
}

func anyDefaultRunning(ctx context.Context, bots store.BotRepository, runtimeID, exceptBotID string) (bool, error) {
	list, err := bots.ListByRuntime(ctx, runtimeID)
	if err != nil {
		return false, err
	}
	for _, b := range list {
		if b.ID == exceptBotID {
			continue
		}
		if launch.IsDefaultType(b.BotType) && b.DesiredState == store.DesiredRunning {
			return true, nil
		}
	}
	return false, nil
}

// MigrateOptions — параметры переноса бота на другую ноду (ТЗ §10.4).
type MigrateOptions struct {
	ToNodeID string
	// BotRunnerCommand / Workdir — для создания bot-runner на целевой ноде (default*).
	BotRunnerCommand string
	BotRunnerWorkdir string
	// WaitTimeout — сколько ждать actual=stopped на исходной ноде.
	WaitTimeout time.Duration
	// PollInterval — шаг опроса actual.
	PollInterval time.Duration
	// MaxBotsPerNode — лимит на целевой ноде (0 = без лимита).
	MaxBotsPerNode int
}

// Migrate: stop → wait actual stopped → reassign assigned_node (+ runtime для default) → start.
//
// Без двойного запуска: процесс на A гасится до смены assignment; B поднимает после.
func Migrate(ctx context.Context, r Repos, botID string, opt MigrateOptions) error {
	if opt.ToNodeID == "" {
		return fmt.Errorf("to_node_id пуст")
	}
	if opt.WaitTimeout <= 0 {
		opt.WaitTimeout = 30 * time.Second
	}
	if opt.PollInterval <= 0 {
		opt.PollInterval = 250 * time.Millisecond
	}

	bot, err := r.Bots.ByID(ctx, botID)
	if err != nil {
		return err
	}
	from := ""
	if bot.AssignedNodeID != nil {
		from = *bot.AssignedNodeID
	}
	if from == opt.ToNodeID {
		return fmt.Errorf("bot %s уже на ноде %s", botID, opt.ToNodeID)
	}

	// 1) Stop на текущей ноде.
	if err := Stop(ctx, r, botID); err != nil {
		return fmt.Errorf("migrate stop: %w", err)
	}

	// 2) Ждём actual=stopped у бота (и custom runtime).
	if err := waitBotStopped(ctx, r, botID, opt.WaitTimeout, opt.PollInterval); err != nil {
		return fmt.Errorf("migrate wait stopped: %w", err)
	}

	bot, err = r.Bots.ByID(ctx, botID)
	if err != nil {
		return err
	}
	to := opt.ToNodeID

	// 3) Reassign — проверка лимита на целевой ноде.
	// Лимит передаётся через opt.MaxBotsPerNode (0 = выкл.).
	if err := CheckBotLimit(ctx, r.Bots, to, opt.MaxBotsPerNode, true); err != nil {
		return fmt.Errorf("migrate limit: %w", err)
	}

	bot.AssignedNodeID = &to
	bot.ActualState = store.ActualMigrating
	bot.DesiredState = store.DesiredStopped // start выставит running ниже
	bot.LastError = nil

	if bot.BotType == store.BotTypeCustom {
		if bot.RuntimeID == nil {
			return fmt.Errorf("custom bot %s: нет runtime_id", botID)
		}
		rt, err := r.Runtimes.ByID(ctx, *bot.RuntimeID)
		if err != nil {
			return err
		}
		rt.AssignedNodeID = &to
		rt.DesiredState = store.DesiredStopped
		rt.ActualState = store.ActualStopped
		rt.PID = nil
		// Сброс lease — целевая нода захватит заново.
		rt.LeaseOwner = nil
		rt.LeaseUntil = nil
		if _, err := r.Runtimes.Update(ctx, rt); err != nil {
			return fmt.Errorf("reassign runtime: %w", err)
		}
	} else if launch.IsDefaultType(bot.BotType) {
		rt, err := getOrCreateBotRunner(ctx, r, to, opt.BotRunnerCommand, opt.BotRunnerWorkdir)
		if err != nil {
			return err
		}
		bot.RuntimeID = &rt.ID
	} else {
		return fmt.Errorf("unsupported bot_type %q for migrate", bot.BotType)
	}

	if _, err := r.Bots.Update(ctx, bot); err != nil {
		return fmt.Errorf("reassign bot: %w", err)
	}

	// 4) Start на целевой ноде.
	if err := StartWithLimit(ctx, r, botID, opt.MaxBotsPerNode); err != nil {
		return fmt.Errorf("migrate start on %s: %w", to, err)
	}

	appendEvent(ctx, r, botID, "migrated",
		fmt.Sprintf("from=%s to=%s", from, to),
		map[string]any{"from_node_id": from, "to_node_id": to})
	return nil
}

func waitBotStopped(ctx context.Context, r Repos, botID string, timeout, poll time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		bot, err := r.Bots.ByID(ctx, botID)
		if err != nil {
			return err
		}
		botOK := bot.ActualState == store.ActualStopped || bot.ActualState == store.ActualUnknown
		rtOK := true
		if bot.RuntimeID != nil {
			rt, err := r.Runtimes.ByID(ctx, *bot.RuntimeID)
			if err != nil {
				return err
			}
			// custom: ждём остановку dedicated процесса.
			// bot_runner: процесс может остаться (другие боты) — достаточно actual бота.
			if rt.Kind == store.RuntimeKindCustomBot {
				rtOK = rt.ActualState == store.ActualStopped ||
					rt.ActualState == store.ActualUnknown ||
					rt.PID == nil
			}
		}
		if botOK && rtOK {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting bot %s stopped (actual=%s)", botID, bot.ActualState)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(poll):
		}
	}
}

func getOrCreateBotRunner(ctx context.Context, r Repos, nodeID, cmd, workdir string) (store.Runtime, error) {
	name := launch.BotRunnerRuntimeName(nodeID)
	rt, err := r.Runtimes.ByName(ctx, name)
	if err == nil {
		if cmd != "" && rt.StartCommand != cmd {
			rt.StartCommand = cmd
			if workdir != "" {
				wd := workdir
				rt.Workdir = &wd
			}
			return r.Runtimes.Update(ctx, rt)
		}
		return rt, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return store.Runtime{}, err
	}
	if cmd == "" {
		cmd = "bot-runner"
	}
	var wd *string
	if workdir != "" {
		w := workdir
		wd = &w
	}
	n := nodeID
	return r.Runtimes.Create(ctx, store.Runtime{
		Kind:           store.RuntimeKindBotRunner,
		Name:           name,
		StartCommand:   cmd,
		Workdir:        wd,
		DesiredState:   store.DesiredStopped,
		ActualState:    store.ActualUnknown,
		AssignedNodeID: &n,
	})
}

func appendEvent(ctx context.Context, r Repos, botID, typ, msg string, meta map[string]any) {
	if r.Events == nil {
		return
	}
	_, _ = r.Events.Append(ctx, store.BotEvent{
		BotID:   botID,
		Type:    typ,
		Message: msg,
		Meta:    meta,
	})
}

// ReposFromConfig — удобная сборка для cmd (memory store уже открыт).
func ReposFromMemory(nodes store.NodeRepository, runtimes store.RuntimeRepository, bots store.BotRepository, events store.EventRepository) Repos {
	return Repos{Nodes: nodes, Runtimes: runtimes, Bots: bots, Events: events}
}

// MigrateOptsFromConfig заполняет команды runner из конфига агента/ctl.
func MigrateOptsFromConfig(cfg config.Config, toNode string) MigrateOptions {
	return MigrateOptions{
		ToNodeID:         toNode,
		BotRunnerCommand: cfg.BotRunnerCommand,
		BotRunnerWorkdir: cfg.BotRunnerWorkdir,
		WaitTimeout:      30 * time.Second,
		PollInterval:     250 * time.Millisecond,
		MaxBotsPerNode:   cfg.MaxBotsPerNode,
	}
}
