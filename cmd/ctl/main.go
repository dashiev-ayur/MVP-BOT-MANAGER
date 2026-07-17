// Точка входа CLI ctl: обёртка над операциями store (создание ботов,
// смена desired state и т.п.). Не смешивается с демоном agent.
//
// Phase 1: bots create/start/stop/list, runtimes list.
// Общее состояние с agent — через MEMORY_STORE_PATH (file-backed memory).
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"mvp-manager/internal/config"
	"mvp-manager/internal/launch"
	"mvp-manager/internal/store"
	"mvp-manager/internal/store/memory"
	"mvp-manager/internal/storeopen"
)

// version — строка версии бинарника; позже можно подставлять через -ldflags.
const version = "0.0.0-dev"

// helpText — краткая справка для пользователя CLI.
const helpText = `mvp-manager ctl — CLI управления ботами (обёртка над store)

Использование:
  ctl [-h|--help]
  ctl [-v|--version]
  ctl bots create --name NAME --custom-name CN --port N --token TOKEN \
      --start-command CMD [--workdir DIR] [--channel telegram|max] \
      [--mode webhook|polling] [--artifact PATH]
  ctl bots start <bot-id>
  ctl bots stop <bot-id>
  ctl bots list
  ctl runtimes list

ENV (как у agent): NODE_ID, STORE=memory, MEMORY_STORE_PATH
(по умолчанию .mvp-manager/store.json — общий файл с agent).

Токен в MVP передаётся флагом --token (или ENV BOT_TOKEN как default для --token).
Vault / control-api не используются.
`

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-v", "--version", "version":
			fmt.Printf("ctl %s\n", version)
			return
		case "-h", "--help", "help":
			fmt.Print(helpText)
			return
		}
	}

	if len(os.Args) < 2 {
		fmt.Print(helpText)
		os.Exit(2)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ctl: конфиг: %v\n", err)
		os.Exit(1)
	}

	st, _, err := storeopen.Open(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ctl: store: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	args := os.Args[1:]

	switch args[0] {
	case "bots":
		if err := cmdBots(ctx, cfg, st, args[1:]); err != nil {
			fmt.Fprintf(os.Stderr, "ctl bots: %v\n", err)
			os.Exit(1)
		}
	case "runtimes":
		if err := cmdRuntimes(ctx, st, args[1:]); err != nil {
			fmt.Fprintf(os.Stderr, "ctl runtimes: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "ctl: неизвестная команда %q\n\n%s", args[0], helpText)
		os.Exit(2)
	}
}

func cmdBots(ctx context.Context, cfg config.Config, st *memory.Store, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("нужна подкоманда: create|start|stop|list")
	}
	switch args[0] {
	case "create":
		return botsCreate(ctx, cfg, st, args[1:])
	case "start":
		if len(args) < 2 {
			return fmt.Errorf("usage: ctl bots start <bot-id>")
		}
		return botsStart(ctx, st, args[1])
	case "stop":
		if len(args) < 2 {
			return fmt.Errorf("usage: ctl bots stop <bot-id>")
		}
		return botsStop(ctx, st, args[1])
	case "list":
		return botsList(ctx, st)
	default:
		return fmt.Errorf("неизвестная подкоманда bots %q", args[0])
	}
}

func cmdRuntimes(ctx context.Context, st *memory.Store, args []string) error {
	if len(args) == 0 || args[0] != "list" {
		return fmt.Errorf("usage: ctl runtimes list")
	}
	return runtimesList(ctx, st)
}

func botsCreate(ctx context.Context, cfg config.Config, st *memory.Store, args []string) error {
	fs := map[string]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "--") {
			return fmt.Errorf("неожиданный аргумент %q", a)
		}
		key := strings.TrimPrefix(a, "--")
		if i+1 >= len(args) {
			return fmt.Errorf("флаг --%s требует значение", key)
		}
		i++
		fs[key] = args[i]
	}

	name := fs["name"]
	customName := fs["custom-name"]
	startCmd := fs["start-command"]
	token := fs["token"]
	if token == "" {
		token = strings.TrimSpace(os.Getenv("BOT_TOKEN"))
	}
	workdir := fs["workdir"]
	artifact := fs["artifact"]
	channel := fs["channel"]
	if channel == "" {
		channel = string(store.BotChannelTelegram)
	}
	mode := fs["mode"]
	if mode == "" {
		mode = string(store.BotRunModeWebhook)
	}
	portStr := fs["port"]
	if portStr == "" {
		return fmt.Errorf("--port обязателен")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("--port: %w", err)
	}

	if err := launch.ValidateCustomCreate(name, customName, startCmd, port, token); err != nil {
		return err
	}

	nodeID := cfg.NodeID
	rtName := "custom-" + customName

	var wd *string
	if workdir != "" {
		wd = &workdir
	} else if artifact != "" {
		wd = &artifact
	}

	rt, err := st.Runtimes.Create(ctx, store.Runtime{
		Kind:           store.RuntimeKindCustomBot,
		Name:           rtName,
		StartCommand:   startCmd,
		Workdir:        wd,
		DesiredState:   store.DesiredStopped,
		ActualState:    store.ActualUnknown,
		AssignedNodeID: &nodeID,
	})
	if err != nil {
		return fmt.Errorf("create runtime: %w", err)
	}

	cn := customName
	var art *string
	if artifact != "" {
		art = &artifact
	}
	sc := startCmd
	bot, err := st.Bots.Create(ctx, store.Bot{
		Name:           name,
		BotType:        store.BotTypeCustom,
		CustomName:     &cn,
		Channel:        store.BotChannel(channel),
		RunMode:        store.BotRunMode(mode),
		Port:           port,
		TokenRef:       token,
		RuntimeID:      &rt.ID,
		ArtifactPath:   art,
		StartCommand:   &sc,
		DesiredState:   store.DesiredStopped,
		ActualState:    store.ActualUnknown,
		AssignedNodeID: &nodeID,
	})
	if err != nil {
		// Runtime уже создан; полного Delete runtime в API нет — сообщаем id.
		return fmt.Errorf("create bot (runtime %s уже создан): %w", rt.ID, err)
	}

	fmt.Printf("created bot_id=%s runtime_id=%s port=%d custom_name=%s\n",
		bot.ID, rt.ID, bot.Port, customName)
	return nil
}

