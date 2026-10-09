package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"kterminal/internal/squad"
)

func newSquadModel(t *testing.T, terminalWidth int) Model {
	t.Helper()
	m := newTestModel(t)
	if err := m.agent.ActivateMode("squad"); err != nil {
		t.Fatalf("activate squad mode: %v", err)
	}
	return step(t, m, tea.WindowSizeMsg{Width: terminalWidth, Height: 30})
}

func assertSidebarLineWidth(t *testing.T, view string, width int) {
	t.Helper()
	for i, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got != width {
			t.Fatalf("sidebar line %d width = %d, want %d:\n%q", i, got, width, line)
		}
	}
}

func TestSidebarWidthMatrix(t *testing.T) {
	cases := []struct {
		terminal int
		want     int
	}{
		{80, 0},
		{99, 0},
		{100, 24},
		{140, 35},
		{200, 40},
		{250, 40},
	}
	for _, c := range cases {
		m := newSquadModel(t, c.terminal)
		if got := m.sidebarWidth(); got != c.want {
			t.Fatalf("sidebarWidth() at %d cols = %d, want %d", c.terminal, got, c.want)
		}
	}
}

func TestSidebarWidthHiddenInSDDMode(t *testing.T) {
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 200, Height: 30})
	if got := m.sidebarWidth(); got != 0 {
		t.Fatalf("sidebarWidth() in sdd mode = %d, want 0", got)
	}
}

func TestSidebarViewRendersPopulatedMesa(t *testing.T) {
	m := newSquadModel(t, 200)
	m.currentModel = "glm-5.3"
	m.personaActivity = map[string]string{"backend": "bash(npm test)"}
	mesa := &squad.Mesa{
		Roles:           []string{"architect", "backend"},
		MaxConvocations: 8,
		TokenBudget:     200000,
		Convocations:    2,
		Tokens:          12400,
		Entries: []squad.MesaEntry{
			{Name: "architect", Discipline: "architecture", Status: squad.StatusDone, Model: "glm-5.3", Tokens: 5000, Cost: 0.01},
			{Name: "backend", Discipline: "backend", Status: squad.StatusDeliberating, Model: "glm-4.7", Tokens: 7400, Cost: 0.02},
		},
	}
	m.agent.RestoreMesa(mesa)

	width := m.sidebarWidth()
	if width != 40 {
		t.Fatalf("sidebarWidth() = %d, want 40 at 200 cols", width)
	}
	view := m.sidebarView(width)
	plain := stripANSI(view)

	for _, want := range []string{
		"squad",
		"maestro",
		"waiting",
		"architect",
		"done",
		"backend",
		"deliberating",
		"glm-5.3",
		"glm-4.7",
		"5.0k tok",
		"$0.0100",
		"7.4k tok",
		"$0.0200",
		"bash(npm test)",
		"—",
		"2/8 convocations",
		"12.4k/200.0k tokens",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("sidebar missing %q:\n%s", want, plain)
		}
	}
	if got := strings.Count(plain, "$"); got != 2 {
		t.Fatalf("maestro must not render its own metrics, found %d costs, want 2:\n%s", got, plain)
	}
	assertSidebarLineWidth(t, view, width)
}

func TestSidebarViewTruncatesLongContent(t *testing.T) {
	m := newSquadModel(t, 100)
	longName := strings.Repeat("n", 200)
	longActivity := strings.Repeat("a", 200)
	mesa := &squad.Mesa{
		MaxConvocations: 4,
		TokenBudget:     100000,
		Entries:         []squad.MesaEntry{{Name: longName, Discipline: "qa", Status: squad.StatusWaiting}},
	}
	m.agent.RestoreMesa(mesa)
	m.personaActivity = map[string]string{longName: longActivity}

	width := m.sidebarWidth()
	if width != 24 {
		t.Fatalf("sidebarWidth() = %d, want 24 at 100 cols", width)
	}
	view := m.sidebarView(width)
	assertSidebarLineWidth(t, view, width)

	plain := stripANSI(view)
	if !strings.Contains(plain, "…") {
		t.Fatalf("long content missing the ellipsis:\n%s", plain)
	}
	if strings.Contains(plain, strings.Repeat("n", 30)) {
		t.Fatalf("long name not truncated:\n%s", plain)
	}
	if strings.Contains(plain, strings.Repeat("a", 30)) {
		t.Fatalf("long activity not truncated:\n%s", plain)
	}
	if !strings.Contains(plain, "waiting") {
		t.Fatalf("status must survive name truncation:\n%s", plain)
	}
}

func TestSidebarViewMesaNotStarted(t *testing.T) {
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	if m.agent.Mesa() != nil {
		t.Fatal("fresh agent mesa = non-nil, want nil")
	}
	view := m.sidebarView(30)
	plain := stripANSI(view)
	if !strings.Contains(plain, "mesa not started") {
		t.Fatalf("nil mesa must render the not-started state:\n%s", plain)
	}
	if !strings.Contains(plain, "maestro") {
		t.Fatalf("maestro line missing with nil mesa:\n%s", plain)
	}
	if strings.Contains(plain, "convocations") || strings.Contains(plain, "tokens") {
		t.Fatalf("nil mesa must not render the budget footer:\n%s", plain)
	}

	m2 := newSquadModel(t, 120)
	if m2.agent.Mesa() == nil {
		t.Fatal("squad mode must initialize an empty mesa")
	}
	plain2 := stripANSI(m2.sidebarView(m2.sidebarWidth()))
	if !strings.Contains(plain2, "mesa not started") {
		t.Fatalf("empty mesa before kickoff must render the not-started state:\n%s", plain2)
	}
}

