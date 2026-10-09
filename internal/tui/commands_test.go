package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"kterminal/internal/agent"
	"kterminal/internal/catalog"
	"kterminal/internal/config"
	"kterminal/internal/kspec"
	"kterminal/internal/llm"
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
	want := []string{"/clear", "/config", "/exit", "/help", "/image", "/kspec-bootstrap", "/kspec-bugfix", "/kspec-ideia", "/kspec-implement", "/kspec-pr-review", "/kspec-prd", "/kspec-qa", "/kspec-tasks", "/kspec-techspec", "/kspec-version", "/mode", "/model", "/quit", "/unimage"}
	if strings.Join(m.cmdItems, "|") != strings.Join(want, "|") {
		t.Fatalf("items = %v, want %v", m.cmdItems, want)
	}
	if m.cmdSelected != 0 {
		t.Fatalf("selected = %d, want 0 on open", m.cmdSelected)
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "▸ /clear") {
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
	want := []string{"/clear", "/config", "/exit", "/help", "/image", "/mode", "/model", "/quit", "/unimage"}
	if strings.Join(m.cmdItems, "|") != strings.Join(want, "|") {
		t.Fatalf("items = %v, want %v", m.cmdItems, want)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if m.cmdOpen {
		t.Fatal("no store means no /kspec-* suggestions")
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
	if hint := stripANSI(m.hintBar()); !strings.Contains(hint, "mode squad") {
		t.Fatalf("hint bar missing the mode:\n%s", hint)
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
	if !strings.Contains(plain, "🖼 shot.png ×") {
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
