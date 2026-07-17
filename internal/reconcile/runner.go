package reconcile

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"mvp-manager/internal/launch"
	"mvp-manager/internal/store"
	"mvp-manager/internal/supervisor"
)

// Поля Loop для управления bot_runner (задаются из agent main).
// Вынесены в этот файл рядом с логикой, чтобы loop.go оставался про custom.

// ensureBotRunner — Phase 2: один bot_runner на ноду.
//
//  1. Если есть default*-боты — создать runtime при отсутствии;
//  2. Привязать bots.runtime_id к этому runner;
//  3. Если нет desired=running default* — desired runtime=stopped;
//  4. Старт/стоп процесса по runtime.desired через supervisor;
//  5. Unhealthy (healthcheck) → рестарт OS-процесса runner (простейшая policy).
func (l *Loop) ensureBotRunner(ctx context.Context) error {
	bots, err := l.Bots.ListByNode(ctx, l.NodeID)
	if err != nil {
		return fmt.Errorf("list bots: %w", err)
	}

	var defaults []store.Bot
	needRunning := false
	for _, b := range bots {
		if !launch.IsDefaultType(b.BotType) {
			continue
		}
		defaults = append(defaults, b)
		if b.DesiredState == store.DesiredRunning {
			needRunning = true
		}
	}

	// Нет default-ботов на ноде — нечего обеспечивать.
	if len(defaults) == 0 {
		return nil
	}

	rt, err := l.getOrCreateBotRunner(ctx)
	if err != nil {
		return err
	}

	// Привязка runtime_id ко всем default* на ноде.
	for _, b := range defaults {
		if b.RuntimeID != nil && *b.RuntimeID == rt.ID {
			continue
		}
		b.RuntimeID = &rt.ID
		if _, err := l.Bots.Update(ctx, b); err != nil {
			return fmt.Errorf("bind bot %s to runner: %w", b.ID, err)
		}
		l.log.Info("bound bot to bot_runner", "bot_id", b.ID, "runtime_id", rt.ID)
	}

	// Нет running default* → останавливаем runner (desired + процесс).
	if !needRunning {
		if rt.DesiredState != store.DesiredStopped {
			if err := l.Runtimes.UpdateDesiredState(ctx, rt.ID, store.DesiredStopped); err != nil {
				return err
			}
			rt.DesiredState = store.DesiredStopped
		}
		return l.reconcileBotRunner(ctx, rt, false)
	}

	// Есть running default* — desired runtime должен быть running, если ctl
	// явно не остановил runner. Не форсим running поверх desired=stopped:
	// E2E «stop runner» оставляет ботов desired=running, но процесс убит.
	// Исключение: runtime только что создан с stopped и боты уже start —
	// ctl bots start сам ставит runtime desired=running.

	return l.reconcileBotRunner(ctx, rt, true)
}

func (l *Loop) getOrCreateBotRunner(ctx context.Context) (store.Runtime, error) {
	name := launch.BotRunnerRuntimeName(l.NodeID)
	rt, err := l.Runtimes.ByName(ctx, name)
	if err == nil {
		// Подтянуть актуальный BOT_RUNNER_COMMAND из конфига агента
		// (ctl мог создать runtime с плейсхолдером «bot-runner»).
		return l.refreshBotRunnerCommand(ctx, rt)
	}
	if !errors.Is(err, store.ErrNotFound) {
		return store.Runtime{}, err
	}

	cmd := l.BotRunnerCommand
	if cmd == "" {
		return store.Runtime{}, fmt.Errorf("BOT_RUNNER_COMMAND пуст: нечем стартовать bot-runner")
	}
	var wd *string
	if l.BotRunnerWorkdir != "" {
		w := l.BotRunnerWorkdir
		wd = &w
	}
	nodeID := l.NodeID
	created, err := l.Runtimes.Create(ctx, store.Runtime{
		Kind:           store.RuntimeKindBotRunner,
		Name:           name,
		StartCommand:   cmd,
		Workdir:        wd,
		DesiredState:   store.DesiredStopped,
		ActualState:    store.ActualUnknown,
		AssignedNodeID: &nodeID,
	})
	if err != nil {
		return store.Runtime{}, fmt.Errorf("create bot_runner runtime: %w", err)
	}
	l.log.Info("created bot_runner runtime", "runtime_id", created.ID, "name", name)
	return created, nil
}

