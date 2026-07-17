package launch_test

import (
	"errors"
	"testing"

	"mvp-manager/internal/launch"
	"mvp-manager/internal/store"
)

func TestResolveTokenRef_Direct(t *testing.T) {
	t.Parallel()
	got, err := launch.ResolveTokenRef("  abc:def-token  ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "abc:def-token" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveTokenRef_EnvPrefix(t *testing.T) {
	t.Setenv("MVP_TEST_BOT_TOKEN", "from-env-value")
	got, err := launch.ResolveTokenRef("env:MVP_TEST_BOT_TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	if got != "from-env-value" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveTokenRef_Dollar(t *testing.T) {
	t.Setenv("MVP_TEST_BOT_TOKEN2", "dollar-val")
	got, err := launch.ResolveTokenRef("$MVP_TEST_BOT_TOKEN2")
	if err != nil {
		t.Fatal(err)
	}
	if got != "dollar-val" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveTokenRef_EnvMissing(t *testing.T) {
	t.Setenv("MVP_TEST_BOT_TOKEN_EMPTY", "")
	_, err := launch.ResolveTokenRef("env:MVP_TEST_BOT_TOKEN_EMPTY")
	if !errors.Is(err, store.ErrInvalidArgument) {
		t.Fatalf("want ErrInvalidArgument, got %v", err)
	}
	_, err = launch.ResolveTokenRef("env:MVP_TEST_BOT_TOKEN_UNSET_XYZ")
	if !errors.Is(err, store.ErrInvalidArgument) {
		t.Fatalf("want ErrInvalidArgument, got %v", err)
	}
}

func TestResolveTokenRef_Empty(t *testing.T) {
	t.Parallel()
	_, err := launch.ResolveTokenRef("  ")
	if !errors.Is(err, store.ErrInvalidArgument) {
		t.Fatalf("want ErrInvalidArgument, got %v", err)
	}
}

func TestTokenHint_NoFullSecret(t *testing.T) {
	t.Parallel()
	token := "123456:ABC-DEF"
	hint := launch.TokenHint(token)
	if hint == token {
		t.Fatal("hint must not equal full token")
	}
	if hint == "" {
		t.Fatal("empty hint")
	}
}

func TestBuildEnv_ResolvesToken(t *testing.T) {
	t.Setenv("MVP_BUILDENV_TOK", "resolved-secret")
	bot := store.Bot{
		Port:     8080,
		TokenRef: "env:MVP_BUILDENV_TOK",
		RunMode:  store.BotRunModeWebhook,
		Channel:  store.BotChannelTelegram,
	}
	env, err := launch.BuildEnv(bot, "https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"PORT":       "8080",
		"BOT_TOKEN":  "resolved-secret",
		"BOT_MODE":   "webhook",
		"CHANNEL":    "telegram",
		"PUBLIC_URL": "https://example.com",
	}
	got := map[string]string{}
	for _, e := range env {
		for k := range want {
			prefix := k + "="
			if len(e) >= len(prefix) && e[:len(prefix)] == prefix {
				got[k] = e[len(prefix):]
			}
		}
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s: want %q got %q", k, v, got[k])
		}
	}
}
