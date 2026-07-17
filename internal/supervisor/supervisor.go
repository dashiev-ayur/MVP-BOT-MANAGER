// Package supervisor управляет OS-процессами custom-ботов (и позже runner).
//
// Жизненный цикл одного процесса (ключ — runtime ID):
//  1. Start: парсинг команды, Setpgid (отдельная process group), Start,
//     учёт PID, фоновый Wait;
//  2. Пока жив: IsRunning / Snapshot отражают актуальный PID;
//  3. Stop: SIGTERM всей группе → grace → SIGKILL → дождаться Wait;
//  4. Краш/выход сам: Wait фиксирует exit_code, процесс снимается с учёта
//     как «завершённый» (reconcile увидит failed при desired=running).
//
// Пакет не знает про store/pgx — только про процессы (DIP).
package supervisor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Spec — параметры запуска процесса (launch contract собирает вызывающий).
type Spec struct {
	// ID — стабильный ключ учёта (обычно runtime.ID).
	ID string
	// Command — строка запуска (shell-подобно: первый токен — бинарник, остальное — args).
	// Для MVP достаточно sh -c при наличии пробелов? Нет: split по полям без shell,
	// чтобы не подмешивать интерпретатор. Сложные команды — один бинарник + args.
	Command string
	// Workdir — каталог процесса; пусто = наследник cwd агента.
	Workdir string
	// Env — полное окружение процесса (обычно os.Environ() + launch vars).
	Env []string
}

// ExitInfo — результат завершения процесса (после Wait).
type ExitInfo struct {
	ExitCode int
	Err      error // Wait error; nil при штатном ExitCode==0
	ExitedAt time.Time
}

// Snapshot — снимок состояния учтённого процесса для reconcile.
type Snapshot struct {
	ID       string
	PID      int
	Running  bool
	ExitCode *int
	WaitErr  error
}

// managed — внутреннее состояние одного процесса под mutex Supervisor.
type managed struct {
	id   string
	cmd  *exec.Cmd
	pid  int
	done chan struct{} // закрывается после Wait

	exitCode *int
	waitErr  error
}

// Supervisor — учёт процессов ноды: Start / Stop / Snapshot / StopAll.
type Supervisor struct {
	mu    sync.Mutex
	procs map[string]*managed
	grace time.Duration // ожидание после SIGTERM перед SIGKILL
}

// New создаёт supervisor. grace <= 0 → 10s по умолчанию.
func New(grace time.Duration) *Supervisor {
	if grace <= 0 {
		grace = 10 * time.Second
	}
	return &Supervisor{
		procs: make(map[string]*managed),
		grace: grace,
	}
}

// Start запускает процесс по Spec и ставит его на учёт.
//
// Если процесс с тем же ID уже running — ошибка (reconcile не должен
// двойной старт; сначала Stop или дождаться exit).
//
// Process group (Setpgid): Stop шлёт сигнал всей группе (−pid), чтобы
// убить потомков, если бот форкнулся.
func (s *Supervisor) Start(ctx context.Context, spec Spec) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("supervisor start: %w", err)
	}
	if strings.TrimSpace(spec.ID) == "" {
		return 0, fmt.Errorf("supervisor start: empty id")
	}
	if strings.TrimSpace(spec.Command) == "" {
		return 0, fmt.Errorf("supervisor start %s: empty command", spec.ID)
	}

	argv, err := splitCommand(spec.Command)
	if err != nil {
		return 0, fmt.Errorf("supervisor start %s: %w", spec.ID, err)
	}

	s.mu.Lock()
	if m, ok := s.procs[spec.ID]; ok {
		// Ещё не дождались Wait — считаем живым, пока done не закрыт и Running.
		select {
		case <-m.done:
			// Завершённый слот можно переиспользовать (новый Start).
			delete(s.procs, spec.ID)
		default:
			s.mu.Unlock()
			return 0, fmt.Errorf("supervisor start %s: already running pid=%d", spec.ID, m.pid)
		}
	}
	s.mu.Unlock()

	cmd := exec.CommandContext(context.Background(), argv[0], argv[1:]...)
	// Не привязываем к ctx отмены reconcile-тика: иначе каждый cancel тика
	// убивал бы детей. Остановка — только Stop / StopAll / краш.
	_ = ctx // проверка отмены уже сделана выше; дальше процесс автономен.
	if spec.Workdir != "" {
		cmd.Dir = spec.Workdir
	}
	if len(spec.Env) > 0 {
		cmd.Env = spec.Env
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("supervisor start %s: %w", spec.ID, err)
	}

	m := &managed{
		id:   spec.ID,
		cmd:  cmd,
		pid:  cmd.Process.Pid,
		done: make(chan struct{}),
	}

	s.mu.Lock()
	// Гонка: параллельный Start того же ID (редко) — убиваем только что стартовавший.
	if existing, ok := s.procs[spec.ID]; ok {
		select {
		case <-existing.done:
			delete(s.procs, spec.ID)
		default:
			s.mu.Unlock()
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			_, _ = cmd.Process.Wait()
			return 0, fmt.Errorf("supervisor start %s: race, already running", spec.ID)
		}
	}
	s.procs[spec.ID] = m
	s.mu.Unlock()

	// Фоновый Wait: фиксирует exit_code и закрывает done.
	go s.waitProc(m)

	return m.pid, nil
}

