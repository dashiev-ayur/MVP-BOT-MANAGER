package scenarios_test

import (
	"context"
	"net/http"
	"testing"

	"mvp-manager/internal/messenger"
	"mvp-manager/internal/runner/scenarios"
	"mvp-manager/internal/store"
)

// fakeChannel записывает последний SendText для проверки ответов сценария.
type fakeChannel struct {
	lastText string
}

func (f *fakeChannel) SendText(_ context.Context, _ int64, text string) error {
	f.lastText = text
	return nil
}

func (f *fakeChannel) RunPolling(context.Context, func(context.Context, messenger.Incoming) error) error {
	return nil
}

func (f *fakeChannel) ServeWebhook(http.ResponseWriter, *http.Request, func(context.Context, messenger.Incoming) error) {
}

var _ messenger.Channel = (*fakeChannel)(nil)

func TestBuiltin_RegistersDefaultAndExtended(t *testing.T) {
	r := scenarios.Builtin()
	if _, ok := r.Get(store.BotTypeDefault); !ok {
		t.Fatal("default не зарегистрирован")
	}
	if _, ok := r.Get(store.BotTypeDefaultExtended); !ok {
		t.Fatal("default_extended не зарегистрирован")
	}
	if _, ok := r.Get(store.BotTypeCustom); ok {
		t.Fatal("custom не должен быть в runner-сценариях")
	}
}

func TestDefaultVsExtended_StartTextDiffers(t *testing.T) {
	ctx := context.Background()
	ch := &fakeChannel{}

	handled, err := scenarios.Default{}.Handle(ctx, ch, messenger.Incoming{ChatID: 1, Text: "/start"})
	if err != nil || !handled {
		t.Fatalf("default /start: handled=%v err=%v", handled, err)
	}
	defaultReply := ch.lastText
	if defaultReply != messenger.DefaultStartReply {
		t.Fatalf("default reply: got %q", defaultReply)
	}

	ch.lastText = ""
	handled, err = scenarios.Extended{}.Handle(ctx, ch, messenger.Incoming{ChatID: 1, Text: "/start"})
	if err != nil || !handled {
		t.Fatalf("extended /start: handled=%v err=%v", handled, err)
	}
	if ch.lastText != scenarios.ExtendedStartReply {
		t.Fatalf("extended reply: got %q want %q", ch.lastText, scenarios.ExtendedStartReply)
	}
	if ch.lastText == defaultReply {
		t.Fatal("default_extended должен отличаться от default по тексту /start")
	}
}

func TestExtended_Ping(t *testing.T) {
	ctx := context.Background()
	ch := &fakeChannel{}
	handled, err := scenarios.Extended{}.Handle(ctx, ch, messenger.Incoming{ChatID: 2, Text: "/ping"})
	if err != nil || !handled {
		t.Fatalf("ping: handled=%v err=%v", handled, err)
	}
	if ch.lastText != scenarios.ExtendedPingReply {
		t.Fatalf("ping reply: got %q", ch.lastText)
	}

	// default не обрабатывает /ping.
	ch.lastText = ""
	handled, err = scenarios.Default{}.Handle(ctx, ch, messenger.Incoming{ChatID: 2, Text: "/ping"})
	if err != nil {
		t.Fatal(err)
	}
	if handled {
		t.Fatal("default не должен обрабатывать /ping")
	}
}
