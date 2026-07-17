// Package runner — multi-tenant ядро bot-runner (вшитые default*-сценарии).
//
// Не запускает custom-ботов и не импортирует SQL/pgx: только store-интерфейсы.
package runner

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"mvp-manager/internal/launch"
	"mvp-manager/internal/store"
)

// Manager синхронизирует набор default*-инстансов с store.
//
// Пишет actual_state ботов сам (агент пишет actual runtime/PID).
type Manager struct {
	NodeID    string
	RuntimeID string

	Bots store.BotRepository

	// SyncInterval — как часто перечитывать store (add/remove/reload).
	SyncInterval time.Duration

	// HealthAddr — опциональный служебный listen runner’а (например ":19000").
	// Пусто — не поднимаем отдельный /healthz процесса.
	HealthAddr string

	log *slog.Logger
	reg *registry
}

// New создаёт Manager с дефолтным интервалом синхронизации.
func New(nodeID, runtimeID string, bots store.BotRepository) *Manager {
	return &Manager{
		NodeID:       nodeID,
		RuntimeID:    runtimeID,
		Bots:         bots,
		SyncInterval: time.Second,
		log:          slog.Default(),
		reg:          newRegistry(),
	}
}

// Run крутит sync до отмены ctx; при выходе останавливает все инстансы.
func (m *Manager) Run(ctx context.Context) error {
	if m.SyncInterval <= 0 {
		m.SyncInterval = time.Second
	}
	if m.log == nil {
		m.log = slog.Default()
	}
	if m.reg == nil {
		m.reg = newRegistry()
	}

	var healthSrv *http.Server
	if m.HealthAddr != "" {
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok\n"))
		})
		healthSrv = &http.Server{
			Addr:              m.HealthAddr,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		}
		go func() {
			m.log.Info("runner health listen", "addr", m.HealthAddr)
			if err := healthSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				m.log.Warn("runner health server", "err", err)
			}
		}()
	}

	ticker := time.NewTicker(m.SyncInterval)
	defer ticker.Stop()

	if err := m.Sync(ctx); err != nil {
		m.log.Warn("runner sync", "err", err)
	}

	for {
		select {
		case <-ctx.Done():
			m.stopAll()
			if healthSrv != nil {
				shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				_ = healthSrv.Shutdown(shutCtx)
				cancel()
			}
			return ctx.Err()
		case <-ticker.C:
			if err := m.Sync(ctx); err != nil {
				m.log.Warn("runner sync", "err", err)
			}
		}
	}
}

// Sync один проход: add / remove / reload по store.
//
// Набор целевых ботов: bot_type default*, assigned_node_id=self,
// desired_state=running, runtime_id=этот runner.
func (m *Manager) Sync(ctx context.Context) error {
	bots, err := m.Bots.ListByRuntime(ctx, m.RuntimeID)
	if err != nil {
		return fmt.Errorf("list bots by runtime: %w", err)
	}

	wanted := make(map[string]store.Bot)
	for _, b := range bots {
		if !launch.IsDefaultType(b.BotType) {
			continue
		}
		if b.AssignedNodeID == nil || *b.AssignedNodeID != m.NodeID {
			continue
		}
		if b.DesiredState != store.DesiredRunning {
			continue
		}
		wanted[b.ID] = b
	}

	m.reg.mu.Lock()
	defer m.reg.mu.Unlock()

	// Remove: инстанс есть, в wanted нет.
	for id, inst := range m.reg.byID {
		if _, ok := wanted[id]; ok {
			continue
		}
		m.log.Info("remove instance", "bot_id", id)
		inst.stop()
		delete(m.reg.byID, id)
		delete(m.reg.fp, id)
		_ = m.Bots.UpdateActual(ctx, id, store.BotActualPatch{
			ActualState: store.ActualStopped,
			LastError:   nil,
		})
	}

	// Add / reload.
	for id, bot := range wanted {
		wantFP := fpOf(bot)
		inst, exists := m.reg.byID[id]
		if exists {
			if m.reg.fp[id] == wantFP {
				// Конфиг тот же. actual_state не трогаем: healthcheck мог
				// выставить failed/unhealthy — восстанавливает agent (рестарт runtime).
				continue
			}
			// Смена token / config_version / mode / port → reload.
			m.log.Info("reload instance", "bot_id", id, "config_version", bot.ConfigVersion)
			inst.stop()
			delete(m.reg.byID, id)
			delete(m.reg.fp, id)
		}

		_ = m.Bots.UpdateActual(ctx, id, store.BotActualPatch{
			ActualState: store.ActualStarting,
			LastError:   nil,
		})
		newInst, err := startInstance(ctx, bot, m.log)
		if err != nil {
			msg := err.Error()
			_ = m.Bots.UpdateActual(ctx, id, store.BotActualPatch{
				ActualState: store.ActualFailed,
				LastError:   &msg,
			})
			m.log.Warn("start instance", "bot_id", id, "err", err)
			continue
		}
		m.reg.byID[id] = newInst
		m.reg.fp[id] = wantFP
		_ = m.Bots.UpdateActual(ctx, id, store.BotActualPatch{
			ActualState: store.ActualRunning,
			LastError:   nil,
		})
		m.log.Info("add instance", "bot_id", id, "mode", bot.RunMode, "port", bot.Port)
	}
	return nil
}

func (m *Manager) stopAll() {
	m.reg.mu.Lock()
	defer m.reg.mu.Unlock()
	for id, inst := range m.reg.byID {
		inst.stop()
		delete(m.reg.byID, id)
		delete(m.reg.fp, id)
	}
}
