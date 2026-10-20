package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"kterminal/internal/agent"
	"kterminal/internal/catalog"
	"kterminal/internal/config"
	"kterminal/internal/kspec"
	"kterminal/internal/laya"
	"kterminal/internal/llm"
	"kterminal/internal/procmon"
	"kterminal/internal/router"
	"kterminal/internal/tools"
)

func kspecStoreAt(t *testing.T, dir string) *kspec.Store {
	t.Helper()
	t.Chdir(dir)
	return kspec.Load()
}

func writeKspecProjectSkill(t *testing.T, dir, name, version, body string) {
	t.Helper()
	skillDir := filepath.Join(dir, ".agents", "skills", name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	skill := "---\nname: " + name + "\nversion: " + version + "\ndescription: Custom " + name + ".\n---\n" + body
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skill), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
}

func newKspecAgentModel(t *testing.T, gwURL string) (Model, *agent.Agent) {
	t.Helper()
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	store := kspecStoreAt(t, t.TempDir())
	ag := agent.New(llm.New(gwURL, "gw-key", false), nil, nil, cat, tools.NewRegistry(), nil, false)
	ag.SetPinned("glm-5.3")
	ag.AttachKspec(store)
	return New(ag, &config.Config{}, cat, true, WithKspec(store)), ag
}

func runCommand(t *testing.T, m Model, cmdline string) Model {
	t.Helper()
	m.input.SetValue(cmdline)
	return step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
}

func TestKspecCommandActivatesAndKicksOff(t *testing.T) {
	gw, bodies := captureGateway(t)
	m, ag := newKspecAgentModel(t, gw.URL)

	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = runCommand(t, m, "/kspec-prd improve the login flow")
	if !m.busy {
		t.Fatal("expected busy=true after /kspec-prd")
	}
	if got := ag.ActiveSkill(); got != "kspec-prd" {
		t.Fatalf("active skill = %q, want kspec-prd", got)
	}
	if hint := stripANSI(m.hintBar()); !strings.Contains(hint, "skill kspec-prd") {
		t.Fatalf("hint bar missing active skill while busy:\n%s", hint)
	}
	if plain := stripANSI(m.contentRaw); !strings.Contains(plain, "/kspec-prd improve the login flow") {
		t.Fatalf("chat missing the command as a user block:\n%s", plain)
	}
	if len(m.promptHistory) != 1 || m.promptHistory[0] != "/kspec-prd improve the login flow" {
		t.Fatalf("history = %v, want the dispatched command", m.promptHistory)
	}

	done := waitForAgentEvent(t, ag.Events, agent.EventTurnDone)
	m = step(t, m, agentEventMsg{event: done})
	if m.busy {
		t.Fatal("expected busy=false after turn done")
	}
	if got := ag.ActiveSkill(); got != "kspec-prd" {
		t.Fatalf("skill must persist after the kickoff turn, got %q", got)
	}
	if hint := stripANSI(m.hintBar()); !strings.Contains(hint, "skill kspec-prd") {
		t.Fatalf("hint bar missing active skill after the turn:\n%s", hint)
	}

	var body string
	select {
	case body = <-bodies:
	default:
		t.Fatal("gateway captured no request body")
	}
	var req struct {
		Messages []llm.Message `json:"messages"`
	}
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	if len(req.Messages) == 0 || req.Messages[0].Role != "system" {
		t.Fatal("first request message must be the assembled system prompt")
	}
	if !strings.Contains(req.Messages[0].Content, "especialista em criar PRDs") {
		t.Fatal("system prompt missing the active skill content")
	}
	var user string
	for _, msg := range req.Messages {
		if msg.Role == "user" {
			user = msg.Content
		}
	}
	if user != "improve the login flow" {
		t.Fatalf("kickoff = %q, want the command argument", user)
	}
}

func TestKspecCommandDefaultKickoffWithoutArgument(t *testing.T) {
	gw, bodies := captureGateway(t)
	m, ag := newKspecAgentModel(t, gw.URL)

	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = runCommand(t, m, "/kspec-qa")
	if got := ag.ActiveSkill(); got != "kspec-qa" {
		t.Fatalf("active skill = %q, want kspec-qa", got)
	}

	done := waitForAgentEvent(t, ag.Events, agent.EventTurnDone)
	m = step(t, m, agentEventMsg{event: done})

	var body string
	select {
	case body = <-bodies:
	default:
		t.Fatal("gateway captured no request body")
	}
	var req struct {
		Messages []llm.Message `json:"messages"`
	}
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	var user string
	for _, msg := range req.Messages {
		if msg.Role == "user" {
			user = msg.Content
		}
	}
	if user != "Begin the kspec-qa workflow now." {
		t.Fatalf("kickoff = %q, want the default start instruction", user)
	}
}

func TestKspecCommandReplacesActiveSkill(t *testing.T) {
	gw, _ := captureGateway(t)
	m, ag := newKspecAgentModel(t, gw.URL)

	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = runCommand(t, m, "/kspec-prd first task")
	done := waitForAgentEvent(t, ag.Events, agent.EventTurnDone)
	m = step(t, m, agentEventMsg{event: done})
	if got := ag.ActiveSkill(); got != "kspec-prd" {
		t.Fatalf("active skill = %q, want kspec-prd", got)
	}

	m = runCommand(t, m, "/kspec-qa second task")
	if got := ag.ActiveSkill(); got != "kspec-qa" {
		t.Fatalf("active skill = %q, want kspec-qa after substitution", got)
	}
	if !m.busy {
		t.Fatal("substitution must start a new turn")
	}
	if hint := stripANSI(m.hintBar()); !strings.Contains(hint, "skill kspec-qa") {
		t.Fatalf("hint bar must show the replaced skill:\n%s", hint)
	}
	done = waitForAgentEvent(t, ag.Events, agent.EventTurnDone)
	m = step(t, m, agentEventMsg{event: done})

	plain := stripANSI(m.contentRaw)
	if !strings.Contains(plain, "/kspec-prd first task") || !strings.Contains(plain, "/kspec-qa second task") {
		t.Fatalf("chat missing one of the command user blocks:\n%s", plain)
	}
}

func TestKspecCommandRejectedWhenBusy(t *testing.T) {
	gw, bodies := captureGateway(t)
	m, ag := newKspecAgentModel(t, gw.URL)

	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m.busy = true
	m = runCommand(t, m, "/kspec-prd now")

	if got := ag.ActiveSkill(); got != "" {
		t.Fatalf("skill must not activate while busy, got %q", got)
	}
	if !m.busy {
		t.Fatal("busy must remain true after the rejection")
	}
	if plain := stripANSI(m.contentRaw); !strings.Contains(plain, "busy — wait for the current task to finish") {
		t.Fatalf("chat missing the busy rejection:\n%s", plain)
	}
	if plain := stripANSI(m.contentRaw); strings.Contains(plain, "/kspec-prd now") {
		t.Fatalf("rejected command must not render a user block:\n%s", plain)
	}
	select {
	case b := <-bodies:
		t.Fatalf("busy rejection must not call the agent, got body: %s", b)
	default:
	}
}

func TestKspecCommandUnknownSkill(t *testing.T) {
	gw, bodies := captureGateway(t)
	m, ag := newKspecAgentModel(t, gw.URL)

	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = runCommand(t, m, "/kspec-nope")

	if got := ag.ActiveSkill(); got != "" {
		t.Fatalf("unknown skill must not activate, got %q", got)
	}
	if m.busy {
		t.Fatal("unknown skill must not start a turn")
	}
	if plain := stripANSI(m.contentRaw); !strings.Contains(plain, "unknown skill kspec-nope") {
		t.Fatalf("chat missing the unknown skill error:\n%s", plain)
	}
	select {
	case b := <-bodies:
		t.Fatalf("unknown skill must not call the agent, got body: %s", b)
	default:
	}
}

func TestKspecCommandWithoutStore(t *testing.T) {
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = runCommand(t, m, "/kspec-prd")

	if m.busy {
		t.Fatal("command without a store must not start a turn")
	}
	if plain := stripANSI(m.contentRaw); !strings.Contains(plain, "kspec is not available") {
		t.Fatalf("chat missing the no-store error:\n%s", plain)
	}
}

func TestKspecVersionEmbeddedMode(t *testing.T) {
	store := kspecStoreAt(t, t.TempDir())
	if store.Source() != kspec.SourceEmbedded {
		t.Fatalf("source = %q, want embedded in an empty dir", store.Source())
	}
	m := newTestModel(t, WithKspec(store))
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = runCommand(t, m, "/kspec-version")

	if m.busy {
		t.Fatal("/kspec-version must not start a turn")
	}
	want := "kspec v" + store.Version() + " (" + kspec.SourceEmbedded + ")"
	if plain := stripANSI(m.contentRaw); !strings.Contains(plain, want) {
		t.Fatalf("chat missing %q:\n%s", want, plain)
	}
}

func TestKspecVersionProjectMode(t *testing.T) {
	dir := t.TempDir()
	writeKspecProjectSkill(t, dir, "kspec-version", "9.8.7", "body\n")
	store := kspecStoreAt(t, dir)
	if store.Source() != kspec.SourceProject {
		t.Fatalf("source = %q, want project with a project skill", store.Source())
	}
	m := newTestModel(t, WithKspec(store))
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = runCommand(t, m, "/kspec-version")

	want := "kspec v9.8.7 (" + kspec.SourceProject + ")"
	if plain := stripANSI(m.contentRaw); !strings.Contains(plain, want) {
		t.Fatalf("chat missing %q:\n%s", want, plain)
	}
}

