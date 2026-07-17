// Package reconcile сводит desired↔actual для custom_bot и bot_runner на ноде.
//
// Phase 1: kind=custom_bot.
// Phase 2: + ensure bot_runner для default*, реакция на unhealthy от healthcheck.
// Не импортирует pgx/database/sql — только интерфейсы store и supervisor.
package reconcile

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"mvp-manager/internal/launch"
	"mvp-manager/internal/store"
	"mvp-manager/internal/supervisor"
)

// Loop — периодический reconcile + heartbeat ноды.
type Loop struct {
	NodeID string

	Nodes    store.NodeRepository
	Runtimes store.RuntimeRepository
	Bots     store.BotRepository

	Supervisor *supervisor.Supervisor

	ReconcileInterval time.Duration
	HeartbeatInterval time.Duration
	// PublicURL прокидывается в ENV дочернего custom-процесса (опционально).
	PublicURL string

	// Параметры запуска bot-runner (из конфига агента, ТЗ §10.1).
	BotRunnerCommand    string
	BotRunnerWorkdir    string
	BotRunnerHealthPort string
	// StoreKind / MemoryStorePath — прокидываются в ENV дочернего runner
	// (общий MEMORY_STORE_PATH критичен для multi-process memory store).
	StoreKind       string
	MemoryStorePath string

	log *slog.Logger
}

// New создаёт Loop с разумными дефолтами интервалов.
func New(nodeID string, nodes store.NodeRepository, runtimes store.RuntimeRepository, bots store.BotRepository, sup *supervisor.Supervisor) *Loop {
	return &Loop{
		NodeID:            nodeID,
		Nodes:             nodes,
		Runtimes:          runtimes,
		Bots:              bots,
		Supervisor:        sup,
		ReconcileInterval: 3 * time.Second,
		HeartbeatInterval: 5 * time.Second,
		log:               slog.Default(),
	}
}

// Run крутит heartbeat и reconcile до отмены ctx.
// При выходе не останавливает процессы — это делает вызывающий (agent shutdown).
func (l *Loop) Run(ctx context.Context) error {
	if l.ReconcileInterval <= 0 {
		l.ReconcileInterval = 3 * time.Second
	}
	if l.HeartbeatInterval <= 0 {
		l.HeartbeatInterval = 5 * time.Second
	}

	reconcileTick := time.NewTicker(l.ReconcileInterval)
	defer reconcileTick.Stop()
	hbTick := time.NewTicker(l.HeartbeatInterval)
	defer hbTick.Stop()

	// Первый проход сразу — не ждать полный интервал после старта agent.
	if err := l.Heartbeat(ctx); err != nil {
		l.log.Warn("heartbeat", "err", err)
	}
	if err := l.Tick(ctx); err != nil {
		l.log.Warn("reconcile tick", "err", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-hbTick.C:
			if err := l.Heartbeat(ctx); err != nil {
				l.log.Warn("heartbeat", "err", err)
			}
		case <-reconcileTick.C:
			if err := l.Tick(ctx); err != nil {
				l.log.Warn("reconcile tick", "err", err)
			}
		}
	}
}

// Heartbeat обновляет last_seen_at ноды в store.
func (l *Loop) Heartbeat(ctx context.Context) error {
	if err := l.Nodes.Heartbeat(ctx, l.NodeID, time.Now().UTC(), store.NodeStatusOnline); err != nil {
		return fmt.Errorf("heartbeat %s: %w", l.NodeID, err)
	}
	return nil
}

// Tick — один проход сверки на этой ноде.
//
// Алгоритм:
//  1. ensure bot_runner (default*-боты, unhealthy→restart);
//  2. ListByNode runtimes → custom_bot как в Phase 1;
//  3. actual_state custom-ботов синхронизируется с runtime;
//     actual_state default*-ботов пишет сам bot-runner / healthcheck.
func (l *Loop) Tick(ctx context.Context) error {
	var first error
	if err := l.ensureBotRunner(ctx); err != nil {
		l.log.Warn("reconcile bot_runner", "err", err)
		first = err
	}

	runtimes, err := l.Runtimes.ListByNode(ctx, l.NodeID)
	if err != nil {
		return fmt.Errorf("list runtimes by node: %w", err)
	}

	for _, rt := range runtimes {
		if rt.Kind != store.RuntimeKindCustomBot {
			continue
		}
		if err := l.reconcileCustom(ctx, rt); err != nil {
			l.log.Warn("reconcile custom", "runtime_id", rt.ID, "err", err)
			if first == nil {
				first = err
			}
		}
	}
	return first
}