// refreshBotRunnerCommand обновляет start_command/workdir из конфига агента.
func (l *Loop) refreshBotRunnerCommand(ctx context.Context, rt store.Runtime) (store.Runtime, error) {
	if l.BotRunnerCommand == "" {
		return rt, nil
	}
	need := rt.StartCommand != l.BotRunnerCommand
	if l.BotRunnerWorkdir != "" {
		if rt.Workdir == nil || *rt.Workdir != l.BotRunnerWorkdir {
			need = true
		}
	}
	if !need {
		return rt, nil
	}
	rt.StartCommand = l.BotRunnerCommand
	if l.BotRunnerWorkdir != "" {
		w := l.BotRunnerWorkdir
		rt.Workdir = &w
	}
	return l.Runtimes.Update(ctx, rt)
}

// reconcileBotRunner управляет OS-процессом runner (не actual_state каждого бота).
//
// Policy восстановления unhealthy: если процесс жив, desired=running и хотя бы
// один default-бот помечен healthcheck как failed — Stop+Start всего runner.
// После рестарта инстансы поднимаются заново со «здоровым» /healthz.
func (l *Loop) reconcileBotRunner(ctx context.Context, rt store.Runtime, allowUnhealthyRestart bool) error {
	// Перечитаем runtime — desired мог измениться.
	fresh, err := l.Runtimes.ByID(ctx, rt.ID)
	if err != nil {
		return err
	}
	rt = fresh

	snap, tracked := l.Supervisor.Snapshot(rt.ID)
	running := tracked && snap.Running

	if allowUnhealthyRestart && rt.DesiredState == store.DesiredRunning && running {
		if unhealthy, botID := l.findUnhealthyDefault(ctx, rt.ID); unhealthy {
			l.log.Info("unhealthy bot → restart bot_runner",
				"runtime_id", rt.ID, "bot_id", botID)
			if err := l.restartBotRunner(ctx, rt); err != nil {
				return err
			}
			return nil
		}
	}

	switch rt.DesiredState {
	case store.DesiredRunning:
		return l.ensureBotRunnerRunning(ctx, rt, snap, tracked, running)
	case store.DesiredStopped:
		return l.ensureBotRunnerStopped(ctx, rt, snap, tracked, running)
	default:
		return fmt.Errorf("bot_runner: unknown desired_state %q", rt.DesiredState)
	}
}

func (l *Loop) findUnhealthyDefault(ctx context.Context, runtimeID string) (bool, string) {
	bots, err := l.Bots.ListByRuntime(ctx, runtimeID)
	if err != nil {
		return false, ""
	}
	for _, b := range bots {
		if !launch.IsDefaultType(b.BotType) {
			continue
		}
		if b.DesiredState != store.DesiredRunning {
			continue
		}
		if b.ActualState != store.ActualFailed {
			continue
		}
		if b.LastError != nil && strings.HasPrefix(*b.LastError, launch.HealthcheckErrPrefix) {
			return true, b.ID
		}
	}
	return false, ""
}

func (l *Loop) restartBotRunner(ctx context.Context, rt store.Runtime) error {
	if err := l.Supervisor.Stop(ctx, rt.ID); err != nil {
		return fmt.Errorf("stop for restart: %w", err)
	}
	l.Supervisor.Forget(rt.ID)

	// Сброс actual ботов — runner выставит running после подъема инстансов.
	bots, _ := l.Bots.ListByRuntime(ctx, rt.ID)
	for _, b := range bots {
		if b.DesiredState != store.DesiredRunning {
			continue
		}
		_ = l.Bots.UpdateActual(ctx, b.ID, store.BotActualPatch{
			ActualState: store.ActualUnknown,
			LastError:   nil,
		})
	}

	_ = l.Runtimes.UpdateActual(ctx, rt.ID, store.RuntimeActualPatch{
		ActualState: store.ActualUnknown,
		PID:         nil,
		ExitCode:    nil,
		LastError:   nil,
	})

	rt2, err := l.Runtimes.ByID(ctx, rt.ID)
	if err != nil {
		return err
	}
	return l.ensureBotRunnerRunning(ctx, rt2, supervisor.Snapshot{}, false, false)
}