func TestKspecVersionProjectModeUnknownVersion(t *testing.T) {
	dir := t.TempDir()
	writeKspecProjectSkill(t, dir, "kspec-prd", "1.0.0", "body\n")
	store := kspecStoreAt(t, dir)
	if v := store.Version(); v != "" {
		t.Fatalf("Version() = %q, want empty without the project version skill", v)
	}
	m := newTestModel(t, WithKspec(store))
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = runCommand(t, m, "/kspec-version")

	want := "kspec version unknown (" + kspec.SourceProject + ")"
	if plain := stripANSI(m.contentRaw); !strings.Contains(plain, want) {
		t.Fatalf("chat missing %q:\n%s", want, plain)
	}
}

func TestKspecVersionWithoutLLMCall(t *testing.T) {
	gw, bodies := captureGateway(t)
	m, ag := newKspecAgentModel(t, gw.URL)

	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = runCommand(t, m, "/kspec-version")

	if m.busy {
		t.Fatal("/kspec-version must not start a turn")
	}
	if got := ag.ActiveSkill(); got != "" {
		t.Fatalf("/kspec-version must not activate a skill, got %q", got)
	}
	select {
	case b := <-bodies:
		t.Fatalf("/kspec-version must not call the LLM, got body: %s", b)
	default:
	}
}

func TestKspecVersionWithoutStore(t *testing.T) {
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = runCommand(t, m, "/kspec-version")

	if plain := stripANSI(m.contentRaw); !strings.Contains(plain, "kspec is not available") {
		t.Fatalf("chat missing the no-store error:\n%s", plain)
	}
}

func TestHelpListsKspecCommands(t *testing.T) {
	store := kspecStoreAt(t, t.TempDir())
	m := newTestModel(t, WithKspec(store))
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = runCommand(t, m, "/help")

	plain := stripANSI(m.contentRaw)
	for _, want := range []string{
		"/config — configure gateway and API keys",
		"/bg — background processes",
		"kspec skills:",
		"/kspec-prd",
		"/kspec-version",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("help missing %q:\n%s", want, plain)
		}
	}
}

func TestHelpListsProjectSkillsFirst(t *testing.T) {
	dir := t.TempDir()
	writeKspecProjectSkill(t, dir, "kspec-prd", "9.9.9", "body\n")
	store := kspecStoreAt(t, dir)
	m := newTestModel(t, WithKspec(store))
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = runCommand(t, m, "/help")

	plain := stripANSI(m.contentRaw)
	if !strings.Contains(plain, "Custom kspec-prd.") {
		t.Fatalf("help missing the project skill description:\n%s", plain)
	}
	if strings.Contains(plain, "solicitação de funcionalidade") {
		t.Fatalf("help must not show the embedded description when the project copy wins:\n%s", plain)
	}
}

func TestHelpWithoutStoreOmitsKspecSection(t *testing.T) {
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = runCommand(t, m, "/help")

	plain := stripANSI(m.contentRaw)
	if !strings.Contains(plain, "/config — configure gateway and API keys") {
		t.Fatalf("help missing the native section:\n%s", plain)
	}
	if strings.Contains(plain, "kspec skills:") || strings.Contains(plain, "/kspec-prd") {
		t.Fatalf("help must omit the kspec section without a store:\n%s", plain)
	}
}

func TestHintBarShowsActiveSkill(t *testing.T) {
	gw, _ := captureGateway(t)
	m, ag := newKspecAgentModel(t, gw.URL)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	if strings.Contains(stripANSI(m.hintBar()), "skill ") {
		t.Fatal("hint bar must not show a skill before activation")
	}
	if err := ag.ActivateSkill("kspec-prd"); err != nil {
		t.Fatalf("ActivateSkill: %v", err)
	}
	if hint := stripANSI(m.hintBar()); !strings.Contains(hint, "skill kspec-prd") {
		t.Fatalf("hint bar missing the active skill:\n%s", hint)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "skill kspec-prd") {
		t.Fatalf("view missing the active skill:\n%s", view)
	}

	m = runCommand(t, m, "/clear")
	if got := ag.ActiveSkill(); got != "" {
		t.Fatalf("active skill = %q, want empty after /clear", got)
	}
	if strings.Contains(stripANSI(m.hintBar()), "skill ") {
		t.Fatalf("hint bar must drop the skill after /clear:\n%s", stripANSI(m.hintBar()))
	}
}

func TestClearRejectedWhenBusy(t *testing.T) {
	gw, _ := captureGateway(t)
	m, ag := newKspecAgentModel(t, gw.URL)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	if err := ag.ActivateSkill("kspec-prd"); err != nil {
		t.Fatalf("ActivateSkill: %v", err)
	}

	m.busy = true
	m = runCommand(t, m, "/clear")

	plain := stripANSI(m.contentRaw)
	if !strings.Contains(plain, "busy — wait for the current task to finish") {
		t.Fatalf("chat missing the busy rejection:\n%s", plain)
	}
	if strings.Contains(plain, "conversation cleared") {
		t.Fatalf("busy /clear must not clear the conversation:\n%s", plain)
	}
	if got := ag.ActiveSkill(); got != "kspec-prd" {
		t.Fatalf("busy /clear must not reset the agent, skill = %q", got)
	}
}

