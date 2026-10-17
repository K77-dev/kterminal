package procmon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"sync"
	"syscall"
	"time"
)

const (
	spawnGrace   = 250 * time.Millisecond
	killGrace    = 2 * time.Second
	captureBytes = 2048
	joinBudget   = 2 * time.Second
)

type Entry struct {
	ID      int
	Command string
	Started time.Time
}

type proc struct {
	id      int
	command string
	started time.Time
	pid     int
	done    chan struct{}
	exited  int
	capture *discardWriter
}

type Manager struct {
	mu     sync.Mutex
	next   int
	procs  map[int]*proc
	events chan struct{}
	now    func() time.Time
}

func New() *Manager {
	return &Manager{
		procs:  make(map[int]*proc),
		events: make(chan struct{}, 1),
		now:    time.Now,
	}
}

func (m *Manager) Events() <-chan struct{} {
	return m.events
}

func (m *Manager) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.procs)
}

func (m *Manager) List() []Entry {
	m.mu.Lock()
	entries := make([]Entry, 0, len(m.procs))
	for _, p := range m.procs {
		entries = append(entries, Entry{ID: p.id, Command: p.command, Started: p.started})
	}
	m.mu.Unlock()
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries
}

func (m *Manager) Start(ctx context.Context, command string) (Entry, error) {
	cmd, pipe, err := launch(command)
	if err != nil {
		return Entry{}, err
	}
	m.mu.Lock()
	m.next++
	id := m.next
	m.mu.Unlock()
	p := &proc{
		id:      id,
		command: command,
		started: m.now(),
		pid:     cmd.Process.Pid,
		done:    make(chan struct{}),
		capture: newDiscardWriter(m.now().Add(spawnGrace)),
	}
	go func() {
		p.exited = exitCode(cmd.Wait())
		close(p.done)
		m.remove(id)
		m.notify()
	}()
	go func() {
		_, _ = io.Copy(p.capture, pipe)
		pipe.Close()
	}()
	timer := time.NewTimer(spawnGrace)
	defer timer.Stop()
	select {
	case <-p.done:
		return Entry{}, startupError(p)
	case <-timer.C:
	}
	select {
	case <-p.done:
		return Entry{}, startupError(p)
	default:
	}
	if err := ctx.Err(); err != nil {
		_ = signalGroup(p.pid, syscall.SIGKILL)
		return Entry{}, err
	}
	m.mu.Lock()
	m.procs[id] = p
	m.mu.Unlock()
	return Entry{ID: p.id, Command: p.command, Started: p.started}, nil
}

func (m *Manager) Stop(id int) bool {
	m.mu.Lock()
	p, ok := m.procs[id]
	m.mu.Unlock()
	if !ok {
		return false
	}
	_ = signalGroup(p.pid, syscall.SIGTERM)
	time.AfterFunc(killGrace, func() {
		select {
		case <-p.done:
		default:
			_ = signalGroup(p.pid, syscall.SIGKILL)
		}
	})
	return true
}

func (m *Manager) KillAll() {
	m.mu.Lock()
	snapshot := make([]*proc, 0, len(m.procs))
	for _, p := range m.procs {
		snapshot = append(snapshot, p)
	}
	m.mu.Unlock()
	if len(snapshot) == 0 {
		return
	}
	for _, p := range snapshot {
		_ = signalGroup(p.pid, syscall.SIGTERM)
	}
	joinWithin(snapshot, joinBudget)
	for _, p := range snapshot {
		select {
		case <-p.done:
		default:
			_ = signalGroup(p.pid, syscall.SIGKILL)
		}
	}
	joinWithin(snapshot, joinBudget)
}

func joinWithin(procs []*proc, budget time.Duration) {
	deadline := time.Now().Add(budget)
	for _, p := range procs {
		select {
		case <-p.done:
		case <-time.After(time.Until(deadline)):
			return
		}
	}
}

func startupError(p *proc) error {
	return fmt.Errorf("background command exited during startup (exit status %d): %s", p.exited, p.capture.captured())
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

func (m *Manager) remove(id int) {
	m.mu.Lock()
	delete(m.procs, id)
	m.mu.Unlock()
}

func (m *Manager) notify() {
	select {
	case m.events <- struct{}{}:
	default:
	}
}

type discardWriter struct {
	mu           sync.Mutex
	buf          []byte
	captureUntil time.Time
	now          func() time.Time
}

func newDiscardWriter(captureUntil time.Time) *discardWriter {
	return &discardWriter{captureUntil: captureUntil, now: time.Now}
}

func (w *discardWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	if w.now().Before(w.captureUntil) && len(w.buf) < captureBytes {
		n := captureBytes - len(w.buf)
		if n > len(p) {
			n = len(p)
		}
		w.buf = append(w.buf, p[:n]...)
	}
	w.mu.Unlock()
	return len(p), nil
}

func (w *discardWriter) captured() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(w.buf)
}
