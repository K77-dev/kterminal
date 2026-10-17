package procmon

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	m := New()
	t.Cleanup(m.KillAll)
	return m
}

func waitForCount(t *testing.T, m *Manager, want int, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if m.Count() == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("Count() = %d, want %d within %s", m.Count(), want, within)
}

func TestStartRegistersRunningProcess(t *testing.T) {
	m := newTestManager(t)
	begin := time.Now()
	e, err := m.Start(context.Background(), "sleep 2")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if elapsed := time.Since(begin); elapsed >= time.Second {
		t.Fatalf("Start took %s, want <1s", elapsed)
	}
	if e.ID <= 0 {
		t.Fatalf("Entry.ID = %d, want > 0", e.ID)
	}
	if m.Count() != 1 {
		t.Fatalf("Count() = %d, want 1", m.Count())
	}
	list := m.List()
	if len(list) != 1 || list[0].Command != "sleep 2" {
		t.Fatalf("List() = %+v, want one entry with the command", list)
	}
	if list[0].Started.IsZero() {
		t.Fatal("Entry.Started is zero")
	}
}

func TestStartSpawnErrorWithinGrace(t *testing.T) {
	m := newTestManager(t)
	_, err := m.Start(context.Background(), "exit 3")
	if err == nil || !strings.Contains(err.Error(), "exit status 3") {
		t.Fatalf("Start error = %v, want exit status 3", err)
	}
	if m.Count() != 0 {
		t.Fatalf("Count() = %d, want 0 (no phantom monitoring)", m.Count())
	}
}

func TestStartUnknownCommand(t *testing.T) {
	m := newTestManager(t)
	_, err := m.Start(context.Background(), "definitely-not-a-cmd-xyz")
	if err == nil || !strings.Contains(err.Error(), "127") {
		t.Fatalf("Start error = %v, want exit status 127", err)
	}
	if m.Count() != 0 {
		t.Fatalf("Count() = %d, want 0", m.Count())
	}
}

func TestStartFastLegitCommand(t *testing.T) {
	m := newTestManager(t)
	_, err := m.Start(context.Background(), "echo hi")
	if err == nil || !strings.Contains(err.Error(), "exited during startup") {
		t.Fatalf("Start error = %v, want honest startup error", err)
	}
	if m.Count() != 0 {
		t.Fatalf("Count() = %d, want 0", m.Count())
	}
}

func TestStartCtxCanceledDuringGrace(t *testing.T) {
	m := newTestManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	_, err := m.Start(ctx, "sleep 30 # procmon-ctx-cancel-marker")
	if err != context.Canceled {
		t.Fatalf("Start error = %v, want context.Canceled", err)
	}
	if m.Count() != 0 {
		t.Fatalf("Count() = %d, want 0", m.Count())
	}
	time.Sleep(200 * time.Millisecond)
	out, _ := exec.Command("pgrep", "-f", "procmon-ctx-cancel-marker").Output()
	if len(out) > 0 {
		t.Fatalf("process survived ctx cancel: %s", out)
	}
}

func TestNaturalExitDecrementsCount(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.Start(context.Background(), "sleep 1"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-m.Events():
	case <-time.After(5 * time.Second):
		t.Fatal("no event after natural exit")
	}
	waitForCount(t, m, 0, 5*time.Second)
	if len(m.List()) != 0 {
		t.Fatalf("List() = %+v, want empty", m.List())
	}
}

func TestStopKillsProcess(t *testing.T) {
	m := newTestManager(t)
	e, err := m.Start(context.Background(), "sleep 30")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	begin := time.Now()
	if !m.Stop(e.ID) {
		t.Fatal("Stop = false, want true")
	}
	if elapsed := time.Since(begin); elapsed >= 250*time.Millisecond {
		t.Fatalf("Stop took %s, want immediate return", elapsed)
	}
	waitForCount(t, m, 0, killGrace+2*time.Second)
}

func TestStopSigtermResistantProcess(t *testing.T) {
	m := newTestManager(t)
	e, err := m.Start(context.Background(), "trap '' TERM; while :; do sleep 1; done")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !m.Stop(e.ID) {
		t.Fatal("Stop = false, want true")
	}
	waitForCount(t, m, 0, 2*killGrace+2*time.Second)
}

func TestStopUnknownID(t *testing.T) {
	m := newTestManager(t)
	if m.Stop(999) {
		t.Fatal("Stop(999) = true, want false")
	}
	if m.Count() != 0 {
		t.Fatalf("Count() = %d, want 0", m.Count())
	}
}

func TestKillAllTerminatesEverything(t *testing.T) {
	m := newTestManager(t)
	for i := 0; i < 2; i++ {
		if _, err := m.Start(context.Background(), "sleep 30"); err != nil {
			t.Fatalf("Start %d: %v", i, err)
		}
	}
	begin := time.Now()
	m.KillAll()
	if elapsed := time.Since(begin); elapsed >= 2*killGrace {
		t.Fatalf("KillAll took %s, want <2×killGrace", elapsed)
	}
	if m.Count() != 0 {
		t.Fatalf("Count() = %d, want 0", m.Count())
	}
	if len(m.List()) != 0 {
		t.Fatalf("List() = %+v, want empty", m.List())
	}
}

func TestKillAllIdempotent(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.Start(context.Background(), "sleep 30"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	m.KillAll()
	begin := time.Now()
	m.KillAll()
	if elapsed := time.Since(begin); elapsed >= time.Second {
		t.Fatalf("second KillAll took %s, want fast no-op", elapsed)
	}
	if m.Count() != 0 {
		t.Fatalf("Count() = %d, want 0", m.Count())
	}
}

func TestEventsCoalesce(t *testing.T) {
	m := newTestManager(t)
	for i := 0; i < 3; i++ {
		if _, err := m.Start(context.Background(), "sleep 1"); err != nil {
			t.Fatalf("Start %d: %v", i, err)
		}
	}
	received := 0
	deadline := time.After(5 * time.Second)
	for m.Count() > 0 || received == 0 {
		select {
		case <-m.Events():
			received++
		case <-deadline:
			t.Fatalf("events stalled: count=%d received=%d", m.Count(), received)
		}
	}
	if received == 0 {
		t.Fatal("no wake received")
	}
}

func TestConcurrentStartStop(t *testing.T) {
	m := newTestManager(t)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = m.Start(context.Background(), "sleep 30")
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			m.Stop(id + 1)
		}(i)
	}
	wg.Wait()
	if m.Count() > 8 {
		t.Fatalf("Count() = %d, want <= 8", m.Count())
	}
}

func TestChattyChildDoesNotDeadlock(t *testing.T) {
	m := newTestManager(t)
	begin := time.Now()
	_, err := m.Start(context.Background(), `for i in $(seq 1 2000); do echo "0123456789012345678901234567890123456789"; done; sleep 1`)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if elapsed := time.Since(begin); elapsed >= time.Second {
		t.Fatalf("Start took %s, want <1s", elapsed)
	}
	if m.Count() != 1 {
		t.Fatalf("Count() = %d, want 1", m.Count())
	}
	waitForCount(t, m, 0, 10*time.Second)
}