func TestCommandPopupListsNativeAndKspec(t *testing.T) {
	store := kspecStoreAt(t, t.TempDir())
	m := newTestModel(t, WithKspec(store))
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})

	if !m.cmdOpen {
		t.Fatal("expected command popup open after typing /")
	}
	want := []string{"/bg", "/clear", "/config", "/exit", "/help", "/image", "/kspec-bootstrap", "/kspec-bugfix", "/kspec-ideia", "/kspec-implement", "/kspec-pr-review", "/kspec-prd", "/kspec-qa", "/kspec-tasks", "/kspec-techspec", "/kspec-version", "/mode", "/model", "/quit", "/router", "/rtk", "/unimage"}
	if strings.Join(m.cmdItems, "|") != strings.Join(want, "|") {
		t.Fatalf("items = %v, want %v", m.cmdItems, want)
	}
	if m.cmdSelected != 0 {
		t.Fatalf("selected = %d, want 0 on open", m.cmdSelected)
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "▸ /bg") {
		t.Fatalf("view missing the selected command:\n%s", view)
	}
	if !strings.Contains(view, "  /kspec-prd") {
		t.Fatalf("view missing the kspec commands:\n%s", view)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.cmdSelected != 1 {
		t.Fatalf("down must move the selection, got %d", m.cmdSelected)
	}
	m = step(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if m.cmdSelected != 0 {
		t.Fatalf("up must move the selection, got %d", m.cmdSelected)
	}
	m = step(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if m.cmdSelected != len(m.cmdItems)-1 {
		t.Fatalf("up must wrap to the last item, got %d", m.cmdSelected)
	}
	m = step(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.cmdSelected != 0 {
		t.Fatalf("down must wrap to the first item, got %d", m.cmdSelected)
	}
	if got := len(textBeforeCursor(m.input)); got != 1 {
		t.Fatalf("popup navigation must not move the input cursor, pos=%d", got)
	}
}

func TestCommandPopupFiltersByPrefix(t *testing.T) {
	store := kspecStoreAt(t, t.TempDir())
	m := newTestModel(t, WithKspec(store))
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/kspec-pr")})

	if !m.cmdOpen {
		t.Fatal("expected popup open for /kspec-pr")
	}
	if strings.Join(m.cmdItems, "|") != "/kspec-pr-review|/kspec-prd" {
		t.Fatalf("items = %v, want [/kspec-pr-review /kspec-prd]", m.cmdItems)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if !m.cmdOpen {
		t.Fatal("popup must stay open after refining the prefix")
	}
	if strings.Join(m.cmdItems, "|") != "/kspec-prd" {
		t.Fatalf("items after typing d = %v, want [/kspec-prd]", m.cmdItems)
	}
	if m.cmdSelected != 0 {
		t.Fatalf("selection must reset on re-filter, got %d", m.cmdSelected)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	if m.cmdOpen {
		t.Fatal("prefix without matches must close the popup")
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	if m.cmdOpen {
		t.Fatal("space must keep the popup closed")
	}
}

func TestCommandPopupWithoutStoreListsNativesOnly(t *testing.T) {
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})

	if !m.cmdOpen {
		t.Fatal("expected popup open after typing /")
	}
	want := []string{"/bg", "/clear", "/config", "/exit", "/help", "/image", "/mode", "/model", "/quit", "/router", "/rtk", "/unimage"}
	if strings.Join(m.cmdItems, "|") != strings.Join(want, "|") {
		t.Fatalf("items = %v, want %v", m.cmdItems, want)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if m.cmdOpen {
		t.Fatal("no store means no /kspec-* suggestions")
	}
}

func waitForBgCount(t *testing.T, pm *procmon.Manager, want int, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if pm.Count() == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("Count() = %d, want %d within %s", pm.Count(), want, within)
}

func TestBgPopupOpenListStop(t *testing.T) {
	pm := procmon.New()
	t.Cleanup(pm.KillAll)
	m := newTestModel(t, WithProcMon(pm))
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	for i := 0; i < 2; i++ {
		if _, err := pm.Start(context.Background(), "sleep 30"); err != nil {
			t.Fatalf("Start %d: %v", i, err)
		}
	}

	m = runCommand(t, m, "/bg")
	if !m.bgOpen {
		t.Fatal("expected bg popup open after /bg")
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "#1 · sleep 30") || !strings.Contains(view, "#2 · sleep 30") {
		t.Fatalf("popup missing the process list:\n%s", view)
	}
	if !strings.Contains(view, "↑↓ move · enter stop · esc close") {
		t.Fatalf("popup missing the help line:\n%s", view)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	waitForBgCount(t, pm, 1, 3*time.Second)
	if !m.bgOpen {
		t.Fatal("popup must stay open after a stop")
	}
	view = stripANSI(m.View())
	if strings.Contains(view, "#2 ·") {
		t.Fatalf("popup must refresh the list after a stop:\n%s", view)
	}
	if !strings.Contains(stripANSI(m.hintBar()), "bg 1") {
		t.Fatalf("hint bar must show bg 1:\n%s", stripANSI(m.hintBar()))
	}
}

func TestBgPopupEscStopsNothing(t *testing.T) {
	pm := procmon.New()
	t.Cleanup(pm.KillAll)
	m := newTestModel(t, WithProcMon(pm))
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	for i := 0; i < 2; i++ {
		if _, err := pm.Start(context.Background(), "sleep 30"); err != nil {
			t.Fatalf("Start %d: %v", i, err)
		}
	}

	m = runCommand(t, m, "/bg")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.bgOpen {
		t.Fatal("esc must close the bg popup")
	}
	if pm.Count() != 2 {
		t.Fatalf("Count() = %d, want 2 (esc stops nothing)", pm.Count())
	}
}

func TestBgPopupEmptyState(t *testing.T) {
	pm := procmon.New()
	t.Cleanup(pm.KillAll)
	m := newTestModel(t, WithProcMon(pm))
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m = runCommand(t, m, "/bg")
	if m.bgOpen {
		t.Fatal("/bg with zero processes must not open the popup")
	}
	if !strings.Contains(stripANSI(m.contentRaw), "no background processes running") {
		t.Fatalf("chat missing the empty-state message:\n%s", stripANSI(m.contentRaw))
	}
}

func TestBgPopupSelectionClamps(t *testing.T) {
	pm := procmon.New()
	t.Cleanup(pm.KillAll)
	m := newTestModel(t, WithProcMon(pm))
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	for i := 0; i < 2; i++ {
		if _, err := pm.Start(context.Background(), "sleep 30"); err != nil {
			t.Fatalf("Start %d: %v", i, err)
		}
	}

	m = runCommand(t, m, "/bg")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	waitForBgCount(t, pm, 1, 3*time.Second)
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	waitForBgCount(t, pm, 0, 3*time.Second)
	if m.bgSelected != 0 {
		t.Fatalf("bgSelected = %d, want 0 clamped on empty list", m.bgSelected)
	}
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if !m.bgOpen {
		t.Fatal("popup must survive no-op keys on the empty list")
	}
}

func TestBgPopupKeyRouting(t *testing.T) {
	pm := procmon.New()
	t.Cleanup(pm.KillAll)
	m := newTestModel(t, WithProcMon(pm))
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	if _, err := pm.Start(context.Background(), "sleep 30"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	m = runCommand(t, m, "/bg")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("xyz")})
	if got := m.input.Value(); got != "" {
		t.Fatalf("typing must not reach the input with the bg popup open, got %q", got)
	}
	m = step(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if got := len(textBeforeCursor(m.input)); got != 0 {
		t.Fatalf("arrows must not move the textarea cursor, pos=%d", got)
	}
	if !m.bgOpen {
		t.Fatal("typing must not close the popup")
	}
}

func TestTabTogglesPermission(t *testing.T) {
	gw, _ := captureGateway(t)
	m, ag := newKspecAgentModel(t, gw.URL)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	if got := ag.Permission(); got != agent.PermissionBuild {
		t.Fatalf("default permission = %q, want build", got)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if got := ag.Permission(); got != agent.PermissionPlan {
		t.Fatalf("permission after tab = %q, want plan", got)
	}
	if hint := stripANSI(m.hintBar()); !strings.Contains(hint, "plan") {
		t.Fatalf("hint bar missing the plan segment:\n%s", hint)
	}
	plain := stripANSI(m.View())
	if !strings.Contains(plain, "plan mode — research and plan only") {
		t.Fatalf("view missing the plan announcement:\n%s", plain)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if got := ag.Permission(); got != agent.PermissionBuild {
		t.Fatalf("permission after second tab = %q, want build", got)
	}
	if hint := stripANSI(m.hintBar()); strings.Contains(hint, "plan") {
		t.Fatalf("hint bar must drop the plan segment in build:\n%s", hint)
	}
}

func TestAltTabTogglesPermissionWithTextInInput(t *testing.T) {
	gw, _ := captureGateway(t)
	m, ag := newKspecAgentModel(t, gw.URL)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("fix the bug")})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyTab, Alt: true})
	if got := ag.Permission(); got != agent.PermissionPlan {
		t.Fatalf("alt+tab with text in the input = %q, want plan", got)
	}
	if got := m.input.Value(); got != "fix the bug" {
		t.Fatalf("alt+tab must not touch the input, got %q", got)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyTab, Alt: true})
	if got := ag.Permission(); got != agent.PermissionBuild {
		t.Fatalf("alt+tab back = %q, want build", got)
	}
}

func TestTabWithTextDoesNotToggle(t *testing.T) {
	gw, _ := captureGateway(t)
	m, ag := newKspecAgentModel(t, gw.URL)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("plan this")})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if got := ag.Permission(); got != agent.PermissionBuild {
		t.Fatalf("tab with text in the input = %q, want build (no toggle)", got)
	}
}

func TestTabToggleInertInSquad(t *testing.T) {
	gw, _ := captureGateway(t)
	m, ag := newKspecAgentModel(t, gw.URL)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	if err := ag.ActivateMode("squad"); err != nil {
		t.Fatalf("ActivateMode: %v", err)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if got := ag.Permission(); got != agent.PermissionBuild {
		t.Fatalf("tab in squad mode = %q, want build (inert)", got)
	}
	plain := stripANSI(m.View())
	if !strings.Contains(plain, "/mode") {
		t.Fatalf("view missing the squad hint after tab:\n%s", plain)
	}
}

func TestPlanModeBlocksMutatingToolCall(t *testing.T) {
	gw, bodies := captureGateway(t)
	m, ag := newKspecAgentModel(t, gw.URL)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	if err := ag.SetPermission(agent.PermissionPlan); err != nil {
		t.Fatalf("SetPermission: %v", err)
	}

	m.input.SetValue("refactor the parser")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = pumpUntilEvent(t, m, ag.Events, func(e agent.Event) bool {
		return e.Kind == agent.EventTurnDone && e.Depth == 0
	})

	select {
	case body := <-bodies:
		if strings.Contains(body, "\"write\"") || strings.Contains(body, "\"edit\"") || strings.Contains(body, "\"bash\"") {
			t.Fatalf("plan mode must not offer mutating tools to the model:\n%s", body)
		}
	default:
	}

	if hint := stripANSI(m.hintBar()); !strings.Contains(hint, "plan") {
		t.Fatalf("hint bar missing the plan segment:\n%s", hint)
	}
}

func TestModeCommandSwitchesToSquad(t *testing.T) {
	gw, _ := captureGateway(t)
	m, ag := newKspecAgentModel(t, gw.URL)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m = runCommand(t, m, "/mode")
	if !m.modeOpen {
		t.Fatal("expected mode popup open after /mode")
	}
	if m.modeSelected != 0 {
		t.Fatalf("selected = %d, want 0 (sdd pre-selected)", m.modeSelected)
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "sdd") || !strings.Contains(view, "squad") {
		t.Fatalf("view missing the mode options:\n%s", view)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.modeOpen {
		t.Fatal("mode popup must close after confirming")
	}
	if got := ag.Mode(); got != "squad" {
		t.Fatalf("mode = %q, want squad", got)
	}
	if hint := stripANSI(m.hintBar()); strings.Contains(hint, "mode squad") {
		t.Fatalf("hint bar must not show the mode segment anymore:\n%s", hint)
	}

	m = runCommand(t, m, "/mode")
	if !m.modeOpen {
		t.Fatal("expected mode popup reopen")
	}
	if m.modeSelected != 1 {
		t.Fatalf("selected = %d, want 1 (squad pre-selected on reopen)", m.modeSelected)
	}
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.modeOpen {
		t.Fatal("esc must close the mode popup")
	}
	if got := ag.Mode(); got != "squad" {
		t.Fatalf("mode = %q, want squad unchanged after esc", got)
	}
}

func TestCommandTabCompletion(t *testing.T) {
	store := kspecStoreAt(t, t.TempDir())
	m := newTestModel(t, WithKspec(store))
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/kspec-pr")})
	if !m.cmdOpen {
		t.Fatal("expected popup open before tab")
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.cmdOpen {
		t.Fatal("popup must close after tab")
	}
	if got := m.input.Value(); got != "/kspec-pr-review " {
		t.Fatalf("input = %q, want /kspec-pr-review + trailing space", got)
	}
	if got := len(textBeforeCursor(m.input)); got != len("/kspec-pr-review ") {
		t.Fatalf("cursor = %d, want %d after completion", got, len("/kspec-pr-review "))
	}

	m2 := newTestModel(t, WithKspec(store))
	m2 = step(t, m2, tea.WindowSizeMsg{Width: 100, Height: 30})
	m2 = step(t, m2, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/kspec-pr")})
	m2 = step(t, m2, tea.KeyMsg{Type: tea.KeyDown})
	m2 = step(t, m2, tea.KeyMsg{Type: tea.KeyTab})
	if got := m2.input.Value(); got != "/kspec-prd " {
		t.Fatalf("input = %q, want /kspec-prd (selected item)", got)
	}
}

func TestCommandEnterCompletesNotSends(t *testing.T) {
	gw, bodies := captureGateway(t)
	m, ag := newKspecAgentModel(t, gw.URL)

	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/kspec-pr")})
	if !m.cmdOpen {
		t.Fatal("expected popup open before enter")
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.busy {
		t.Fatal("enter with popup open must not send")
	}
	if m.cmdOpen {
		t.Fatal("popup must close after enter completion")
	}
	if got := m.input.Value(); got != "/kspec-pr-review " {
		t.Fatalf("input = %q, want /kspec-pr-review completed", got)
	}
	select {
	case b := <-bodies:
		t.Fatalf("enter with popup open must not call the agent, got body: %s", b)
	default:
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.busy {
		t.Fatal("enter with popup closed must send")
	}
	if got := ag.ActiveSkill(); got != "kspec-pr-review" {
		t.Fatalf("active skill = %q, want kspec-pr-review", got)
	}
	done := waitForAgentEvent(t, ag.Events, agent.EventTurnDone)
	m = step(t, m, agentEventMsg{event: done})
	if m.busy {
		t.Fatal("expected busy=false after turn done")
	}
}

func TestCommandPopupEscCloses(t *testing.T) {
	store := kspecStoreAt(t, t.TempDir())
	m := newTestModel(t, WithKspec(store))
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if !m.cmdOpen {
		t.Fatal("expected popup open before esc")
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.cmdOpen {
		t.Fatal("esc must close the command popup")
	}
	if len(m.cmdItems) != 0 || m.cmdSelected != 0 {
		t.Fatalf("items/selection must clear after esc: %v %d", m.cmdItems, m.cmdSelected)
	}
	if got := m.input.Value(); got != "/" {
		t.Fatalf("esc must keep the input, got %q", got)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if !m.cmdOpen {
		t.Fatal("typing after esc must re-filter and reopen the popup")
	}
	if strings.Join(m.cmdItems, "|") != "/kspec-bootstrap|/kspec-bugfix|/kspec-ideia|/kspec-implement|/kspec-pr-review|/kspec-prd|/kspec-qa|/kspec-tasks|/kspec-techspec|/kspec-version" {
		t.Fatalf("items after reopen = %v", m.cmdItems)
	}
}

func TestCommandPopupNotOpenForPlainInput(t *testing.T) {
	storeDir := t.TempDir()
	store := kspecStoreAt(t, storeDir)
	m := newTestModel(t, WithKspec(store))
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hello /")})
	if m.cmdOpen {
		t.Fatal("popup must not open when / is not the first character")
	}

	m2 := newTestModel(t, WithKspec(store))
	m2 = step(t, m2, tea.WindowSizeMsg{Width: 100, Height: 30})
	writeFile(t, storeDir, "a.txt", "x")
	m2 = step(t, m2, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("@a")})
	if m2.cmdOpen {
		t.Fatal("mention input must not open the command popup")
	}
	if !m2.mentionOpen {
		t.Fatal("mention popup must still open for @ tokens")
	}
}

func TestCommandEnterDoubleFlowOnNativeCommand(t *testing.T) {
	gw, _ := captureGateway(t)
	m, ag := newKspecAgentModel(t, gw.URL)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	if err := ag.ActivateSkill("kspec-prd"); err != nil {
		t.Fatalf("ActivateSkill: %v", err)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/clear")})
	if !m.cmdOpen {
		t.Fatal("expected popup open after typing /clear")
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if got := ag.ActiveSkill(); got != "kspec-prd" {
		t.Fatalf("enter with popup open must complete, not execute — skill = %q", got)
	}
	if m.cmdOpen {
		t.Fatal("popup must close after enter completion")
	}
	if got := m.input.Value(); got != "/clear " {
		t.Fatalf("input = %q, want /clear + trailing space", got)
	}
	if plain := stripANSI(m.contentRaw); strings.Contains(plain, "conversation cleared") {
		t.Fatalf("first enter must not execute the command:\n%s", plain)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if got := ag.ActiveSkill(); got != "" {
		t.Fatalf("second enter must execute /clear, skill = %q", got)
	}
	if plain := stripANSI(m.contentRaw); !strings.Contains(plain, "conversation cleared") {
		t.Fatalf("chat missing cleared confirmation:\n%s", plain)
	}
}

func TestCommandPopupKeepsNativesVisibleWithManySkills(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 20; i++ {
		writeKspecProjectSkill(t, dir, fmt.Sprintf("kspec-skill-%02d", i), "1.0.0", "body\n")
	}
	store := kspecStoreAt(t, dir)
	m := newTestModel(t, WithKspec(store))
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})

	if len(m.cmdItems) != maxCommandSuggestions {
		t.Fatalf("items = %d, want the cap %d", len(m.cmdItems), maxCommandSuggestions)
	}
	for _, native := range nativeCommands {
		if !slices.Contains(m.cmdItems, native) {
			t.Fatalf("native %s must stay visible with many skills: %v", native, m.cmdItems)
		}
	}
}

func TestCommandPopupCacheRefreshesOnReopen(t *testing.T) {
	dir := t.TempDir()
	store := kspecStoreAt(t, dir)
	m := newTestModel(t, WithKspec(store))
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if !m.cmdOpen {
		t.Fatal("expected popup open after typing /")
	}

	writeKspecProjectSkill(t, dir, "kspec-new-skill", "1.0.0", "body\n")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if !m.cmdOpen {
		t.Fatal("popup must stay open while filtering")
	}
	if slices.Contains(m.cmdItems, "/kspec-new-skill") {
		t.Fatalf("skill list must stay frozen while the popup remains open: %v", m.cmdItems)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.cmdOpen {
		t.Fatal("esc must close the popup")
	}
	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if !m.cmdOpen {
		t.Fatal("typing after esc must reopen the popup")
	}
	if !slices.Contains(m.cmdItems, "/kspec-new-skill") {
		t.Fatalf("reopen must refresh the skill list: %v", m.cmdItems)
	}
}

func TestKspecCommandSendsPendingAttachments(t *testing.T) {
	imgDir := t.TempDir()
	writeFile(t, imgDir, "shot.png", "pngdata")

	gw, bodies := captureGateway(t)
	m, ag := newKspecAgentModel(t, gw.URL)

	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = runCommand(t, m, "/image "+filepath.Join(imgDir, "shot.png"))
	if len(m.pendingAttachments) != 1 {
		t.Fatalf("pendingAttachments = %d, want 1 before the kickoff", len(m.pendingAttachments))
	}

	m = runCommand(t, m, "/kspec-qa review this screenshot")
	if !m.busy {
		t.Fatal("expected busy=true after /kspec-qa with attachments")
	}
	if len(m.pendingAttachments) != 0 {
		t.Fatalf("pendingAttachments = %d, want 0 after the kickoff", len(m.pendingAttachments))
	}
	if len(m.sentAttachments) != 1 {
		t.Fatalf("sentAttachments = %d, want 1 during the kickoff turn", len(m.sentAttachments))
	}
	if got := ag.ActiveSkill(); got != "kspec-qa" {
		t.Fatalf("active skill = %q, want kspec-qa", got)
	}

	done := waitForAgentEvent(t, ag.Events, agent.EventTurnDone)
	m = step(t, m, agentEventMsg{event: done})

	var body string
	select {
	case body = <-bodies:
	default:
		t.Fatal("gateway captured no request body")
	}
	if !strings.Contains(body, `"content":[{"type":"text","text":"review this screenshot"},{"type":"image_url","image_url":{"url":"data:image/png;base64,`) {
		t.Fatalf("kickoff must send text + image parts:\n%s", body)
	}
	plain := stripANSI(m.contentRaw)
	if !strings.Contains(plain, "▣ shot.png ×") {
		t.Fatalf("user block missing the attachment chip:\n%s", plain)
	}
}

func TestKspecHelpLineTruncatesByRunes(t *testing.T) {
	sk := kspec.Skill{
		Name:        "kspec-long",
		ArgHint:     "<uma ideia grande de aplicação> — hint comprido demais com acentuação",
		Description: strings.Repeat("descrição ", 30),
	}
	line := kspecHelpLine(sk)
	if got := len([]rune(line)); got > maxHelpLineRunes {
		t.Fatalf("help line = %d runes, want <= %d: %q", got, maxHelpLineRunes, line)
	}
	if !strings.HasSuffix(line, "…") {
		t.Fatalf("truncated line must end with ellipsis: %q", line)
	}

	short := kspecHelpLine(kspec.Skill{Name: "kspec-a", Description: "do thing"})
	if short != "/kspec-a — do thing" {
		t.Fatalf("short line must not truncate: %q", short)
	}
}

func newModelCommandModel(t *testing.T) (Model, *agent.Agent) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("KTERMINAL_MODEL", "")
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	cfg := &config.Config{}
	cfg.LLM.BaseURL = "http://localhost:4000"
	cfg.LLM.APIKey = "gw-key"
	ag := agent.New(nil, nil, nil, cat, tools.NewRegistry(), nil, false)
	return New(ag, cfg, cat, true), ag
}

func seedPinnedConfig(t *testing.T, m Model, ag *agent.Agent, name string) {
	t.Helper()
	m.cfg.Model = name
	if err := m.cfg.Save(); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	ag.SetPinned(name)
}

func pickerIndexOf(m Model, name string) int {
	for i, choice := range m.modelChoices() {
		if choice.model == name {
			return i
		}
	}
	return -1
}

func TestModelCommandPersistsPinAndRewires(t *testing.T) {
	m, ag := newModelCommandModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m = runCommand(t, m, "/model glm-5.3")

	if got := ag.Pinned(); got != "glm-5.3" {
		t.Fatalf("pinned = %q, want glm-5.3", got)
	}
	if m.cfg.Model != "glm-5.3" {
		t.Fatalf("cfg.Model = %q, want glm-5.3", m.cfg.Model)
	}
	reloaded, err := config.Load()
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if reloaded.Model != "glm-5.3" {
		t.Fatalf("reloaded model = %q, want glm-5.3 (must survive restart)", reloaded.Model)
	}
	if _, ok := ag.Router.(*router.FixedRouter); !ok {
		t.Fatalf("router = %T, want *router.FixedRouter without a typesafe key", ag.Router)
	}
	if ag.Fallback != nil {
		t.Fatalf("fallback = %T, want nil with the fixed router", ag.Fallback)
	}
	plain := stripANSI(m.contentRaw)
	if !strings.Contains(plain, "model pinned: glm-5.3") {
		t.Fatalf("chat missing the pin confirmation:\n%s", plain)
	}
	if !strings.Contains(plain, "config saved") {
		t.Fatalf("chat missing the save confirmation:\n%s", plain)
	}
}

func TestModelCommandKeepsJevRoutingWithPin(t *testing.T) {
	m, ag := newModelCommandModel(t)
	m.cfg.Typesafe.APIKey = "jev-key"
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m = runCommand(t, m, "/model glm-5.3")

	if got := ag.Pinned(); got != "glm-5.3" {
		t.Fatalf("pinned = %q, want glm-5.3 (fixed model wins with Jev on)", got)
	}
	if _, ok := ag.Router.(*router.JevRouter); !ok {
		t.Fatalf("router = %T, want *router.JevRouter with Jev enabled", ag.Router)
	}
	if _, ok := ag.Fallback.(*router.HeuristicRouter); !ok {
		t.Fatalf("fallback = %T, want *router.HeuristicRouter with Jev enabled", ag.Fallback)
	}
	reloaded, err := config.Load()
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if reloaded.Model != "glm-5.3" {
		t.Fatalf("reloaded model = %q, want glm-5.3", reloaded.Model)
	}
}

func TestModelCommandUnknownModelKeepsConfig(t *testing.T) {
	m, ag := newModelCommandModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m = runCommand(t, m, "/model nope-model")

	if got := ag.Pinned(); got != "" {
		t.Fatalf("unknown model must not pin, got %q", got)
	}
	if m.cfg.Model != "" {
		t.Fatalf("unknown model must not touch cfg.Model, got %q", m.cfg.Model)
	}
	if _, err := os.Stat(config.Path()); err == nil {
		t.Fatal("unknown model must not persist the config file")
	}
	plain := stripANSI(m.contentRaw)
	if !strings.Contains(plain, "unknown model nope-model — not in catalog") {
		t.Fatalf("chat missing the error box:\n%s", plain)
	}
	if strings.Contains(plain, "model pinned") || strings.Contains(plain, "config saved") {
		t.Fatalf("unknown model must not confirm anything:\n%s", plain)
	}
}

func TestModelAutoClearsPersistedModel(t *testing.T) {
	m, ag := newModelCommandModel(t)
	seedPinnedConfig(t, m, ag, "glm-5.3")
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m = runCommand(t, m, "/model auto")

	if got := ag.Pinned(); got != "" {
		t.Fatalf("pinned = %q, want empty after auto", got)
	}
	if m.cfg.Model != "" {
		t.Fatalf("cfg.Model = %q, want empty after auto", m.cfg.Model)
	}
	reloaded, err := config.Load()
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if reloaded.Model != "" {
		t.Fatalf("reloaded model = %q, want empty (auto clears the field)", reloaded.Model)
	}
	plain := stripANSI(m.contentRaw)
	if !strings.Contains(plain, "model auto") || !strings.Contains(plain, "config saved") {
		t.Fatalf("chat missing the auto confirmation:\n%s", plain)
	}
}

func TestModelCommandWithoutArgsOpensPicker(t *testing.T) {
	m, ag := newModelCommandModel(t)
	ag.SetPinned("glm-5.3")
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m = runCommand(t, m, "/model")

	if !m.modelOpen {
		t.Fatal("/model without args must open the picker")
	}
	if got := ag.Pinned(); got != "glm-5.3" {
		t.Fatalf("opening the picker must not remove the pin, got %q", got)
	}
	choices := m.modelChoices()
	if len(choices) != len(m.catalog.Models)+1 {
		t.Fatalf("choices = %d, want %d (auto + catalog)", len(choices), len(m.catalog.Models)+1)
	}
	if choices[0].model != "" || choices[0].label != "auto" {
		t.Fatalf("first choice = %+v, want the auto entry", choices[0])
	}
	for i, mm := range m.catalog.Models {
		if choices[i+1].model != mm.Name {
			t.Fatalf("choice %d = %q, want catalog model %q", i+1, choices[i+1].model, mm.Name)
		}
	}
	if !strings.Contains(choices[1].label, "$") || !strings.Contains(choices[1].label, "tok/s") {
		t.Fatalf("choice label missing the price/speed summary: %q", choices[1].label)
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "MODEL") {
		t.Fatalf("view missing the picker header:\n%s", view)
	}
	if !strings.Contains(view, "glm-5.2 · $1.00/$4.00 · ~70 tok/s") {
		t.Fatalf("view missing the model summary line:\n%s", view)
	}
	if _, err := os.Stat(config.Path()); err == nil {
		t.Fatal("opening the picker must not persist the config file")
	}
}

func TestModelPickerPreSelectsPinnedModel(t *testing.T) {
	m, ag := newModelCommandModel(t)
	ag.SetPinned("glm-5.3")
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m = runCommand(t, m, "/model")

	want := pickerIndexOf(m, "glm-5.3")
	if want < 0 {
		t.Fatal("glm-5.3 missing from the picker choices")
	}
	if m.modelSelected != want {
		t.Fatalf("selected = %d, want %d (pin glm-5.3 pre-selected)", m.modelSelected, want)
	}
	if !strings.Contains(stripANSI(m.View()), "▸ glm-5.3") {
		t.Fatalf("view missing the pre-selected pin marker:\n%s", stripANSI(m.View()))
	}

	m2, _ := newModelCommandModel(t)
	m2 = step(t, m2, tea.WindowSizeMsg{Width: 100, Height: 30})
	m2 = runCommand(t, m2, "/model")
	if m2.modelSelected != 0 {
		t.Fatalf("selected = %d, want 0 (auto) without a pin", m2.modelSelected)
	}
}

func TestModelPickerEscKeepsConfig(t *testing.T) {
	m, ag := newModelCommandModel(t)
	seedPinnedConfig(t, m, ag, "glm-5.3")
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m = runCommand(t, m, "/model")
	if !m.modelOpen {
		t.Fatal("expected picker open before esc")
	}
	before, err := os.ReadFile(config.Path())
	if err != nil {
		t.Fatalf("read seeded config: %v", err)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	if m.modelOpen {
		t.Fatal("esc must close the picker")
	}
	if m.modelSelected != 0 {
		t.Fatalf("selection must reset after esc, got %d", m.modelSelected)
	}
	if got := ag.Pinned(); got != "glm-5.3" {
		t.Fatalf("esc must keep the pin, got %q", got)
	}
	after, err := os.ReadFile(config.Path())
	if err != nil {
		t.Fatalf("read config after esc: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("esc must not rewrite the config file:\nbefore: %q\nafter:  %q", string(before), string(after))
	}
}

func TestModelPickerEnterPersistsSelection(t *testing.T) {
	m, ag := newModelCommandModel(t)
	seedPinnedConfig(t, m, ag, "glm-5.3")
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m = runCommand(t, m, "/model")
	start := m.modelSelected
	m = step(t, m, tea.KeyMsg{Type: tea.KeyDown})
	moved := m.modelSelected
	if moved == start {
		t.Fatal("down must move the selection before enter")
	}
	want := m.modelChoices()[moved].model

	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.modelOpen {
		t.Fatal("enter must close the picker")
	}
	if got := ag.Pinned(); got != want {
		t.Fatalf("pinned = %q, want %q from the picker selection", got, want)
	}
	if m.cfg.Model != want {
		t.Fatalf("cfg.Model = %q, want %q", m.cfg.Model, want)
	}
	reloaded, err := config.Load()
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if reloaded.Model != want {
		t.Fatalf("reloaded model = %q, want %q (selection must survive restart)", reloaded.Model, want)
	}
	if _, ok := ag.Router.(*router.FixedRouter); !ok {
		t.Fatalf("router = %T, want *router.FixedRouter re-wired after the selection", ag.Router)
	}
	if ag.Fallback != nil {
		t.Fatalf("fallback = %T, want nil with the fixed router", ag.Fallback)
	}
	plain := stripANSI(m.contentRaw)
	if !strings.Contains(plain, "model pinned: "+want) || !strings.Contains(plain, "config saved") {
		t.Fatalf("chat missing the selection confirmation:\n%s", plain)
	}
}

func TestModelPickerEnterOnAutoClearsModel(t *testing.T) {
	m, ag := newModelCommandModel(t)
	seedPinnedConfig(t, m, ag, "glm-5.3")
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m = runCommand(t, m, "/model")
	start := m.modelSelected
	for range start {
		m = step(t, m, tea.KeyMsg{Type: tea.KeyUp})
	}
	if m.modelSelected != 0 {
		t.Fatalf("navigating up from the pre-selected pin must reach auto, got %d", m.modelSelected)
	}
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if got := ag.Pinned(); got != "" {
		t.Fatalf("pinned = %q, want empty after choosing auto", got)
	}
	reloaded, err := config.Load()
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if reloaded.Model != "" {
		t.Fatalf("reloaded model = %q, want empty after choosing auto", reloaded.Model)
	}
}

func newJevCommandModel(t *testing.T, gwURL, jevURL, jevKey string) (Model, *agent.Agent) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("KTERMINAL_MODEL", "")
	t.Setenv("KTERMINAL_TYPESAFE_ENABLED", "")
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	cfg := &config.Config{}
	cfg.LLM.BaseURL = gwURL
	cfg.LLM.APIKey = "gw-key"
	cfg.Typesafe.APIKey = jevKey
	primary, fallback := router.Wire(jevKey, cfg.Typesafe.JevEnabled(), cfg.Model, cat)
	if jr, ok := primary.(*router.JevRouter); ok {
		jr.Client.BaseURL = jevURL
	}
	ag := agent.New(llm.New(gwURL, "gw-key", false), primary, fallback, cat, tools.NewRegistry(), nil, false)
	return New(ag, cfg, cat, true), ag
}

func countingJev(t *testing.T, choice string, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/systemone", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		json.NewEncoder(w).Encode(map[string]any{
			"model": "jev-test",
			"answers": map[string]any{
				"model_choice": map[string]any{
					"type":          "choice",
					"choice":        choice,
					"confidence":    0.9,
					"probabilities": map[string]float64{choice: 0.9},
				},
			},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestRouterJevOnWithoutAPIKeyKeepsConfig(t *testing.T) {
	gw, _ := captureGateway(t)
	m, ag := newJevCommandModel(t, gw.URL, "http://jev.unused", "")
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	if err := m.cfg.Save(); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	before, err := os.ReadFile(config.Path())
	if err != nil {
		t.Fatalf("read seeded config: %v", err)
	}
	routerBefore := ag.Router

	m = runCommand(t, m, "/router on")

	after, err := os.ReadFile(config.Path())
	if err != nil {
		t.Fatalf("read config after /router on: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("config file must stay unchanged:\nbefore: %q\nafter:  %q", string(before), string(after))
	}
	if ag.Router == nil || routerBefore == nil {
		t.Fatalf("routers must stay non-nil: %T / %T", ag.Router, routerBefore)
	}
	if _, ok := ag.Router.(*router.FixedRouter); !ok {
		t.Fatalf("router must stay *router.FixedRouter, got %T", ag.Router)
	}
	plain := stripANSI(m.contentRaw)
	if !strings.Contains(plain, "jev needs a typesafe api key") {
		t.Fatalf("chat missing the api key error:\n%s", plain)
	}
	if strings.Contains(plain, "config saved") || strings.Contains(plain, "jev on") {
		t.Fatalf("chat must not confirm anything:\n%s", plain)
	}
}

func TestRouterOffPersistsAndOpensPicker(t *testing.T) {
	gw, _ := captureGateway(t)
	var hits atomic.Int32
	jevSrv := countingJev(t, "glm-5.2", &hits)
	m, ag := newJevCommandModel(t, gw.URL, jevSrv.URL, "jev-key")
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m = runCommand(t, m, "/router off")

	if !m.modelOpen {
		t.Fatal("/router off must open the model picker")
	}
	if m.cfg.Router.RoutingEnabled() {
		t.Fatal("routing must be disabled in memory after /router off")
	}
	if _, ok := ag.Router.(*router.FixedRouter); !ok {
		t.Fatalf("router = %T, want *router.FixedRouter after /router off", ag.Router)
	}
	if ag.Fallback != nil {
		t.Fatalf("fallback = %T, want nil with the fixed router", ag.Fallback)
	}
	reloaded, err := config.Load()
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if reloaded.Router.RoutingEnabled() {
		t.Fatal("routing disabled must survive a restart")
	}
	plain := stripANSI(m.contentRaw)
	if !strings.Contains(plain, "jev off") || !strings.Contains(plain, "config saved") {
		t.Fatalf("chat missing the off confirmation:\n%s", plain)
	}
}

func TestRouterOffStopsJevRequestsInNextTurn(t *testing.T) {
	gw, _ := captureGateway(t)
	var hits atomic.Int32
	jevSrv := countingJev(t, "glm-5.2", &hits)
	m, ag := newJevCommandModel(t, gw.URL, jevSrv.URL, "jev-key")
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m.input.SetValue("first question")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	route := waitForAgentEvent(t, ag.Events, agent.EventRoute)
	if route.Router != "jev" {
		t.Fatalf("first turn must route via jev, got %q", route.Router)
	}
	done := waitForAgentEvent(t, ag.Events, agent.EventTurnDone)
	m = step(t, m, agentEventMsg{event: done})
	hitsAfterFirst := hits.Load()
	if hitsAfterFirst == 0 {
		t.Fatal("precondition: the jev mock must have been called at least once")
	}

	m = runCommand(t, m, "/router off")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.modelOpen {
		t.Fatal("esc must close the picker opened by /router off")
	}

	m.input.SetValue("second question")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	route = waitForAgentEvent(t, ag.Events, agent.EventRoute)
	if route.Router != "fixed" {
		t.Fatalf("turn after /router off must route via fixed, got %q", route.Router)
	}
	if route.Model != m.catalog.DefaultModel {
		t.Fatalf("model = %q, want the catalog default %q (esc keeps auto)", route.Model, m.catalog.DefaultModel)
	}
	done = waitForAgentEvent(t, ag.Events, agent.EventTurnDone)
	m = step(t, m, agentEventMsg{event: done})
	if got := hits.Load(); got != hitsAfterFirst {
		t.Fatalf("jev mock hits = %d, want %d (no requests after /router off)", got, hitsAfterFirst)
	}
}

func TestRouterOffThenModelCommandPersistsBothFields(t *testing.T) {
	gw, _ := captureGateway(t)
	m, ag := newJevCommandModel(t, gw.URL, "http://jev.unused", "jev-key")
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m = runCommand(t, m, "/router off")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	m = runCommand(t, m, "/model glm-5.3")

	reloaded, err := config.Load()
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if reloaded.Router.RoutingEnabled() {
		t.Fatal("routing disabled must survive /router off followed by /model")
	}
	if reloaded.Model != "glm-5.3" {
		t.Fatalf("reloaded model = %q, want glm-5.3 (both fields must survive a restart)", reloaded.Model)
	}
	if got := ag.Pinned(); got != "glm-5.3" {
		t.Fatalf("pinned = %q, want glm-5.3", got)
	}
	if _, ok := ag.Router.(*router.FixedRouter); !ok {
		t.Fatalf("router = %T, want *router.FixedRouter after /router off + /model", ag.Router)
	}
	if ag.Fallback != nil {
		t.Fatalf("fallback = %T, want nil with Jev disabled", ag.Fallback)
	}
}

func TestRouterOnReinstallsActiveEngineRouter(t *testing.T) {
	gw, _ := captureGateway(t)
	m, ag := newJevCommandModel(t, gw.URL, "http://jev.unused", "jev-key")
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m = runCommand(t, m, "/router off")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if _, ok := ag.Router.(*router.FixedRouter); !ok {
		t.Fatalf("precondition: router = %T, want *router.FixedRouter after /router off", ag.Router)
	}

	m = runCommand(t, m, "/router on")

	if _, ok := ag.Router.(*router.JevRouter); !ok {
		t.Fatalf("router = %T, want *router.JevRouter reinstalled by /router on", ag.Router)
	}
	if _, ok := ag.Fallback.(*router.HeuristicRouter); !ok {
		t.Fatalf("fallback = %T, want *router.HeuristicRouter with Jev on", ag.Fallback)
	}
	reloaded, err := config.Load()
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if !reloaded.Router.RoutingEnabled() {
		t.Fatal("routing enabled must be persisted by /router on")
	}
	plain := stripANSI(m.contentRaw)
	if !strings.Contains(plain, "jev on") || !strings.Contains(plain, "config saved") {
		t.Fatalf("chat missing the on confirmation:\n%s", plain)
	}
}

func TestRouterOffIdempotent(t *testing.T) {
	gw, _ := captureGateway(t)
	m, ag := newJevCommandModel(t, gw.URL, "http://jev.unused", "jev-key")
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m = runCommand(t, m, "/router off")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	m = runCommand(t, m, "/router off")

	if !m.modelOpen {
		t.Fatal("second /router off must open the picker again")
	}
	if _, ok := ag.Router.(*router.FixedRouter); !ok {
		t.Fatalf("router = %T, want *router.FixedRouter after the second /router off", ag.Router)
	}
	reloaded, err := config.Load()
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if reloaded.Router.RoutingEnabled() {
		t.Fatal("routing must stay disabled after the second /router off")
	}
	plain := stripANSI(m.contentRaw)
	if strings.Contains(plain, "failed to save config") {
		t.Fatalf("idempotent /router off must not error:\n%s", plain)
	}
	if !strings.Contains(plain, "config saved") {
		t.Fatalf("chat missing the save confirmation:\n%s", plain)
	}
}

func TestRouterStatusShowsStateAndModel(t *testing.T) {
	gw, _ := captureGateway(t)
	m, _ := newJevCommandModel(t, gw.URL, "http://jev.unused", "jev-key")
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m = runCommand(t, m, "/router")
	plain := stripANSI(m.contentRaw)
	if !strings.Contains(plain, "engine: jev") || !strings.Contains(plain, "router: on") {
		t.Fatalf("status must show engine jev on with a key:\n%s", plain)
	}
	if !strings.Contains(plain, "model: "+m.catalog.DefaultModel) {
		t.Fatalf("status must show the catalog default as the fixed model:\n%s", plain)
	}

	m = runCommand(t, m, "/model glm-5.3")
	m = runCommand(t, m, "/router")
	plain = stripANSI(m.contentRaw)
	if !strings.Contains(plain, "model: glm-5.3") {
		t.Fatalf("status must show the configured fixed model:\n%s", plain)
	}

	m = runCommand(t, m, "/router off")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	m = runCommand(t, m, "/router")
	plain = stripANSI(m.contentRaw)
	if !strings.Contains(plain, "router: off") {
		t.Fatalf("status must show router off after /router off:\n%s", plain)
	}

	m2, _ := newJevCommandModel(t, gw.URL, "http://jev.unused", "")
	m2 = step(t, m2, tea.WindowSizeMsg{Width: 100, Height: 30})
	m2 = runCommand(t, m2, "/router")
	if plain := stripANSI(m2.contentRaw); !strings.Contains(plain, "router: off") {
		t.Fatalf("status must show router off without an api key:\n%s", plain)
	}
}

func TestRouterUnknownStateErrors(t *testing.T) {
	gw, _ := captureGateway(t)
	m, _ := newJevCommandModel(t, gw.URL, "http://jev.unused", "jev-key")
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m = runCommand(t, m, "/router toggle")

	if _, err := os.Stat(config.Path()); err == nil {
		t.Fatal("unknown state must not persist the config file")
	}
	plain := stripANSI(m.contentRaw)
	if !strings.Contains(plain, "unknown router command toggle") {
		t.Fatalf("chat missing the unknown state error:\n%s", plain)
	}
}

func newConfigScreenModel(t *testing.T) (Model, *agent.Agent) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("KTERMINAL_MODEL", "")
	t.Setenv("KTERMINAL_TYPESAFE_ENABLED", "")
	t.Setenv("TYPESAFE_API_KEY", "")
	t.Setenv("KTERMINAL_LLM_BASE_URL", "")
	t.Setenv("KTERMINAL_LLM_API_KEY", "")
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	cfg := &config.Config{}
	cfg.LLM.BaseURL = "http://localhost:4000"
	cfg.LLM.APIKey = "gw-key"
	cfg.Typesafe.APIKey = "jev-key"
	ag := agent.New(llm.New(cfg.LLM.BaseURL, cfg.LLM.APIKey, false), nil, nil, cat, tools.NewRegistry(), nil, false)
	return New(ag, cfg, cat, true), ag
}

func TestConfigInvalidModelKeepsScreenAndFile(t *testing.T) {
	m, ag := newConfigScreenModel(t)
	if err := m.cfg.Save(); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	before, err := os.ReadFile(config.Path())
	if err != nil {
		t.Fatalf("read seeded config: %v", err)
	}
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	enabledBefore := m.cfg.Typesafe.Enabled

	m = runCommand(t, m, "/config")
	if m.state != stateConfig {
		t.Fatalf("state = %q, want config", m.state)
	}
	if len(m.cfgInputs) != 7 {
		t.Fatalf("inputs = %d, want 7", len(m.cfgInputs))
	}
	if got := m.cfgInputs[4].Value(); got != "yes" {
		t.Fatalf("jev enabled input = %q, want yes (default)", got)
	}
	m.cfgInputs[5].SetValue("nope-model")
	for range len(m.cfgInputs) {
		m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	}

	if m.state != stateConfig {
		t.Fatalf("state = %q, want config (invalid model must keep the screen)", m.state)
	}
	after, err := os.ReadFile(config.Path())
	if err != nil {
		t.Fatalf("read config after failed save: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("config file must stay unchanged:\nbefore: %q\nafter:  %q", string(before), string(after))
	}
	if m.cfg.Model != "" {
		t.Fatalf("cfg.Model = %q, want empty (invalid model must not leak into memory)", m.cfg.Model)
	}
	if m.cfg.Typesafe.Enabled != enabledBefore {
		t.Fatalf("invalid model must not mutate enabled, got %+v", m.cfg.Typesafe.Enabled)
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "unknown model nope-model — not in catalog") {
		t.Fatalf("config view missing the error box:\n%s", view)
	}
	if got := ag.Pinned(); got != "" {
		t.Fatalf("pinned = %q, want empty (no re-wiring on invalid model)", got)
	}

	m.cfgInputs[5].SetValue("glm-5.3")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != stateChat {
		t.Fatalf("state = %q, want chat after fixing the model", m.state)
	}
	if strings.Contains(stripANSI(m.View()), "unknown model") {
		t.Fatal("error must clear after a successful save")
	}
	if got := ag.Pinned(); got != "glm-5.3" {
		t.Fatalf("pinned = %q, want glm-5.3 after the recovery save", got)
	}
}

func TestConfigSavePersistsJevAndModel(t *testing.T) {
	m, ag := newConfigScreenModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m = runCommand(t, m, "/config")
	m.cfgInputs[4].SetValue("no")
	m.cfgInputs[5].SetValue("glm-5.3")
	for range len(m.cfgInputs) {
		m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	}

	if m.state != stateChat {
		t.Fatalf("state = %q, want chat after save", m.state)
	}
	reloaded, err := config.Load()
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if reloaded.Typesafe.JevEnabled() {
		t.Fatal("enabled must be false after saving no")
	}
	if reloaded.Model != "glm-5.3" {
		t.Fatalf("reloaded model = %q, want glm-5.3", reloaded.Model)
	}
	if reloaded.LLM.BaseURL != "http://localhost:4000" || reloaded.LLM.APIKey != "gw-key" {
		t.Fatalf("existing llm fields broken: %+v", reloaded.LLM)
	}
	if reloaded.Typesafe.APIKey != "jev-key" {
		t.Fatalf("typesafe api key = %q, want jev-key", reloaded.Typesafe.APIKey)
	}
	if _, ok := ag.Router.(*router.FixedRouter); !ok {
		t.Fatalf("router = %T, want *router.FixedRouter after saving with jev disabled", ag.Router)
	}
	if ag.Fallback != nil {
		t.Fatalf("fallback = %T, want nil with the fixed router", ag.Fallback)
	}
	if got := ag.Pinned(); got != "glm-5.3" {
		t.Fatalf("pinned = %q, want glm-5.3 (rebuildAgent applies the pin)", got)
	}
	plain := stripANSI(m.contentRaw)
	if !strings.Contains(plain, "config saved") {
		t.Fatalf("chat missing the save confirmation:\n%s", plain)
	}
}

func TestConfigSaveMaterializesLegacyFields(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("KTERMINAL_MODEL", "")
	t.Setenv("KTERMINAL_TYPESAFE_ENABLED", "")
	t.Setenv("TYPESAFE_API_KEY", "")
	if err := os.MkdirAll(config.Dir(), 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	legacy := "[llm]\nbase_url = \"http://localhost:4000\"\napi_key = \"gw-key\"\n\n[typesafe]\napi_key = \"jev-key\"\n"
	if err := os.WriteFile(config.Path(), []byte(legacy), 0o600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load legacy config: %v", err)
	}
	if cfg.Typesafe.Enabled != nil || cfg.Model != "" {
		t.Fatalf("legacy config must load without the new fields: %+v", cfg)
	}
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	ag := agent.New(llm.New(cfg.LLM.BaseURL, cfg.LLM.APIKey, false), nil, nil, cat, tools.NewRegistry(), nil, false)
	m := New(ag, cfg, cat, true)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m = runCommand(t, m, "/config")
	if got := m.cfgInputs[4].Value(); got != "yes" {
		t.Fatalf("jev enabled input = %q, want yes (nil enabled defaults to true)", got)
	}
	if got := m.cfgInputs[5].Value(); got != "" {
		t.Fatalf("model input = %q, want empty for a legacy config", got)
	}
	for range len(m.cfgInputs) {
		m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	}

	if m.state != stateChat {
		t.Fatalf("state = %q, want chat after save", m.state)
	}
	raw, err := os.ReadFile(config.Path())
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	if !strings.Contains(string(raw), "enabled = true") {
		t.Fatalf("saved config must materialize enabled = true:\n%s", string(raw))
	}
	if !strings.Contains(string(raw), "model = \"\"") {
		t.Fatalf("saved config must materialize the model field:\n%s", string(raw))
	}
	reloaded, err := config.Load()
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if !reloaded.Typesafe.JevEnabled() {
		t.Fatal("enabled must stay true after materialization")
	}
	if reloaded.Model != "" {
		t.Fatalf("model = %q, want empty", reloaded.Model)
	}
	if reloaded.LLM.BaseURL != "http://localhost:4000" || reloaded.LLM.APIKey != "gw-key" {
		t.Fatalf("legacy llm fields broken: %+v", reloaded.LLM)
	}
	if reloaded.Typesafe.APIKey != "jev-key" {
		t.Fatalf("legacy typesafe key broken: %q", reloaded.Typesafe.APIKey)
	}
}

func TestLegacyConfigKeepsJevActiveEndToEnd(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("KTERMINAL_MODEL", "")
	t.Setenv("KTERMINAL_TYPESAFE_ENABLED", "")
	t.Setenv("TYPESAFE_API_KEY", "")
	if err := os.MkdirAll(config.Dir(), 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	legacy := "[llm]\nbase_url = \"http://localhost:4000\"\napi_key = \"gw-key\"\n\n[typesafe]\napi_key = \"jev-key\"\n"
	if err := os.WriteFile(config.Path(), []byte(legacy), 0o600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load legacy config: %v", err)
	}
	if !cfg.Typesafe.JevEnabled() {
		t.Fatal("legacy config without enabled must keep Jev on")
	}
	if cfg.Model != "" {
		t.Fatalf("legacy model = %q, want empty", cfg.Model)
	}

	gw, _ := captureGateway(t)
	var hits atomic.Int32
	jevSrv := countingJev(t, "glm-5.2", &hits)
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	primary, fallback := router.Wire(cfg.Typesafe.APIKey, cfg.Typesafe.JevEnabled(), cfg.Model, cat)
	jr, ok := primary.(*router.JevRouter)
	if !ok {
		t.Fatalf("primary = %T, want *router.JevRouter for a legacy config with an api key", primary)
	}
	jr.Client.BaseURL = jevSrv.URL
	if _, ok := fallback.(*router.HeuristicRouter); !ok {
		t.Fatalf("fallback = %T, want *router.HeuristicRouter with Jev active", fallback)
	}

	ag := agent.New(llm.New(gw.URL, "gw-key", false), primary, fallback, cat, tools.NewRegistry(), nil, false)
	m := New(ag, cfg, cat, true)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m.input.SetValue("hello")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	route := waitForAgentEvent(t, ag.Events, agent.EventRoute)
	if route.Router != "jev" {
		t.Fatalf("route router = %q, want jev — legacy config must keep the current behavior", route.Router)
	}
	if route.Model != "glm-5.2" {
		t.Fatalf("route model = %q, want the jev choice glm-5.2", route.Model)
	}
	done := waitForAgentEvent(t, ag.Events, agent.EventTurnDone)
	m = step(t, m, agentEventMsg{event: done})
	if hits.Load() == 0 {
		t.Fatal("jev mock must receive the routing request for a legacy config with an api key")
	}
}

func TestHelpDocumentsRouterAndPersistentModel(t *testing.T) {
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = runCommand(t, m, "/help")

	plain := stripANSI(m.contentRaw)
	if !strings.Contains(plain, "/router [status|jev|laya|on|off]") {
		t.Fatalf("help missing the /router line:\n%s", plain)
	}
	if !strings.Contains(plain, "persisted across restarts") {
		t.Fatalf("help missing the persistent /model description:\n%s", plain)
	}
}

func TestCommandPopupSuggestsRouter(t *testing.T) {
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/ro")})

	if !m.cmdOpen {
		t.Fatal("expected popup open for /ro")
	}
	if strings.Join(m.cmdItems, "|") != "/router" {
		t.Fatalf("items = %v, want [/router]", m.cmdItems)
	}
}

func TestRouterLayaWithoutEngineKeepsConfig(t *testing.T) {
	gw, _ := captureGateway(t)
	m, ag := newJevCommandModel(t, gw.URL, "http://jev.unused", "jev-key")
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	if err := m.cfg.Save(); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	before, err := os.ReadFile(config.Path())
	if err != nil {
		t.Fatalf("read seeded config: %v", err)
	}

	m = runCommand(t, m, "/router laya")

	after, err := os.ReadFile(config.Path())
	if err != nil {
		t.Fatalf("read config after /router laya: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("config must stay unchanged without the laya engine:\nbefore: %q\nafter:  %q", string(before), string(after))
	}
	if m.cfg.Router.Engine != "jev" {
		t.Fatalf("engine = %q, want jev untouched", m.cfg.Router.Engine)
	}
	if _, ok := ag.Router.(*router.JevRouter); !ok {
		t.Fatalf("router = %T, want *router.JevRouter untouched", ag.Router)
	}
	plain := stripANSI(m.contentRaw)
	if !strings.Contains(plain, "laya engine ainda não disponível") {
		t.Fatalf("chat missing the laya unavailable error:\n%s", plain)
	}
	if strings.Contains(plain, "config saved") {
		t.Fatalf("chat must not confirm a save:\n%s", plain)
	}
}

type stubLayaEngine struct{}

func (stubLayaEngine) Predict(context.Context, string, map[string]laya.Question) (map[string]laya.Answer, error) {
	return nil, errors.New("no inference in this stub")
}

func (stubLayaEngine) Status() laya.Status { return laya.Status{Ready: true} }

func (stubLayaEngine) EnsureReady(context.Context, func(laya.Progress)) error { return nil }

func TestRouterLayaWithEngineSwitchesAndPersists(t *testing.T) {
	gw, _ := captureGateway(t)
	m, ag := newJevCommandModel(t, gw.URL, "http://jev.unused", "jev-key")
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m.laya = stubLayaEngine{}

	m = runCommand(t, m, "/router laya")

	if m.cfg.Router.Engine != "laya" {
		t.Fatalf("engine = %q, want laya", m.cfg.Router.Engine)
	}
	if _, ok := ag.Router.(*router.LayaRouter); !ok {
		t.Fatalf("router = %T, want *router.LayaRouter after the switch", ag.Router)
	}
	if _, ok := ag.Fallback.(*router.HeuristicRouter); !ok {
		t.Fatalf("fallback = %T, want *router.HeuristicRouter with laya active", ag.Fallback)
	}
	reloaded, err := config.Load()
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if reloaded.Router.Engine != "laya" {
		t.Fatalf("reloaded engine = %q, want laya persisted", reloaded.Router.Engine)
	}
	plain := stripANSI(m.contentRaw)
	if !strings.Contains(plain, "engine laya · router on") || !strings.Contains(plain, "config saved") {
		t.Fatalf("chat missing the switch confirmation:\n%s", plain)
	}

	m = runCommand(t, m, "/router jev")
	if m.cfg.Router.Engine != "jev" {
		t.Fatalf("engine = %q, want jev after switching back", m.cfg.Router.Engine)
	}
	if _, ok := ag.Router.(*router.JevRouter); !ok {
		t.Fatalf("router = %T, want *router.JevRouter after switching back", ag.Router)
	}
	reloaded, err = config.Load()
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if reloaded.Router.Engine != "jev" {
		t.Fatalf("reloaded engine = %q, want jev persisted", reloaded.Router.Engine)
	}
}

func TestRouterOnWithLayaEngineNeedsEngineInstalled(t *testing.T) {
	gw, _ := captureGateway(t)
	m, ag := newJevCommandModel(t, gw.URL, "http://jev.unused", "jev-key")
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m.cfg.Router.Engine = "laya"

	m = runCommand(t, m, "/router on")

	if m.cfg.Router.RoutingEnabled() != true {
		t.Fatal("precondition broken")
	}
	if _, ok := ag.Router.(*router.JevRouter); !ok {
		t.Fatalf("router = %T, must stay untouched when laya is not installed", ag.Router)
	}
	plain := stripANSI(m.contentRaw)
	if !strings.Contains(plain, "laya engine ainda não disponível") {
		t.Fatalf("chat missing the laya unavailable error:\n%s", plain)
	}
}

func TestRouterStatusShowsLayaEngine(t *testing.T) {
	gw, _ := captureGateway(t)
	m, _ := newJevCommandModel(t, gw.URL, "http://jev.unused", "")
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m.laya = stubLayaEngine{}
	m.cfg.Router.Engine = "laya"

	m = runCommand(t, m, "/router")

	plain := stripANSI(m.contentRaw)
	if !strings.Contains(plain, "engine: laya") || !strings.Contains(plain, "router: on") {
		t.Fatalf("status must show engine laya on with the engine installed:\n%s", plain)
	}
}
