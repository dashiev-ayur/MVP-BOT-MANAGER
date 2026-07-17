// Точка входа CLI ctl: обёртка над операциями store (создание ботов,
// смена desired state и т.п.). Не смешивается с демоном agent.
//
// Phase 1: custom bots; Phase 2: + default (привязка к bot_runner).
// Общее состояние с agent/runner/healthcheck — через MEMORY_STORE_PATH.
package main

import (
	"context"
	"errors"
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
  ctl bots create --type custom --name NAME --custom-name CN --port N --token TOKEN \
      --start-command CMD [--workdir DIR] [--channel telegram|max] \
      [--mode webhook|polling] [--artifact PATH]
  ctl bots create --type default --name NAME --port N --token TOKEN \
      [--mode webhook|polling] [--channel telegram|max]
  ctl bots start <bot-id>
  ctl bots stop <bot-id>
  ctl bots list
  ctl runtimes list
  ctl runtimes start <runtime-id>
  ctl runtimes stop <runtime-id>

ENV (как у agent): NODE_ID, STORE=memory, MEMORY_STORE_PATH
(по умолчанию .mvp-manager/store.json — общий файл с agent/runner/healthcheck).

Для default create нужен (или будет создан) runtime bot-runner-<NODE_ID>;
команду запуска runner задаёт agent через BOT_RUNNER_COMMAND.

Токен в MVP передаётся флагом --token (или ENV BOT_TOKEN как default для --token).
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
		if err := cmdRuntimes(ctx, cfg, st, args[1:]); err != nil {
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

func cmdRuntimes(ctx context.Context, cfg config.Config, st *memory.Store, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ctl runtimes list|start|stop")
	}
	switch args[0] {
	case "list":
		return runtimesList(ctx, st)
	case "start":
		if len(args) < 2 {
			return fmt.Errorf("usage: ctl runtimes start <runtime-id>")
		}
		return runtimeSetDesired(ctx, st, args[1], store.DesiredRunning)
	case "stop":
		if len(args) < 2 {
			return fmt.Errorf("usage: ctl runtimes stop <runtime-id>")
		}
		return runtimeSetDesired(ctx, st, args[1], store.DesiredStopped)
	default:
		return fmt.Errorf("неизвестная подкоманда runtimes %q", args[0])
	}
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

	botType := fs["type"]
	if botType == "" {
		botType = fs["bot-type"]
	}
	// Обратная совместимость Phase 1: без --type, но с --custom-name → custom.
	if botType == "" {
		if fs["custom-name"] != "" {
			botType = string(store.BotTypeCustom)
		} else {
			return fmt.Errorf("--type обязателен (custom|default|default_extended)")
		}
	}

	switch store.BotType(botType) {
	case store.BotTypeCustom:
		return botsCreateCustom(ctx, cfg, st, fs)
	case store.BotTypeDefault, store.BotTypeDefaultExtended:
		return botsCreateDefault(ctx, cfg, st, fs, store.BotType(botType))
	default:
		return fmt.Errorf("неизвестный --type %q", botType)
	}
}

func botsCreateCustom(ctx context.Context, cfg config.Config, st *memory.Store, fs map[string]string) error {
	name := fs["name"]
	customName := fs["custom-name"]
	startCmd := fs["start-command"]
	token := tokenFromFlags(fs)
	workdir := fs["workdir"]
	artifact := fs["artifact"]
	channel, mode, port, err := channelModePort(fs)
	if err != nil {
		return err
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
		return fmt.Errorf("create bot (runtime %s уже создан): %w", rt.ID, err)
	}

	fmt.Printf("created bot_id=%s runtime_id=%s port=%d custom_name=%s type=custom\n",
		bot.ID, rt.ID, bot.Port, customName)
	return nil
}

func botsCreateDefault(ctx context.Context, cfg config.Config, st *memory.Store, fs map[string]string, botType store.BotType) error {
	name := fs["name"]
	token := tokenFromFlags(fs)
	channel, mode, port, err := channelModePort(fs)
	if err != nil {
		return err
	}
	if err := launch.ValidateDefaultCreate(name, port, token); err != nil {
		return err
	}
	if fs["custom-name"] != "" {
		return fmt.Errorf("custom-name недопустим для type=%s", botType)
	}
	if fs["start-command"] != "" {
		return fmt.Errorf("start-command недопустим для type=%s (команда у bot-runner)", botType)
	}

	nodeID := cfg.NodeID
	rt, err := getOrCreateBotRunnerRuntime(ctx, cfg, st, nodeID)
	if err != nil {
		return err
	}

	bot, err := st.Bots.Create(ctx, store.Bot{
		Name:           name,
		BotType:        botType,
		CustomName:     nil,
		Channel:        store.BotChannel(channel),
		RunMode:        store.BotRunMode(mode),
		Port:           port,
		TokenRef:       token,
		RuntimeID:      &rt.ID,
		DesiredState:   store.DesiredStopped,
		ActualState:    store.ActualUnknown,
		AssignedNodeID: &nodeID,
	})
	if err != nil {
		return fmt.Errorf("create bot: %w", err)
	}

	fmt.Printf("created bot_id=%s runtime_id=%s port=%d type=%s\n",
		bot.ID, rt.ID, bot.Port, botType)
	return nil
}

// getOrCreateBotRunnerRuntime — runtime kind=bot_runner для NODE_ID.
// StartCommand может быть пустым до первого старта agent (подставит из ENV);
// если BOT_RUNNER_COMMAND задан в окружении ctl — записываем сразу.
func getOrCreateBotRunnerRuntime(ctx context.Context, cfg config.Config, st *memory.Store, nodeID string) (store.Runtime, error) {
	name := launch.BotRunnerRuntimeName(nodeID)
	rt, err := st.Runtimes.ByName(ctx, name)
	if err == nil {
		// Обновим команду, если в ENV ctl она есть, а в store пусто/устарела.
		if cfg.BotRunnerCommand != "" && rt.StartCommand != cfg.BotRunnerCommand {
			rt.StartCommand = cfg.BotRunnerCommand
			if cfg.BotRunnerWorkdir != "" {
				wd := cfg.BotRunnerWorkdir
				rt.Workdir = &wd
			}
			return st.Runtimes.Update(ctx, rt)
		}
		return rt, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return store.Runtime{}, err
	}

	cmd := cfg.BotRunnerCommand
	if cmd == "" {
		// Плейсхолдер: agent при ensure подставит/проверит BOT_RUNNER_COMMAND.
		cmd = "bot-runner"
	}
	var wd *string
	if cfg.BotRunnerWorkdir != "" {
		w := cfg.BotRunnerWorkdir
		wd = &w
	}
	return st.Runtimes.Create(ctx, store.Runtime{
		Kind:           store.RuntimeKindBotRunner,
		Name:           name,
		StartCommand:   cmd,
		Workdir:        wd,
		DesiredState:   store.DesiredStopped,
		ActualState:    store.ActualUnknown,
		AssignedNodeID: &nodeID,
	})
}

func tokenFromFlags(fs map[string]string) string {
	token := fs["token"]
	if token == "" {
		token = strings.TrimSpace(os.Getenv("BOT_TOKEN"))
	}
	return token
}

func channelModePort(fs map[string]string) (channel, mode string, port int, err error) {
	channel = fs["channel"]
	if channel == "" {
		channel = string(store.BotChannelTelegram)
	}
	mode = fs["mode"]
	if mode == "" {
		mode = string(store.BotRunModeWebhook)
	}
	portStr := fs["port"]
	if portStr == "" {
		return "", "", 0, fmt.Errorf("--port обязателен")
	}
	port, err = strconv.Atoi(portStr)
	if err != nil {
		return "", "", 0, fmt.Errorf("--port: %w", err)
	}
	return channel, mode, port, nil
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

	rt, err := st.Runtimes.ByID(ctx, rtID)
	if err != nil {
		return err
	}
	// Для custom — сброс failed как в Phase 1.
	// Для bot_runner — тоже, чтобы reconcile снова поднял процесс.
	if rt.ActualState == store.ActualFailed || rt.ActualState == store.ActualStopped {
		_ = st.Runtimes.UpdateActual(ctx, rtID, store.RuntimeActualPatch{
			ActualState: store.ActualUnknown,
		})
	}
	if bot.ActualState == store.ActualFailed || bot.ActualState == store.ActualStopped {
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

	rt, err := st.Runtimes.ByID(ctx, rtID)
	if err != nil {
		return err
	}

	// custom 1:1 — стопаем runtime вместе с ботом.
	// bot_runner — стопаем runtime только если больше нет desired=running default*.
	if rt.Kind == store.RuntimeKindCustomBot {
		if err := st.Runtimes.UpdateDesiredState(ctx, rtID, store.DesiredStopped); err != nil {
			return err
		}
	} else if rt.Kind == store.RuntimeKindBotRunner {
		still, err := anyDefaultRunning(ctx, st, rtID, botID)
		if err != nil {
			return err
		}
		if !still {
			if err := st.Runtimes.UpdateDesiredState(ctx, rtID, store.DesiredStopped); err != nil {
				return err
			}
		}
	}

	fmt.Printf("desired=stopped bot_id=%s runtime_id=%s\n", botID, rtID)
	return nil
}

func anyDefaultRunning(ctx context.Context, st *memory.Store, runtimeID, exceptBotID string) (bool, error) {
	bots, err := st.Bots.ListByRuntime(ctx, runtimeID)
	if err != nil {
		return false, err
	}
	for _, b := range bots {
		if b.ID == exceptBotID {
			continue
		}
		if launch.IsDefaultType(b.BotType) && b.DesiredState == store.DesiredRunning {
			return true, nil
		}
	}
	return false, nil
}

func runtimeSetDesired(ctx context.Context, st *memory.Store, runtimeID string, desired store.DesiredState) error {
	rt, err := st.Runtimes.ByID(ctx, runtimeID)
	if err != nil {
		return err
	}
	if err := st.Runtimes.UpdateDesiredState(ctx, runtimeID, desired); err != nil {
		return err
	}
	if desired == store.DesiredRunning && (rt.ActualState == store.ActualFailed || rt.ActualState == store.ActualStopped) {
		_ = st.Runtimes.UpdateActual(ctx, runtimeID, store.RuntimeActualPatch{
			ActualState: store.ActualUnknown,
		})
	}
	fmt.Printf("runtime desired=%s runtime_id=%s kind=%s\n", desired, runtimeID, rt.Kind)
	return nil
}

func botsList(ctx context.Context, st *memory.Store) error {
	bots, err := st.Bots.List(ctx)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tTYPE\tPORT\tDESIRED\tACTUAL\tRUNTIME_ID\tLAST_ERROR")
	for _, b := range bots {
		rt := ""
		if b.RuntimeID != nil {
			rt = *b.RuntimeID
		}
		lastErr := ""
		if b.LastError != nil {
			lastErr = *b.LastError
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\t%s\t%s\n",
			b.ID, b.Name, b.BotType, b.Port, b.DesiredState, b.ActualState, rt, lastErr)
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