func TestSidebarViewPersonaWithoutActivity(t *testing.T) {
	m := newSquadModel(t, 200)
	mesa := &squad.Mesa{
		MaxConvocations: 4,
		TokenBudget:     100000,
		Entries:         []squad.MesaEntry{{Name: "architect", Discipline: "architecture", Status: squad.StatusWaiting}},
	}
	m.agent.RestoreMesa(mesa)

	plain := stripANSI(m.sidebarView(m.sidebarWidth()))
	lines := strings.Split(plain, "\n")
	for i, line := range lines {
		if !strings.Contains(line, "▸ architect · waiting") {
			continue
		}
		if i+2 >= len(lines) {
			t.Fatalf("persona block truncated:\n%s", plain)
		}
		if !strings.HasSuffix(strings.TrimSpace(lines[i+1]), "—") || !strings.HasSuffix(strings.TrimSpace(lines[i+2]), "—") {
			t.Fatalf("persona without activity must render — for metrics and activity:\n%s", plain)
		}
		return
	}
	t.Fatalf("persona header missing:\n%s", plain)
}

func TestSidebarMaestroStatusTracksBusy(t *testing.T) {
	m := newSquadModel(t, 200)
	m.busy = true
	plain := stripANSI(m.sidebarView(m.sidebarWidth()))
	if !strings.Contains(plain, "maestro · deliberating") {
		t.Fatalf("busy maestro must show deliberating:\n%s", plain)
	}
	m.busy = false
	plain = stripANSI(m.sidebarView(m.sidebarWidth()))
	if !strings.Contains(plain, "maestro · waiting") {
		t.Fatalf("idle maestro must show waiting:\n%s", plain)
	}

	narrow := newSquadModel(t, 100)
	narrow.busy = true
	view := narrow.sidebarView(narrow.sidebarWidth())
	assertSidebarLineWidth(t, view, 24)
	if !strings.Contains(stripANSI(view), "deliberating") {
		t.Fatalf("status must stay textual at the minimum panel width:\n%s", stripANSI(view))
	}
}

func TestSidebarStatusAlwaysTextual(t *testing.T) {
	m := newSquadModel(t, 200)
	for _, status := range []string{squad.StatusWaiting, squad.StatusDeliberating, squad.StatusDone} {
		mesa := &squad.Mesa{
			MaxConvocations: 1,
			TokenBudget:     1000,
			Entries:         []squad.MesaEntry{{Name: "persona-" + status, Discipline: "backend", Status: status}},
		}
		m.agent.RestoreMesa(mesa)
		plain := stripANSI(m.sidebarView(m.sidebarWidth()))
		if !strings.Contains(plain, status) {
			t.Fatalf("status %q missing as text:\n%s", status, plain)
		}
		if !strings.Contains(plain, "persona-"+status) {
			t.Fatalf("persona name missing as text:\n%s", plain)
		}
	}
}

func TestSidebarReusesDisciplineColor(t *testing.T) {
	forceTrueColor(t)
	m := newSquadModel(t, 200)
	mesa := &squad.Mesa{
		MaxConvocations: 1,
		TokenBudget:     1000,
		Entries:         []squad.MesaEntry{{Name: "architect", Discipline: "architecture", Status: squad.StatusWaiting}},
	}
	m.agent.RestoreMesa(mesa)
	view := m.sidebarView(m.sidebarWidth())
	if !strings.Contains(view, "\x1b[38;2;157;124;216m▸ architect") {
		t.Fatalf("persona name missing the architecture discipline color:\n%q", view)
	}
	plain := stripANSI(view)
	if !strings.Contains(plain, "▸ architect · waiting") {
		t.Fatalf("name and status must stay textual next to the color:\n%s", plain)
	}
}

func TestSidebarFillsWindowHeight(t *testing.T) {
	m := newSquadModel(t, 200)
	mesa := &squad.Mesa{
		MaxConvocations: 4,
		TokenBudget:     100000,
		Entries:         []squad.MesaEntry{{Name: "architect", Discipline: "architecture", Status: squad.StatusWaiting}},
	}
	m.agent.RestoreMesa(mesa)
	if got := lipgloss.Height(m.sidebarView(m.sidebarWidth())); got != m.height {
		t.Fatalf("sidebar height = %d, want %d (full window height):\n%s", got, m.height, m.sidebarView(m.sidebarWidth()))
	}
	if got := lipgloss.Height(m.View()); got != m.height {
		t.Fatalf("view height = %d, want %d (sidebar must span top to bottom):\n%s", got, m.height, m.View())
	}

	idle := newSquadModel(t, 120)
	if got := lipgloss.Height(idle.sidebarView(idle.sidebarWidth())); got != idle.height {
		t.Fatalf("idle sidebar height = %d, want %d:\n%s", got, idle.height, idle.sidebarView(idle.sidebarWidth()))
	}

	sdd := newTestModel(t)
	sdd = step(t, sdd, tea.WindowSizeMsg{Width: 200, Height: 30})
	if got := lipgloss.Height(sdd.View()); got != sdd.height {
		t.Fatalf("sdd view height = %d, want %d (chat unchanged):\n%s", got, sdd.height, sdd.View())
	}
}