// Shutdown останавливает все процессы под supervisor (grace из supervisor).
func (l *Loop) Shutdown(ctx context.Context) error {
	l.log.Info("reconcile shutdown: останавливаем управляемые процессы")
	if err := l.Supervisor.StopAll(ctx); err != nil {
		return err
	}
	// Помечаем actual=stopped для runtimes на ноде, у которых мы держали процессы.
	runtimes, err := l.Runtimes.ListByNode(ctx, l.NodeID)
	if err != nil {
		return err
	}
	for _, rt := range runtimes {
		if rt.Kind != store.RuntimeKindCustomBot && rt.Kind != store.RuntimeKindBotRunner {
			continue
		}
		snap, ok := l.Supervisor.Snapshot(rt.ID)
		if ok && snap.Running {
			continue
		}
		_ = l.Runtimes.UpdateActual(ctx, rt.ID, store.RuntimeActualPatch{
			ActualState: store.ActualStopped,
			PID:         nil,
			ExitCode:    snapExit(snap, ok),
			LastError:   nil,
		})
		if rt.Kind == store.RuntimeKindCustomBot {
			l.syncBotsActual(ctx, rt.ID, store.ActualStopped, nil)
		} else {
			// default*: помечаем stopped без затирания через sync «всех как runtime».
			bots, _ := l.Bots.ListByRuntime(ctx, rt.ID)
			for _, b := range bots {
				_ = l.Bots.UpdateActual(ctx, b.ID, store.BotActualPatch{
					ActualState: store.ActualStopped,
					LastError:   nil,
				})
			}
		}
		l.Supervisor.Forget(rt.ID)
	}
	return nil
}

func snapExit(snap supervisor.Snapshot, ok bool) *int {
	if !ok {
		return nil
	}
	return snap.ExitCode
}

func (l *Loop) reconcileCustom(ctx context.Context, rt store.Runtime) error {
	snap, tracked := l.Supervisor.Snapshot(rt.ID)
	running := tracked && snap.Running

	switch rt.DesiredState {
	case store.DesiredRunning:
		return l.ensureRunning(ctx, rt, snap, tracked, running)
	case store.DesiredStopped:
		return l.ensureStopped(ctx, rt, snap, tracked, running)
	default:
		return fmt.Errorf("unknown desired_state %q", rt.DesiredState)
	}
}

