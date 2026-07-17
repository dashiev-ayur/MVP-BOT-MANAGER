package memory

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"mvp-manager/internal/store"
)

// snapshot — сериализуемый снимок всего store для file-backed режима.
// Индексы byRuntimeName/byPort на диске не храним: восстанавливаем при load.
type snapshot struct {
	Nodes    []store.Node     `json:"nodes"`
	Runtimes []store.Runtime  `json:"runtimes"`
	Bots     []store.Bot      `json:"bots"`
	Events   []store.BotEvent `json:"events,omitempty"`
}

// withDiskLocked выполняет fn под flock файла store.
// Перед fn — reload с диска; после успешного fn при write — запись обратно.
// Так agent и ctl не затирают чужие изменения между read и write.
//
// Вызывается только под s.mu (см. doRead/doWrite).
func (s *shared) withDiskLocked(write bool, fn func() error) error {
	if s.path == "" {
		return fn()
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("memory store mkdir %s: %w", filepath.Dir(s.path), err)
	}

	f, err := os.OpenFile(s.path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return fmt.Errorf("memory store open %s: %w", s.path, err)
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("memory store flock %s: %w", s.path, err)
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()

	if err := s.loadFromFileLocked(f); err != nil {
		return err
	}
	if err := fn(); err != nil {
		return err
	}
	if !write {
		return nil
	}
	return s.saveToFileLocked(f)
}

// loadFromFileLocked читает JSON из уже открытого и залоченного файла.
func (s *shared) loadFromFileLocked(f *os.File) error {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("memory store seek: %w", err)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("memory store read %s: %w", s.path, err)
	}
	if len(data) == 0 {
		s.replaceMaps(nil, nil, nil, nil)
		return nil
	}

	var snap snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return fmt.Errorf("memory store decode %s: %w", s.path, err)
	}
	s.replaceMaps(snap.Nodes, snap.Runtimes, snap.Bots, snap.Events)
	return nil
}

// saveToFileLocked пишет maps в уже залоченный файл (truncate + write).
// Под flock rename не обязателен: читатели тоже ждут LOCK_EX.
func (s *shared) saveToFileLocked(f *os.File) error {
	snap := snapshot{
		Nodes:    make([]store.Node, 0, len(s.nodes)),
		Runtimes: make([]store.Runtime, 0, len(s.runtimes)),
		Bots:     make([]store.Bot, 0, len(s.bots)),
		Events:   make([]store.BotEvent, 0, len(s.events)),
	}
	for _, n := range s.nodes {
		snap.Nodes = append(snap.Nodes, cloneNode(n))
	}
	for _, rt := range s.runtimes {
		snap.Runtimes = append(snap.Runtimes, cloneRuntime(rt))
	}
	for _, b := range s.bots {
		snap.Bots = append(snap.Bots, cloneBot(b))
	}
	for _, ev := range s.events {
		snap.Events = append(snap.Events, cloneEvent(ev))
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("memory store encode: %w", err)
	}
	data = append(data, '\n')

	if err := f.Truncate(0); err != nil {
		return fmt.Errorf("memory store truncate %s: %w", s.path, err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("memory store seek: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("memory store write %s: %w", s.path, err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("memory store sync %s: %w", s.path, err)
	}
	return nil
}

// replaceMaps полностью подменяет внутренние maps и индексы UNIQUE.
func (s *shared) replaceMaps(nodes []store.Node, runtimes []store.Runtime, bots []store.Bot, events []store.BotEvent) {
	s.nodes = make(map[string]store.Node, len(nodes))
	s.runtimes = make(map[string]store.Runtime, len(runtimes))
	s.bots = make(map[string]store.Bot, len(bots))
	s.events = make([]store.BotEvent, 0, len(events))
	s.byRuntimeName = make(map[string]string, len(runtimes))
	s.byPort = make(map[int]string, len(bots))

	for _, n := range nodes {
		s.nodes[n.ID] = cloneNode(n)
	}
	for _, rt := range runtimes {
		stored := cloneRuntime(rt)
		s.runtimes[stored.ID] = stored
		s.byRuntimeName[stored.Name] = stored.ID
	}
	for _, b := range bots {
		stored := cloneBot(b)
		s.bots[stored.ID] = stored
		s.byPort[stored.Port] = stored.ID
	}
	for _, ev := range events {
		s.events = append(s.events, cloneEvent(ev))
	}
}
