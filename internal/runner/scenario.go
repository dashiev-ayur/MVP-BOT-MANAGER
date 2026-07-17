package runner

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"mvp-manager/internal/launch"
	"mvp-manager/internal/messenger"
	"mvp-manager/internal/runner/scenarios"
	"mvp-manager/internal/store"
)

// scenarioRegistry — каталог вшитых сценариев (default / default_extended).
// Подменяется в тестах через SetScenarioRegistry; по умолчанию Builtin().
var scenarioRegistry = scenarios.Builtin()

// SetScenarioRegistry подменяет реестр сценариев (для тестов).
func SetScenarioRegistry(r *scenarios.Registry) {
	if r == nil {
		scenarioRegistry = scenarios.Builtin()
		return
	}
	scenarioRegistry = r
}

// instance — один логический бот внутри процесса runner.
//
// webhook: HTTP server на bot.Port с GET /healthz + POST webhook updates;
// polling: long poll канала (порт зарезервирован в store, но не слушается).
type instance struct {
	bot store.Bot

	cancel context.CancelFunc
	done   chan struct{}

	// unhealthy — debug-флаг для E2E: POST /debug/unhealthy ломает /healthz.
	unhealthy atomic.Bool

	srv     *http.Server
	ch      messenger.Channel
	handler scenarios.Handler // сценарий из registry (default / default_extended)
}

// fingerprint — что считать «конфигом инстанса» для reload.
type fingerprint struct {
	configVersion int64
	tokenRef      string
	runMode       store.BotRunMode
	port          int
	botType       store.BotType
	channel       store.BotChannel
}

func fpOf(b store.Bot) fingerprint {
	return fingerprint{
		configVersion: b.ConfigVersion,
		tokenRef:      b.TokenRef,
		runMode:       b.RunMode,
		port:          b.Port,
		botType:       b.BotType,
		channel:       b.Channel,
	}
}

// startInstance поднимает сценарий default* для бота (Telegram или Max).
func startInstance(parent context.Context, bot store.Bot, log *slog.Logger) (*instance, error) {
	ctx, cancel := context.WithCancel(parent)
	inst := &instance{
		bot:    bot,
		cancel: cancel,
		done:   make(chan struct{}),
	}

	// Тип должен быть в реестре сценариев (явная Register, без вечного switch).
	handler, err := scenarioRegistry.MustGet(bot.BotType)
	if err != nil {
		cancel()
		return nil, err
	}
	inst.handler = handler

	token, err := launch.ResolveTokenRef(bot.TokenRef)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("resolve token_ref: %w", err)
	}
	ch, err := messenger.NewChannel(bot.Channel, token, nil, "")
	if err != nil {
		cancel()
		return nil, err
	}
	inst.ch = ch
	log.Info("messenger ready",
		"bot_id", bot.ID,
		"channel", bot.Channel,
		"mode", bot.RunMode,
		"token", launch.TokenHint(token),
	)

	switch bot.RunMode {
	case store.BotRunModeWebhook:
		if err := inst.startWebhook(ctx, log); err != nil {
			cancel()
			return nil, err
		}
	case store.BotRunModePolling:
		inst.startPolling(ctx, log)
	default:
		cancel()
		return nil, fmt.Errorf("unsupported run_mode %q", bot.RunMode)
	}

	return inst, nil
}

// onIncoming делегирует зарегистрированному сценарию (default / default_extended).
func (inst *instance) onIncoming(ctx context.Context, in messenger.Incoming) error {
	if inst.handler == nil {
		return fmt.Errorf("no scenario handler for bot %s", inst.bot.ID)
	}
	_, err := inst.handler.Handle(ctx, inst.ch, in)
	return err
}

// startWebhook слушает уникальный port бота: /healthz + webhook updates.
//
// Важно: setWebhook в Telegram/Max здесь НЕ вызывается — для этого нужен
// публичный HTTPS PUBLIC_URL. Локально можно POST-нуть Update на порт бота
// (best effort) или использовать run_mode=polling.
func (inst *instance) startWebhook(ctx context.Context, log *slog.Logger) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		if inst.unhealthy.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("unhealthy\n"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	// Debug-эндпоинты для E2E Phase 2: «сломать» / восстановить healthz без kill процесса.
	mux.HandleFunc("/debug/unhealthy", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		inst.unhealthy.Store(true)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("unhealthy armed\n"))
	})
	mux.HandleFunc("/debug/healthy", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		inst.unhealthy.Store(false)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("healthy\n"))
	})
	// Приём updates: POST /webhook или POST / (кроме /healthz — отдельный путь).
	webhook := func(w http.ResponseWriter, r *http.Request) {
		inst.ch.ServeWebhook(w, r, inst.onIncoming)
	}
	mux.HandleFunc("/webhook", webhook)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPost {
			webhook(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("default-scenario ready\n"))
	})

	addr := fmt.Sprintf(":%d", inst.bot.Port)
	inst.srv = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("webhook listen", "bot_id", inst.bot.ID, "port", inst.bot.Port, "channel", inst.bot.Channel)
		err := inst.srv.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	// Короткая проверка, что bind не упал сразу (порт занят и т.п.).
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("listen %s: %w", addr, err)
		}
	case <-time.After(100 * time.Millisecond):
		// Сервер ещё работает — нормальный путь.
	case <-ctx.Done():
		return ctx.Err()
	}

	go func() {
		defer close(inst.done)
		select {
		case <-ctx.Done():
			shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = inst.srv.Shutdown(shutCtx)
			<-errCh
		case err := <-errCh:
			if err != nil {
				log.Warn("webhook stopped", "bot_id", inst.bot.ID, "err", err)
			}
		}
	}()
	return nil
}

// startPolling — long poll канала мессенджера (порт только зарезервирован в store).
func (inst *instance) startPolling(ctx context.Context, log *slog.Logger) {
	go func() {
		defer close(inst.done)
		log.Info("polling start", "bot_id", inst.bot.ID, "channel", inst.bot.Channel, "port_reserved", inst.bot.Port)
		err := inst.ch.RunPolling(ctx, inst.onIncoming)
		if err != nil && ctx.Err() == nil {
			log.Warn("polling stopped", "bot_id", inst.bot.ID, "err", err)
			return
		}
		log.Info("polling stopped", "bot_id", inst.bot.ID)
	}()
}

// stop останавливает инстанс и ждёт завершения.
func (inst *instance) stop() {
	if inst.cancel != nil {
		inst.cancel()
	}
	<-inst.done
}

// registry — in-memory карта bot_id → instance.
type registry struct {
	mu   sync.Mutex
	byID map[string]*instance
	fp   map[string]fingerprint
}

func newRegistry() *registry {
	return &registry{
		byID: make(map[string]*instance),
		fp:   make(map[string]fingerprint),
	}
}