func (l *Loop) ensureRunning(ctx context.Context, rt store.Runtime, snap supervisor.Snapshot, tracked, running bool) error {
	// Процесс был на учёте и умер → failed (агент остаётся жив).
	if tracked && !running {
		msg := "process exited unexpectedly"
		if snap.WaitErr != nil {
			msg = snap.WaitErr.Error()
		}
		errMsg := msg
		if err := l.Runtimes.UpdateActual(ctx, rt.ID, store.RuntimeActualPatch{
			ActualState: store.ActualFailed,
			PID:         nil,
			ExitCode:    snap.ExitCode,
			LastError:   &errMsg,
		}); err != nil {
			return err
		}
		l.syncBotsActual(ctx, rt.ID, store.ActualFailed, &errMsg)
		l.Supervisor.Forget(rt.ID)
		// Без авто-рестарта: actual=failed при desired=running остаётся
		// до ctl stop→start (см. ветку ниже). Агент при этом жив.
		return nil
	}

	// Уже failed в store — ждём смены desired (stop), не долбим Start.
	if rt.ActualState == store.ActualFailed {
		return nil
	}

	if running {
		// Уже жив — убедимся, что store отражает running/pid.
		pid := snap.PID
		if rt.ActualState != store.ActualRunning || rt.PID == nil || *rt.PID != pid {
			if err := l.Runtimes.UpdateActual(ctx, rt.ID, store.RuntimeActualPatch{
				ActualState: store.ActualRunning,
				PID:         &pid,
				ExitCode:    nil,
				LastError:   nil,
			}); err != nil {
				return err
			}
			l.syncBotsActual(ctx, rt.ID, store.ActualRunning, nil)
		}
		return nil
	}

	// Нет процесса — стартуем.
	if err := l.Runtimes.UpdateActual(ctx, rt.ID, store.RuntimeActualPatch{
		ActualState: store.ActualStarting,
		PID:         nil,
		ExitCode:    nil,
		LastError:   nil,
	}); err != nil {
		return err
	}
	l.syncBotsActual(ctx, rt.ID, store.ActualStarting, nil)

	bots, err := l.Bots.ListByRuntime(ctx, rt.ID)
	if err != nil {
		return fmt.Errorf("list bots by runtime: %w", err)
	}
	if len(bots) == 0 {
		msg := "no bot linked to runtime"
		_ = l.Runtimes.UpdateActual(ctx, rt.ID, store.RuntimeActualPatch{
			ActualState: store.ActualFailed,
			LastError:   &msg,
		})
		return fmt.Errorf("runtime %s: %s", rt.ID, msg)
	}
	bot := bots[0] // custom 1:1

	cmd := launch.StartCommand(rt, bot)
	if cmd == "" {
		msg := "empty start_command"
		_ = l.Runtimes.UpdateActual(ctx, rt.ID, store.RuntimeActualPatch{
			ActualState: store.ActualFailed,
			LastError:   &msg,
		})
		l.syncBotsActual(ctx, rt.ID, store.ActualFailed, &msg)
		return fmt.Errorf("runtime %s: %s", rt.ID, msg)
	}

	workdir := ""
	if rt.Workdir != nil {
		workdir = *rt.Workdir
	}
	if workdir == "" {
		workdir = launch.WorkdirFromBot(bot)
	}

	pid, err := l.Supervisor.Start(ctx, supervisor.Spec{
		ID:      rt.ID,
		Command: cmd,
		Workdir: workdir,
		Env:     launch.BuildEnv(bot, l.PublicURL),
	})
	if err != nil {
		msg := err.Error()
		_ = l.Runtimes.UpdateActual(ctx, rt.ID, store.RuntimeActualPatch{
			ActualState: store.ActualFailed,
			LastError:   &msg,
		})
		l.syncBotsActual(ctx, rt.ID, store.ActualFailed, &msg)
		return fmt.Errorf("start runtime %s: %w", rt.ID, err)
	}

	if err := l.Runtimes.UpdateActual(ctx, rt.ID, store.RuntimeActualPatch{
		ActualState: store.ActualRunning,
		PID:         &pid,
		ExitCode:    nil,
		LastError:   nil,
	}); err != nil {
		return err
	}
	l.syncBotsActual(ctx, rt.ID, store.ActualRunning, nil)
	l.log.Info("custom runtime started", "runtime_id", rt.ID, "pid", pid, "bot_id", bot.ID)
	return nil
}

func (l *Loop) ensureStopped(ctx context.Context, rt store.Runtime, snap supervisor.Snapshot, tracked, running bool) error {
	if running {
		if err := l.Runtimes.UpdateActual(ctx, rt.ID, store.RuntimeActualPatch{
			ActualState: store.ActualStopping,
			PID:         rt.PID,
			ExitCode:    nil,
			LastError:   nil,
		}); err != nil {
			return err
		}
		l.syncBotsActual(ctx, rt.ID, store.ActualStopping, nil)

		if err := l.Supervisor.Stop(ctx, rt.ID); err != nil {
			msg := err.Error()
			_ = l.Runtimes.UpdateActual(ctx, rt.ID, store.RuntimeActualPatch{
				ActualState: store.ActualFailed,
				LastError:   &msg,
			})
			return err
		}
	}

	// После stop / если не был running — stopped в store.
	var exitCode *int
	if tracked {
		exitCode = snap.ExitCode
		// Перечитаем snapshot после Stop.
		if s2, ok := l.Supervisor.Snapshot(rt.ID); ok {
			exitCode = s2.ExitCode
		}
	}
	if err := l.Runtimes.UpdateActual(ctx, rt.ID, store.RuntimeActualPatch{
		ActualState: store.ActualStopped,
		PID:         nil,
		ExitCode:    exitCode,
		LastError:   nil,
	}); err != nil {
		return err
	}
	l.syncBotsActual(ctx, rt.ID, store.ActualStopped, nil)
	l.Supervisor.Forget(rt.ID)
	return nil
}

// syncBotsActual копирует actual (и last_error) на всех ботов runtime.
func (l *Loop) syncBotsActual(ctx context.Context, runtimeID string, state store.ActualState, lastErr *string) {
	bots, err := l.Bots.ListByRuntime(ctx, runtimeID)
	if err != nil {
		l.log.Warn("sync bot actual: list", "runtime_id", runtimeID, "err", err)
		return
	}
	for _, b := range bots {
		if err := l.Bots.UpdateActual(ctx, b.ID, store.BotActualPatch{
			ActualState: state,
			LastError:   lastErr,
		}); err != nil {
			l.log.Warn("sync bot actual", "bot_id", b.ID, "err", err)
		}
	}
}