func (l *Loop) ensureBotRunnerRunning(ctx context.Context, rt store.Runtime, snap supervisor.Snapshot, tracked, running bool) error {
	if tracked && !running {
		msg := "bot_runner exited unexpectedly"
		if snap.WaitErr != nil {
			msg = snap.WaitErr.Error()
		}
		errMsg := msg
		_ = l.Runtimes.UpdateActual(ctx, rt.ID, store.RuntimeActualPatch{
			ActualState: store.ActualFailed,
			PID:         nil,
			ExitCode:    snap.ExitCode,
			LastError:   &errMsg,
		})
		l.Supervisor.Forget(rt.ID)
		// Авто-рестарт runner при падении процесса (в отличие от custom Phase 1).
		l.log.Info("bot_runner crashed → restart", "runtime_id", rt.ID)
		rt.ActualState = store.ActualUnknown
		return l.startBotRunnerProcess(ctx, rt)
	}

	if running {
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
		}
		// actual_state ботов пишет сам runner — здесь не затираем.
		return nil
	}

	return l.startBotRunnerProcess(ctx, rt)
}

func (l *Loop) startBotRunnerProcess(ctx context.Context, rt store.Runtime) error {
	// Предпочитаем команду из конфига агента — источник истины на ноде.
	cmd := l.BotRunnerCommand
	if cmd == "" {
		cmd = rt.StartCommand
	}
	if cmd == "" {
		msg := "empty BOT_RUNNER_COMMAND"
		_ = l.Runtimes.UpdateActual(ctx, rt.ID, store.RuntimeActualPatch{
			ActualState: store.ActualFailed,
			LastError:   &msg,
		})
		return fmt.Errorf("runtime %s: %s", rt.ID, msg)
	}

	workdir := ""
	if rt.Workdir != nil {
		workdir = *rt.Workdir
	}
	if workdir == "" {
		workdir = l.BotRunnerWorkdir
	}

	if err := l.Runtimes.UpdateActual(ctx, rt.ID, store.RuntimeActualPatch{
		ActualState: store.ActualStarting,
		PID:         nil,
		ExitCode:    nil,
		LastError:   nil,
	}); err != nil {
		return err
	}

	storeKind := l.StoreKind
	if storeKind == "" {
		storeKind = "memory"
	}
	pid, err := l.Supervisor.Start(ctx, supervisor.Spec{
		ID:      rt.ID,
		Command: cmd,
		Workdir: workdir,
		Env: launch.BuildRunnerEnv(
			l.NodeID, rt.ID, storeKind, l.MemoryStorePath, l.BotRunnerHealthPort,
		),
	})
	if err != nil {
		msg := err.Error()
		_ = l.Runtimes.UpdateActual(ctx, rt.ID, store.RuntimeActualPatch{
			ActualState: store.ActualFailed,
			LastError:   &msg,
		})
		return fmt.Errorf("start bot_runner %s: %w", rt.ID, err)
	}

	if err := l.Runtimes.UpdateActual(ctx, rt.ID, store.RuntimeActualPatch{
		ActualState: store.ActualRunning,
		PID:         &pid,
		ExitCode:    nil,
		LastError:   nil,
	}); err != nil {
		return err
	}
	l.log.Info("bot_runner started", "runtime_id", rt.ID, "pid", pid)
	return nil
}

func (l *Loop) ensureBotRunnerStopped(ctx context.Context, rt store.Runtime, snap supervisor.Snapshot, tracked, running bool) error {
	if running {
		if err := l.Runtimes.UpdateActual(ctx, rt.ID, store.RuntimeActualPatch{
			ActualState: store.ActualStopping,
			PID:         rt.PID,
			ExitCode:    nil,
			LastError:   nil,
		}); err != nil {
			return err
		}
		if err := l.Supervisor.Stop(ctx, rt.ID); err != nil {
			msg := err.Error()
			_ = l.Runtimes.UpdateActual(ctx, rt.ID, store.RuntimeActualPatch{
				ActualState: store.ActualFailed,
				LastError:   &msg,
			})
			return err
		}
	}

	var exitCode *int
	if tracked {
		if s2, ok := l.Supervisor.Snapshot(rt.ID); ok {
			exitCode = s2.ExitCode
		} else {
			exitCode = snap.ExitCode
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
	// Ботов помечаем stopped, если их инстансы умерли вместе с runner.
	bots, _ := l.Bots.ListByRuntime(ctx, rt.ID)
	for _, b := range bots {
		if !launch.IsDefaultType(b.BotType) {
			continue
		}
		if b.ActualState == store.ActualStopped {
			continue
		}
		_ = l.Bots.UpdateActual(ctx, b.ID, store.BotActualPatch{
			ActualState: store.ActualStopped,
			LastError:   nil,
		})
	}
	l.Supervisor.Forget(rt.ID)
	return nil
}
