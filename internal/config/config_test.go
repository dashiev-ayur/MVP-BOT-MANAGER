package config_test

import (
	"strings"
	"testing"

	"mvp-manager/internal/config"
)

// TestLoad_HappyPathMemory проверяет успешную загрузку с STORE=memory и NODE_ID.
func TestLoad_HappyPathMemory(t *testing.T) {
	t.Setenv(config.EnvNodeID, "node-1")
	t.Setenv(config.EnvStore, config.StoreMemory)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() неожиданная ошибка: %v", err)
	}
	if cfg.NodeID != "node-1" {
		t.Fatalf("NodeID: got %q, want %q", cfg.NodeID, "node-1")
	}
	if cfg.Store != config.StoreMemory {
		t.Fatalf("Store: got %q, want %q", cfg.Store, config.StoreMemory)
	}
}

// TestLoad_DefaultStoreMemory проверяет, что пустой/отсутствующий STORE
// даёт значение по умолчанию memory.
func TestLoad_DefaultStoreMemory(t *testing.T) {
	t.Setenv(config.EnvNodeID, "local")
	// Пустая строка имитирует «не задано» после TrimSpace; t.Setenv("")
	// гарантированно переопределяет возможное значение из окружения CI.
	t.Setenv(config.EnvStore, "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() неожиданная ошибка: %v", err)
	}
	if cfg.Store != config.DefaultStore {
		t.Fatalf("Store по умолчанию: got %q, want %q", cfg.Store, config.DefaultStore)
	}
}

// TestLoad_UnknownStore проверяет понятную ошибку без паники при STORE=redis.
func TestLoad_UnknownStore(t *testing.T) {
	t.Setenv(config.EnvNodeID, "node-1")
	t.Setenv(config.EnvStore, "redis")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() ожидалась ошибка для неизвестного STORE, got nil")
	}

	msg := err.Error()
	// Сообщение должно подсказывать допустимые значения (memory / postgres).
	if !strings.Contains(msg, "redis") {
		t.Fatalf("ошибка должна упоминать неизвестное значение: %q", msg)
	}
	if !strings.Contains(msg, config.StoreMemory) {
		t.Fatalf("ошибка должна упоминать %q: %q", config.StoreMemory, msg)
	}
	if !strings.Contains(msg, config.StorePostgres) {
		t.Fatalf("ошибка должна упоминать %q: %q", config.StorePostgres, msg)
	}
}

// TestLoad_MissingNodeID проверяет ошибку при пустом NODE_ID.
func TestLoad_MissingNodeID(t *testing.T) {
	t.Setenv(config.EnvNodeID, "")
	t.Setenv(config.EnvStore, config.StoreMemory)

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() ожидалась ошибка при пустом NODE_ID, got nil")
	}
	if !strings.Contains(err.Error(), config.EnvNodeID) {
		t.Fatalf("ошибка должна упоминать %s: %q", config.EnvNodeID, err.Error())
	}
}

// TestLoad_PostgresAcceptedAsKnownStore проверяет, что STORE=postgres
// принимается конфигом (wiring БД — Phase PG, здесь только валидация значения).
func TestLoad_PostgresAcceptedAsKnownStore(t *testing.T) {
	t.Setenv(config.EnvNodeID, "node-pg")
	t.Setenv(config.EnvStore, config.StorePostgres)
	t.Setenv(config.EnvDatabaseURL, "postgres://user:pass@localhost:5432/mvp?sslmode=disable")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() неожиданная ошибка: %v", err)
	}
	if cfg.Store != config.StorePostgres {
		t.Fatalf("Store: got %q, want %q", cfg.Store, config.StorePostgres)
	}
	if cfg.DatabaseURL == "" {
		t.Fatal("DatabaseURL должен быть прочитан из ENV")
	}
}