// waitProc дожидается завершения OS-процесса и сохраняет код выхода.
func (s *Supervisor) waitProc(m *managed) {
	err := m.cmd.Wait()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else {
			code = -1
		}
	} else if m.cmd.ProcessState != nil {
		code = m.cmd.ProcessState.ExitCode()
	}

	s.mu.Lock()
	m.exitCode = &code
	m.waitErr = err
	close(m.done)
	s.mu.Unlock()
}

// Stop останавливает процесс: SIGTERM группе → grace → SIGKILL.
// Если процесса нет или уже завершён — nil (идемпотентность для reconcile).
func (s *Supervisor) Stop(ctx context.Context, id string) error {
	s.mu.Lock()
	m, ok := s.procs[id]
	s.mu.Unlock()
	if !ok {
		return nil
	}

	select {
	case <-m.done:
		return nil
	default:
	}

	// SIGTERM всей process group (−pid).
	if err := syscall.Kill(-m.pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("supervisor stop %s: SIGTERM: %w", id, err)
	}

	grace := s.grace
	timer := time.NewTimer(grace)
	defer timer.Stop()

	select {
	case <-m.done:
		return nil
	case <-ctx.Done():
		_ = syscall.Kill(-m.pid, syscall.SIGKILL)
		<-m.done
		return fmt.Errorf("supervisor stop %s: %w", id, ctx.Err())
	case <-timer.C:
		// Не дождались — принудительно.
		if err := syscall.Kill(-m.pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("supervisor stop %s: SIGKILL: %w", id, err)
		}
		select {
		case <-m.done:
			return nil
		case <-ctx.Done():
			return fmt.Errorf("supervisor stop %s: wait after kill: %w", id, ctx.Err())
		}
	}
}

// Snapshot возвращает состояние процесса; ok=false если ID никогда не стартовали
// или слот уже убран. Завершённые процессы остаются в map до следующего Start
// или Forget — чтобы reconcile успел прочитать exit_code.
func (s *Supervisor) Snapshot(id string) (Snapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.procs[id]
	if !ok {
		return Snapshot{}, false
	}
	snap := Snapshot{ID: id, PID: m.pid, WaitErr: m.waitErr}
	select {
	case <-m.done:
		snap.Running = false
		if m.exitCode != nil {
			c := *m.exitCode
			snap.ExitCode = &c
		}
	default:
		snap.Running = true
	}
	return snap, true
}

// IsRunning — удобная обёртка над Snapshot.
func (s *Supervisor) IsRunning(id string) bool {
	snap, ok := s.Snapshot(id)
	return ok && snap.Running
}

// Forget снимает завершённый слот с учёта (после того как reconcile записал failed/stopped).
// Running процесс не трогает.
func (s *Supervisor) Forget(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.procs[id]
	if !ok {
		return
	}
	select {
	case <-m.done:
		delete(s.procs, id)
	default:
		// ещё жив — не забываем
	}
}

// ManagedIDs — список ID на учёте (включая уже завершённые, пока не Forget).
func (s *Supervisor) ManagedIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.procs))
	for id := range s.procs {
		out = append(out, id)
	}
	return out
}

// StopAll останавливает все учтённые процессы (shutdown агента).
func (s *Supervisor) StopAll(ctx context.Context) error {
	ids := s.ManagedIDs()
	var first error
	for _, id := range ids {
		if err := s.Stop(ctx, id); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// splitCommand делит команду на argv без shell.
// Кавычки не поддерживаем в MVP: «./bot» или «/path/fake-bot --flag».
func splitCommand(command string) ([]string, error) {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	return fields, nil
}
