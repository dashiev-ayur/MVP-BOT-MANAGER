package supervisor_test

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"mvp-manager/internal/supervisor"
)

// TestStartAliveStop — критерий Phase 1.2: start → процесс жив → stop → процесса нет.
func TestStartAliveStop(t *testing.T) {
	ctx := context.Background()
	sup := supervisor.New(2 * time.Second)

	// Долгоживущий sleep: достаточно для проверки PID и stop.
	pid, err := sup.Start(ctx, supervisor.Spec{
		ID:      "rt-sleep",
		Command: "sleep 60",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if pid <= 0 {
		t.Fatalf("pid: %d", pid)
	}
	if !sup.IsRunning("rt-sleep") {
		t.Fatal("ожидали running после Start")
	}
	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf("процесс pid=%d не жив: %v", pid, err)
	}

	if err := sup.Stop(ctx, "rt-sleep"); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Даём Wait дописать состояние.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !sup.IsRunning("rt-sleep") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if sup.IsRunning("rt-sleep") {
		t.Fatal("после Stop процесс всё ещё running в supervisor")
	}
	if err := syscall.Kill(pid, 0); err == nil {
		t.Fatalf("OS-процесс pid=%d всё ещё жив после Stop", pid)
	} else if err != syscall.ESRCH {
		// На некоторых системах Kill(0) даёт другой errno для zombie — допускаем «нет процесса».
		if !os.IsPermission(err) {
			// ESRCH ожидаем; Permission маловероятен для своего child.
			t.Logf("Kill(0) после stop: %v (считаем остановленным)", err)
		}
	}
}

// TestCrashRecordsExitCode — краш ребёнка фиксируется в Snapshot (не running + exit code).
func TestCrashRecordsExitCode(t *testing.T) {
	ctx := context.Background()
	sup := supervisor.New(time.Second)

	// false сразу завершается с ненулевым кодом.
	bin := filepath.Join(t.TempDir(), "fail.sh")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 7\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := sup.Start(ctx, supervisor.Spec{
		ID:      "rt-fail",
		Command: bin,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	var snap supervisor.Snapshot
	var ok bool
	for time.Now().Before(deadline) {
		snap, ok = sup.Snapshot("rt-fail")
		if ok && !snap.Running && snap.ExitCode != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ok || snap.Running {
		t.Fatal("ожидали завершение процесса")
	}
	if snap.ExitCode == nil || *snap.ExitCode != 7 {
		t.Fatalf("exit code: %+v", snap.ExitCode)
	}
}
