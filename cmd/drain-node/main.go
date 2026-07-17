// Точка входа drain-node: увести ботов с ноды перед ребутом/обслуживанием.
//
// По умолчанию: status=draining + desired=stopped всем ботам ноды.
// С --to-node: migrate каждого бота на целевую ноду (ops.Migrate).
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"mvp-manager/internal/config"
	"mvp-manager/internal/ops"
	"mvp-manager/internal/store"
	"mvp-manager/internal/storeopen"
)

const version = "0.0.0-dev"

const helpText = `mvp-manager drain-node — увести ботов с ноды

Использование:
  drain-node [-h|--help]
  drain-node [-v|--version]
  drain-node [--node NODE_ID] [--to-node TARGET]
  drain-node [--node NODE_ID]          # stop всех ботов, status=draining

ENV: NODE_ID (нода по умолчанию), STORE, MEMORY_STORE_PATH, BOT_RUNNER_COMMAND
(для migrate default*), MAX_BOTS_PER_NODE (проверка на --to-node).
`

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-v", "--version", "version":
			fmt.Printf("drain-node %s\n", version)
			return
		case "-h", "--help", "help":
			fmt.Print(helpText)
			return
		}
	}

	nodeID := ""
	toNode := ""
	for i := 1; i < len(os.Args); i++ {
		a := os.Args[i]
		switch a {
		case "--node":
			if i+1 >= len(os.Args) {
				fatal("--node требует значение")
			}
			i++
			nodeID = os.Args[i]
		case "--to-node", "--to":
			if i+1 >= len(os.Args) {
				fatal("%s требует значение", a)
			}
			i++
			toNode = os.Args[i]
		default:
			fatal("неизвестный аргумент %q", a)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		fatal("конфиг: %v", err)
	}
	if nodeID == "" {
		nodeID = cfg.NodeID
	}

	st, _, err := storeopen.Open(cfg)
	if err != nil {
		fatal("store: %v", err)
	}
	repos := ops.Repos{
		Nodes: st.Nodes, Runtimes: st.Runtimes, Bots: st.Bots, Events: st.Events,
	}
	ctx := context.Background()

	// 1) Статус draining (даже если ноды ещё нет — Upsert).
	n, err := st.Nodes.ByID(ctx, nodeID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			hostname := nodeID
			n, err = st.Nodes.Upsert(ctx, store.Node{
				ID: nodeID, Hostname: hostname, Status: store.NodeStatusDraining,
			})
			if err != nil {
				fatal("upsert draining: %v", err)
			}
		} else {
			fatal("node: %v", err)
		}
	} else {
		n.Status = store.NodeStatusDraining
		n, err = st.Nodes.Upsert(ctx, n)
		if err != nil {
			fatal("set draining: %v", err)
		}
	}
	fmt.Printf("node %s → status=%s\n", n.ID, n.Status)

	bots, err := st.Bots.ListByNode(ctx, nodeID)
	if err != nil {
		fatal("list bots: %v", err)
	}
	fmt.Printf("bots on node: %d\n", len(bots))

	if toNode != "" {
		if toNode == nodeID {
			fatal("--to-node совпадает с исходной нодой")
		}
		opt := ops.MigrateOptsFromConfig(cfg, toNode)
		opt.WaitTimeout = 45 * time.Second
		for _, b := range bots {
			fmt.Printf("migrate bot_id=%s → %s ...\n", b.ID, toNode)
			if err := ops.Migrate(ctx, repos, b.ID, opt); err != nil {
				fmt.Fprintf(os.Stderr, "  FAIL %s: %v\n", b.ID, err)
				os.Exit(1)
			}
			fmt.Printf("  ok bot_id=%s\n", b.ID)
		}
		fmt.Printf("drain-node done: migrated %d bot(s) to %s\n", len(bots), toNode)
		return
	}

	// Stop всех ботов (desired=stopped).
	for _, b := range bots {
		if b.DesiredState == store.DesiredStopped {
			fmt.Printf("skip already stopped bot_id=%s\n", b.ID)
			continue
		}
		fmt.Printf("stop bot_id=%s ...\n", b.ID)
		if err := ops.Stop(ctx, repos, b.ID); err != nil {
			fmt.Fprintf(os.Stderr, "  FAIL %s: %v\n", b.ID, err)
			os.Exit(1)
		}
		fmt.Printf("  desired=stopped bot_id=%s\n", b.ID)
	}
	fmt.Printf("drain-node done: stopped %d bot(s), node draining\n", len(bots))
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "drain-node: "+format+"\n", args...)
	os.Exit(1)
}
