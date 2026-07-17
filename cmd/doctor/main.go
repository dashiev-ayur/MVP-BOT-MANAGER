// Точка входа doctor: сверка портов, PID, lease, orphan-процессов (best-effort).
// Читаемый отчёт в stdout. Не меняет store.
package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"mvp-manager/internal/config"
	"mvp-manager/internal/store"
	"mvp-manager/internal/storeopen"
)

const version = "0.0.0-dev"

const helpText = `mvp-manager doctor — диагностика состояния ноды / store

Использование:
  doctor [-h|--help]
  doctor [-v|--version]
  doctor [--node NODE_ID]

ENV: NODE_ID, STORE, MEMORY_STORE_PATH (как у agent/ctl).
Без --node используется NODE_ID из ENV.
`

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-v", "--version", "version":
			fmt.Printf("doctor %s\n", version)
			return
		case "-h", "--help", "help":
			fmt.Print(helpText)
			return
		}
	}

	nodeFilter := ""
	for i := 1; i < len(os.Args); i++ {
		a := os.Args[i]
		if a == "--node" {
			if i+1 >= len(os.Args) {
				fmt.Fprintln(os.Stderr, "doctor: --node требует значение")
				os.Exit(2)
			}
			i++
			nodeFilter = os.Args[i]
			continue
		}
		fmt.Fprintf(os.Stderr, "doctor: неизвестный аргумент %q\n", a)
		os.Exit(2)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "doctor: конфиг: %v\n", err)
		os.Exit(1)
	}
	if nodeFilter == "" {
		nodeFilter = cfg.NodeID
	}

	st, storeKind, err := storeopen.Open(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "doctor: store: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	issues := 0

	fmt.Println("=== mvp-manager doctor ===")
	fmt.Printf("store=%s path=%s focus_node=%s\n", storeKind, cfg.MemoryStorePath, nodeFilter)
	fmt.Println()

	// --- Nodes ---
	nodes, err := st.Nodes.List(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "doctor: list nodes: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("## Nodes")
	foundNode := false
	for _, n := range nodes {
		age := ""
		if !n.LastSeenAt.IsZero() {
			age = time.Since(n.LastSeenAt).Truncate(time.Second).String()
		}
		mark := ""
		if n.ID == nodeFilter {
			foundNode = true
			mark = " ← focus"
		}
		fmt.Printf("  %s  status=%s  hostname=%s  last_seen_age=%s%s\n",
			n.ID, n.Status, n.Hostname, age, mark)
	}
	if !foundNode {
		fmt.Printf("  WARN: нода %s не найдена в store\n", nodeFilter)
		issues++
	}
	fmt.Println()

	// --- Ports ---
	bots, err := st.Bots.List(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "doctor: list bots: %v\n", err)
		os.Exit(1)
	}
	portOwners := map[int][]string{}
	for _, b := range bots {
		portOwners[b.Port] = append(portOwners[b.Port], b.ID+"/"+b.Name)
	}
	fmt.Println("## Ports (уникальность в store)")
	dupPorts := 0
	for port, owners := range portOwners {
		if len(owners) > 1 {
			fmt.Printf("  CONFLICT port=%d owners=%s\n", port, strings.Join(owners, ", "))
			dupPorts++
			issues++
		}
	}
	if dupPorts == 0 {
		fmt.Printf("  ok: %d уникальных портов\n", len(portOwners))
	}
	fmt.Println()

	// --- Runtimes / PID / lease ---
	runtimes, err := st.Runtimes.List(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "doctor: list runtimes: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("## Runtimes (PID жив? lease)")
	now := time.Now().UTC()
	trackedPIDs := map[int]string{}
	for _, rt := range runtimes {
		if rt.AssignedNodeID != nil && *rt.AssignedNodeID != nodeFilter && nodeFilter != "" {
			continue
		}
		pidAlive := "n/a"
		if rt.PID != nil {
			trackedPIDs[*rt.PID] = rt.ID
			if processAlive(*rt.PID) {
				pidAlive = "alive"
			} else {
				pidAlive = "DEAD"
				if rt.ActualState == store.ActualRunning {
					fmt.Printf("  WARN runtime=%s actual=running но PID %d мёртв\n", rt.ID, *rt.PID)
					issues++
				}
			}
		}
		leaseInfo := "none"
		if rt.LeaseOwner != nil {
			until := ""
			valid := false
			if rt.LeaseUntil != nil {
				until = rt.LeaseUntil.Format(time.RFC3339)
				valid = rt.LeaseUntil.After(now)
			}
			leaseInfo = fmt.Sprintf("owner=%s until=%s valid=%v", *rt.LeaseOwner, until, valid)
		}
		pidStr := "-"
		if rt.PID != nil {
			pidStr = strconv.Itoa(*rt.PID)
		}
		fmt.Printf("  %s  kind=%s  desired=%s  actual=%s  pid=%s(%s)  lease=%s\n",
			rt.ID, rt.Kind, rt.DesiredState, rt.ActualState, pidStr, pidAlive, leaseInfo)
	}
	fmt.Println()

	// --- Orphan listen ports (best-effort): порт в store занят чужим процессом ---
	fmt.Println("## Listen check (best-effort)")
	for _, b := range bots {
		if b.AssignedNodeID != nil && *b.AssignedNodeID != nodeFilter {
			continue
		}
		if b.DesiredState != store.DesiredRunning && b.ActualState != store.ActualRunning {
			continue
		}
		addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(b.Port))
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err != nil {
			if b.ActualState == store.ActualRunning {
				fmt.Printf("  WARN bot=%s port=%d actual=running но :%d не отвечает\n", b.ID, b.Port, b.Port)
				issues++
			}
			continue
		}
		_ = conn.Close()
		fmt.Printf("  ok bot=%s port=%d listening\n", b.ID, b.Port)
	}
	fmt.Println()

	// --- Orphan PIDs hint ---
	fmt.Println("## Orphan hint")
	fmt.Printf("  tracked PIDs on focus: %d (сверка с ps — вручную при подозрении)\n", len(trackedPIDs))
	fmt.Println()

	fmt.Println("## Summary")
	if issues == 0 {
		fmt.Println("  issues=0  OK")
		return
	}
	fmt.Printf("  issues=%d  NEEDS_ATTENTION\n", issues)
	os.Exit(1)
}

// processAlive — best-effort: signal 0 (без реальной доставки).
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil
}
