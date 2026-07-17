// Package health — логика опроса /healthz webhook-ботов для cmd/healthcheck.
//
// Пакет только читает/пишет store. Процессы не рестартует (это делает agent).
package health

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"mvp-manager/internal/launch"
	"mvp-manager/internal/store"
)

// ErrPrefix — см. launch.HealthcheckErrPrefix (агент реагирует на тот же маркер).
const ErrPrefix = launch.HealthcheckErrPrefix

// Checker периодически опрашивает webhook-ботов и пишет статус в store.
type Checker struct {
	NodeID   string
	AllNodes bool

	Bots store.BotRepository

	Interval         time.Duration
	HTTPTimeout      time.Duration
	FailureThreshold int

	client *http.Client
	log    *slog.Logger

	// failures — счётчик подряд идущих фейлов по bot_id (in-memory процесса).
	failures map[string]int
}

// New создаёт Checker с дефолтами интервала/порога.
func New(nodeID string, bots store.BotRepository) *Checker {
	return &Checker{
		NodeID:           nodeID,
		Bots:             bots,
		Interval:         10 * time.Second,
		HTTPTimeout:      2 * time.Second,
		FailureThreshold: 3,
		log:              slog.Default(),
		failures:         make(map[string]int),
	}
}

// Run крутит проверки до отмены ctx.
func (c *Checker) Run(ctx context.Context) error {
	if c.Interval <= 0 {
		c.Interval = 10 * time.Second
	}
	if c.HTTPTimeout <= 0 {
		c.HTTPTimeout = 2 * time.Second
	}
	if c.FailureThreshold <= 0 {
		c.FailureThreshold = 3
	}
	if c.client == nil {
		c.client = &http.Client{Timeout: c.HTTPTimeout}
	}
	if c.log == nil {
		c.log = slog.Default()
	}
	if c.failures == nil {
		c.failures = make(map[string]int)
	}

	ticker := time.NewTicker(c.Interval)
	defer ticker.Stop()

	if err := c.Tick(ctx); err != nil {
		c.log.Warn("healthcheck tick", "err", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := c.Tick(ctx); err != nil {
				c.log.Warn("healthcheck tick", "err", err)
			}
		}
	}
}

// Tick — один проход опроса.
//
// Кандидаты: desired=running, run_mode=webhook; нода = NODE_ID или все (AllNodes).
// При серии фейлов: actual_state=failed, last_error с префиксом healthcheck:.
func (c *Checker) Tick(ctx context.Context) error {
	if c.HTTPTimeout <= 0 {
		c.HTTPTimeout = 2 * time.Second
	}
	if c.FailureThreshold <= 0 {
		c.FailureThreshold = 3
	}
	if c.client == nil {
		c.client = &http.Client{Timeout: c.HTTPTimeout}
	}
	if c.failures == nil {
		c.failures = make(map[string]int)
	}
	if c.log == nil {
		c.log = slog.Default()
	}

	var bots []store.Bot
	var err error
	if c.AllNodes {
		bots, err = c.Bots.List(ctx)
	} else {
		bots, err = c.Bots.ListByNode(ctx, c.NodeID)
	}
	if err != nil {
		return fmt.Errorf("list bots: %w", err)
	}

	var first error
	for _, b := range bots {
		if b.DesiredState != store.DesiredRunning {
			delete(c.failures, b.ID)
			continue
		}
		if b.RunMode != store.BotRunModeWebhook {
			continue
		}
		// Custom тоже может иметь webhook /healthz — опрашиваем всех webhook.
		if err := c.checkOne(ctx, b); err != nil {
			c.log.Warn("check bot", "bot_id", b.ID, "err", err)
			if first == nil {
				first = err
			}
		}
	}
	return first
}

func (c *Checker) checkOne(ctx context.Context, b store.Bot) error {
	url := launch.HealthzURL(b.Port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return c.onFailure(ctx, b, err.Error())
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))

	if resp.StatusCode != http.StatusOK {
		return c.onFailure(ctx, b, fmt.Sprintf("status %d", resp.StatusCode))
	}

	// Успех — сбрасываем счётчик; если ранее был healthcheck-failed, вернём running.
	c.failures[b.ID] = 0
	if b.ActualState == store.ActualFailed && b.LastError != nil && strings.HasPrefix(*b.LastError, ErrPrefix) {
		if err := c.Bots.UpdateActual(ctx, b.ID, store.BotActualPatch{
			ActualState: store.ActualRunning,
			LastError:   nil,
		}); err != nil {
			return err
		}
		c.log.Info("health restored", "bot_id", b.ID, "port", b.Port)
	}
	return nil
}

func (c *Checker) onFailure(ctx context.Context, b store.Bot, reason string) error {
	c.failures[b.ID]++
	n := c.failures[b.ID]
	c.log.Warn("healthz fail", "bot_id", b.ID, "port", b.Port, "n", n, "reason", reason)

	if n < c.FailureThreshold {
		return nil
	}

	msg := fmt.Sprintf("%s %d consecutive failures: %s", ErrPrefix, n, reason)
	if err := c.Bots.UpdateActual(ctx, b.ID, store.BotActualPatch{
		ActualState: store.ActualFailed,
		LastError:   &msg,
	}); err != nil {
		return fmt.Errorf("mark unhealthy %s: %w", b.ID, err)
	}
	c.log.Info("bot marked unhealthy", "bot_id", b.ID, "last_error", msg)
	return nil
}