func botsStart(ctx context.Context, st *memory.Store, botID string) error {
	bot, err := st.Bots.ByID(ctx, botID)
	if err != nil {
		return err
	}
	if bot.RuntimeID == nil {
		return fmt.Errorf("bot %s: нет runtime_id", botID)
	}
	rtID := *bot.RuntimeID

	if err := st.Bots.UpdateDesiredState(ctx, botID, store.DesiredRunning); err != nil {
		return err
	}
	if err := st.Runtimes.UpdateDesiredState(ctx, rtID, store.DesiredRunning); err != nil {
		return err
	}

	// Сброс failed/stopped → unknown, чтобы reconcile снова сделал Start
	// (без авто-рестарта сразу после краша).
	rt, err := st.Runtimes.ByID(ctx, rtID)
	if err != nil {
		return err
	}
	if rt.ActualState == store.ActualFailed || rt.ActualState == store.ActualStopped {
		_ = st.Runtimes.UpdateActual(ctx, rtID, store.RuntimeActualPatch{
			ActualState: store.ActualUnknown,
		})
		_ = st.Bots.UpdateActual(ctx, botID, store.BotActualPatch{
			ActualState: store.ActualUnknown,
		})
	}

	fmt.Printf("desired=running bot_id=%s runtime_id=%s\n", botID, rtID)
	return nil
}

func botsStop(ctx context.Context, st *memory.Store, botID string) error {
	bot, err := st.Bots.ByID(ctx, botID)
	if err != nil {
		return err
	}
	if bot.RuntimeID == nil {
		return fmt.Errorf("bot %s: нет runtime_id", botID)
	}
	rtID := *bot.RuntimeID

	if err := st.Bots.UpdateDesiredState(ctx, botID, store.DesiredStopped); err != nil {
		return err
	}
	if err := st.Runtimes.UpdateDesiredState(ctx, rtID, store.DesiredStopped); err != nil {
		return err
	}
	fmt.Printf("desired=stopped bot_id=%s runtime_id=%s\n", botID, rtID)
	return nil
}

func botsList(ctx context.Context, st *memory.Store) error {
	bots, err := st.Bots.List(ctx)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tTYPE\tPORT\tDESIRED\tACTUAL\tRUNTIME_ID")
	for _, b := range bots {
		rt := ""
		if b.RuntimeID != nil {
			rt = *b.RuntimeID
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\t%s\n",
			b.ID, b.Name, b.BotType, b.Port, b.DesiredState, b.ActualState, rt)
	}
	return w.Flush()
}

func runtimesList(ctx context.Context, st *memory.Store) error {
	runtimes, err := st.Runtimes.List(ctx)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tKIND\tDESIRED\tACTUAL\tPID\tNODE")
	for _, rt := range runtimes {
		pid := ""
		if rt.PID != nil {
			pid = strconv.Itoa(*rt.PID)
		}
		node := ""
		if rt.AssignedNodeID != nil {
			node = *rt.AssignedNodeID
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			rt.ID, rt.Name, rt.Kind, rt.DesiredState, rt.ActualState, pid, node)
	}
	return w.Flush()
}
