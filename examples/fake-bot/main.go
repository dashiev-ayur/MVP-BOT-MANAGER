// fake-bot — заглушка custom-бота для Phase 1 E2E.
//
// Читает launch contract (ТЗ §9): PORT, BOT_TOKEN, BOT_MODE, CHANNEL, PUBLIC_URL.
//   - webhook: HTTP на PORT, GET /healthz → 200 ok
//   - polling: живёт до SIGTERM (без HTTP)
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	port := envOr("PORT", "8080")
	mode := envOr("BOT_MODE", "webhook")
	token := os.Getenv("BOT_TOKEN")
	channel := envOr("CHANNEL", "telegram")
	publicURL := os.Getenv("PUBLIC_URL")

	log.Printf("fake-bot start mode=%s port=%s channel=%s token_set=%v public_url=%q",
		mode, port, channel, token != "", publicURL)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch mode {
	case "polling":
		log.Printf("polling: waiting for SIGTERM")
		<-ctx.Done()
		log.Printf("polling: got signal, exit 0")
		return
	case "webhook":
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok\n"))
		})
		srv := &http.Server{
			Addr:              ":" + port,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		}
		go func() {
			log.Printf("webhook: listen on :%s", port)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("http: %v", err)
				os.Exit(1)
			}
		}()
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
		log.Printf("webhook: shutdown")
	default:
		fmt.Fprintf(os.Stderr, "fake-bot: unknown BOT_MODE=%q (webhook|polling)\n", mode)
		os.Exit(2)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
