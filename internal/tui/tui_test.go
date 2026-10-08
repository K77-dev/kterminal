package tui

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/rivo/uniseg"

	"kterminal/internal/agent"
	"kterminal/internal/catalog"
	"kterminal/internal/config"
	"kterminal/internal/llm"
	"kterminal/internal/session"
	"kterminal/internal/tools"
)

func newTestModel(t *testing.T, opts ...Option) Model {
	t.Helper()
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	cfg := &config.Config{}
	ag := agent.New(nil, nil, nil, cat, nil, nil, false)
	return New(ag, cfg, cat, true, opts...)
}

func step(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(Model)
}

func TestStreamDeltasAcrossUpdateCopies(t *testing.T) {
	m := newTestModel(t)

	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventRoute, Model: "glm-5.2", Router: "jev", Confidence: 0.9}})
	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventDelta, Model: "glm-5.2", Text: "hel"}})
	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventDelta, Model: "glm-5.2", Text: "lo"}})

	next, cmd := m.Update(agentEventMsg{event: agent.Event{Kind: agent.EventTurnDone, Model: "glm-5.2", Text: "hello", SessionCost: 0.001, TPS: 90}})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected listen cmd after turn done")
	}

	content := m.vp.View()
	if !strings.Contains(content, "hello") {
		t.Fatalf("rendered content missing streamed text:\n%s", content)
	}
	if m.stream.Len() != 0 {
		t.Fatalf("stream not reset after turn done, len=%d", m.stream.Len())
	}
}

func TestScrollFollow(t *testing.T) {
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 120, Height: 20})

	for i := 0; i < 60; i++ {
		m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventRoute, Model: "glm-5.2", Router: "jev", Confidence: 0.9}})
	}
	if m.vp.YOffset == 0 {
		t.Fatal("expected content taller than viewport to scroll to bottom")
	}
	if !m.follow {
		t.Fatal("expected follow=true at bottom")
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyPgUp})
	if m.follow {
		t.Fatal("pgup should pause follow")
	}
	offset := m.vp.YOffset

	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventDelta, Model: "glm-5.2", Text: "more text"}})
	if m.vp.YOffset != offset {
		t.Fatalf("delta yanked viewport while scrolled: %d -> %d", offset, m.vp.YOffset)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnd})
	if !m.follow {
		t.Fatal("end should resume follow")
	}
	if !m.vp.AtBottom() {
		t.Fatal("end should jump to bottom")
	}
}

func TestToolEventsRender(t *testing.T) {
	m := newTestModel(t)

	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventToolStart, Tool: "bash", Args: `{"command":"echo hi"}`}})
	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventToolResult, Tool: "bash", Result: "hi"}})
	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventTurnDone, Model: "glm-5.2", Text: "done"}})

	content := m.vp.View()
	if !strings.Contains(content, "bash") {
		t.Fatalf("content missing tool call:\n%s", content)
	}
	if !strings.Contains(content, "hi") {
		t.Fatalf("content missing tool result:\n%s", content)
	}
}

func TestRouteLineShowsReason(t *testing.T) {
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventRoute, Model: "glm-5.2", Router: "jev", Confidence: 0.43, Reason: "low confidence, using default"}})
	plain := stripANSI(m.vp.View())
	if !strings.Contains(plain, "⚡ glm-5.2 · jev 0.43 · low confidence, using default") {
		t.Fatalf("route line missing reason:\n%s", plain)
	}

	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventRoute, Model: "deepseek-v4-flash", Router: "jev", Confidence: 0.66}})
	plain = stripANSI(m.vp.View())
	if !strings.Contains(plain, "⚡ deepseek-v4-flash · jev 0.66") {
		t.Fatalf("route line missing direct choice:\n%s", plain)
	}
	if strings.Contains(plain, "deepseek-v4-flash · jev 0.66 ·") {
		t.Fatalf("direct choice must not append a reason:\n%s", plain)
	}
}

func TestLiveBlockAccumulatesAndReplaces(t *testing.T) {
	forceTrueColor(t)
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.busy = true

	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventToolStart, Tool: "bash", Args: `{"command":"stream"}`}})
	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventToolOutput, Tool: "bash", Text: "line 1"}})
	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventToolOutput, Tool: "bash", Text: "line 2"}})
	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventToolOutput, Tool: "bash", Text: "line 3"}})

	content := m.vp.View()
	if !strings.Contains(content, "\x1b[38;2;128;128;128mline 1") {
		t.Fatalf("live block missing colTextMuted ANSI:\n%q", content)
	}
	plain := stripANSI(content)
	toolIdx := strings.Index(plain, `● bash({"command":"stream"})`)
	liveIdx := strings.Index(plain, "   line 1")
	if toolIdx < 0 || liveIdx < 0 {
		t.Fatalf("content missing tool line or live block:\n%s", plain)
	}
	if toolIdx > liveIdx {
		t.Fatalf("live block must render below the tool line:\n%s", plain)
	}
	for _, want := range []string{"   line 1", "   line 2", "   line 3"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("live block missing %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "… +") {
		t.Fatalf("3 live lines must not show an omitted counter:\n%s", plain)
	}
	if len(m.liveLines) != 3 || m.liveOmitted != 0 {
		t.Fatalf("liveLines = %v, liveOmitted = %d, want 3 lines 0 omitted", m.liveLines, m.liveOmitted)
	}

	burst := make([]string, 20)
	for i := range burst {
		burst[i] = fmt.Sprintf("out %02d", i)
	}
	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventToolOutput, Tool: "bash", Text: strings.Join(burst, "\n")}})

	content = m.vp.View()
	if !strings.Contains(content, "\x1b[38;2;128;128;128m… +8 lines") {
		t.Fatalf("omitted counter missing colTextMuted ANSI:\n%q", content)
	}
	plain = stripANSI(content)
	counterIdx := strings.Index(plain, "… +8 lines")
	firstKeptIdx := strings.Index(plain, "   out 05")
	if counterIdx < 0 || firstKeptIdx < 0 {
		t.Fatalf("live block missing counter or kept lines:\n%s", plain)
	}
	if counterIdx > firstKeptIdx {
		t.Fatalf("omitted counter must render at the top of the live block:\n%s", plain)
	}
	for _, want := range []string{"   out 05", "   out 12", "   out 19"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("sliding window missing %q:\n%s", plain, want)
		}
	}
	for _, hidden := range []string{"line 1", "line 3", "out 00", "out 04"} {
		if strings.Contains(plain, hidden) {
			t.Fatalf("sliding window must drop %q:\n%s", hidden, plain)
		}
	}
	if len(m.liveLines) != liveBlockMaxLines {
		t.Fatalf("liveLines = %d, want %d", len(m.liveLines), liveBlockMaxLines)
	}
	if m.liveOmitted != 8 {
		t.Fatalf("liveOmitted = %d, want 8", m.liveOmitted)
	}

	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventToolResult, Tool: "bash", Result: "consolidated\nresult"}})

	if len(m.liveLines) != 0 || m.liveOmitted != 0 {
		t.Fatalf("live block not discarded on tool result: lines=%v omitted=%d", m.liveLines, m.liveOmitted)
	}
	plain = stripANSI(m.vp.View())
	if !strings.Contains(plain, "  consolidated ⏎ result") {
		t.Fatalf("content missing consolidated result:\n%s", plain)
	}
	for _, gone := range []string{"out 05", "out 19", "… +8 lines"} {
		if strings.Contains(plain, gone) {
			t.Fatalf("live block must be replaced by the result, found %q:\n%s", gone, plain)
		}
	}
}

func TestSilentCommandNoLiveBlock(t *testing.T) {
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m.busy = true

	hintBefore := m.hintBar()
	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventToolStart, Tool: "bash", Args: `{"command":"sleep 30"}`}})

	if len(m.liveLines) != 0 || m.liveOmitted != 0 {
		t.Fatalf("tool start must open with an empty live block: lines=%v omitted=%d", m.liveLines, m.liveOmitted)
	}

	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventToolResult, Tool: "bash", Result: "done"}})

	if len(m.liveLines) != 0 || m.liveOmitted != 0 {
		t.Fatalf("silent command must leave no live block: lines=%v omitted=%d", m.liveLines, m.liveOmitted)
	}
	if got := len(strings.Split(strings.TrimRight(stripANSI(m.contentRaw), "\n"), "\n")); got != 3 {
		t.Fatalf("silent command content = %d lines, want 3 (tool line, blank, result):\n%s", got, stripANSI(m.contentRaw))
	}
	if !m.busy {
		t.Fatal("silent command must not change busy (spinner must keep running)")
	}
	if hint := m.hintBar(); hint != hintBefore {
		t.Fatalf("spinner hint changed:\nbefore: %q\nafter: %q", hintBefore, hint)
	}
	if !strings.Contains(m.View(), "Thinking") {
		t.Fatalf("spinner missing from view:\n%s", m.View())
	}
	plain := stripANSI(m.vp.View())
	if !strings.Contains(plain, `● bash({"command":"sleep 30"})`) {
		t.Fatalf("content missing tool line:\n%s", plain)
	}
	if !strings.Contains(plain, "  done") {
		t.Fatalf("content missing tool result:\n%s", plain)
	}
}

func TestLiveBlockEmptyLineRenders(t *testing.T) {
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m.busy = true

	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventToolStart, Tool: "bash", Args: `{"command":"echo"}`}})
	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventToolOutput, Tool: "bash", Text: ""}})
	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventToolOutput, Tool: "bash", Text: "\n"}})

	if len(m.liveLines) != 3 {
		t.Fatalf("liveLines = %d, want 3 (empty output lines are real lines)", len(m.liveLines))
	}
	if m.liveOmitted != 0 {
		t.Fatalf("liveOmitted = %d, want 0", m.liveOmitted)
	}
	plain := stripANSI(m.contentRaw)
	if !strings.Contains(plain, `● bash({"command":"echo"})`) {
		t.Fatalf("content missing tool line:\n%s", plain)
	}
}

func TestLiveBlockDiscardedOnAbort(t *testing.T) {
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m.busy = true

	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventToolStart, Tool: "bash", Args: `{"command":"sleep 300"}`}})
	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventToolOutput, Tool: "bash", Text: "partial output"}})
	if len(m.liveLines) != 1 {
		t.Fatalf("liveLines = %d, want 1 before abort", len(m.liveLines))
	}

	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventTurnAborted, Model: "glm-5.3", Text: ""}})

	if len(m.liveLines) != 0 || m.liveOmitted != 0 {
		t.Fatalf("abort must discard the live block: lines=%v omitted=%d", m.liveLines, m.liveOmitted)
	}
	plain := stripANSI(m.vp.View())
	if strings.Contains(plain, "partial output") {
		t.Fatalf("aborted live block must not linger in content:\n%s", plain)
	}
	if !strings.Contains(plain, "⊘") {
		t.Fatalf("content missing abort marker:\n%s", plain)
	}
}

func TestShiftEnterCreatesNewline(t *testing.T) {
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	if m.input.Height() != defaultPromptHeight {
		t.Fatalf("initial prompt height = %d, want %d", m.input.Height(), defaultPromptHeight)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("first line")})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter, Alt: true})

	if got := m.input.Value(); got != "first line\n" {
		t.Fatalf("input = %q, want first line + newline", got)
	}
	if m.input.Height() != defaultPromptHeight {
		t.Fatalf("prompt height = %d, want %d after shift+enter", m.input.Height(), defaultPromptHeight)
	}
	if want := 30 - defaultPromptHeight - viewChromeRows; m.vp.Height != want {
		t.Fatalf("viewport height = %d, want %d after prompt growth", m.vp.Height, want)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyCtrlJ})
	if got := m.input.Value(); got != "first line\n\n" {
		t.Fatalf("input = %q, want second newline via ctrl+j", got)
	}
	if m.input.Height() != defaultPromptHeight {
		t.Fatalf("prompt height = %d, want %d after second newline", m.input.Height(), defaultPromptHeight)
	}
}

func TestEnterSendsMultiline(t *testing.T) {
	gw, bodies := captureGateway(t)
	m, ag := newAgentModel(t, gw.URL, false)

	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("line one")})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("line two")})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.busy {
		t.Fatal("expected busy=true after enter")
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
	content := ""
	for _, msg := range req.Messages {
		if msg.Role == "user" {
			content = msg.Content
		}
	}
	if content != "line one\nline two" {
		t.Fatalf("agent received %q, want line one\\nline two preserved", content)
	}
	if m.input.Value() != "" {
		t.Fatalf("input = %q, want cleared after send", m.input.Value())
	}
	if m.input.Height() != defaultPromptHeight {
		t.Fatalf("prompt height = %d, want %d after send", m.input.Height(), defaultPromptHeight)
	}
}

func TestPromptHeightClamped(t *testing.T) {
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	for i := 1; i <= 12; i++ {
		m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(fmt.Sprintf("line %02d", i))})
		m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	}

	if m.input.LineCount() != 13 {
		t.Fatalf("logical lines = %d, want 13", m.input.LineCount())
	}
	if m.input.Height() != maxPromptHeight {
		t.Fatalf("prompt height = %d, want %d (clamped)", m.input.Height(), maxPromptHeight)
	}
	if m.vp.Height < minViewportRows {
		t.Fatalf("viewport height = %d, want >= %d", m.vp.Height, minViewportRows)
	}

	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 12})
	if m.input.Height() != maxPromptHeight {
		t.Fatalf("prompt height after resize = %d, want %d", m.input.Height(), maxPromptHeight)
	}
	if m.vp.Height != minViewportRows {
		t.Fatalf("viewport height = %d, want %d (minimum enforced)", m.vp.Height, minViewportRows)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.input.Height() != defaultPromptHeight {
		t.Fatalf("prompt height after send = %d, want %d", m.input.Height(), defaultPromptHeight)
	}
	if want := 12 - defaultPromptHeight - viewChromeRows; m.vp.Height != want {
		t.Fatalf("viewport height after send = %d, want %d", m.vp.Height, want)
	}
}

func TestContentWrappedToViewportWidth(t *testing.T) {
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 40, Height: 30})

	m.appendLiveOutput(strings.Repeat("x", 100))
	m.refreshContent()
	assertLinesWithinWidth(t, m.contentPlain, 40, "live output")
	if len(m.contentPlain) < 3 {
		t.Fatalf("100-char live line must wrap into multiple lines, got %d", len(m.contentPlain))
	}

	m.blocks = append(m.blocks, errorBoxStyle.Render(strings.Repeat("erro ", 40)))
	m.refreshContent()
	assertLinesWithinWidth(t, m.contentPlain, 40, "error block")

	longURL := "https://example.com/" + strings.Repeat("a", 80)
	m.blocks = append(m.blocks, renderMarkdown("[link]("+longURL+")", m.mdWidth, m.dark))
	m.refreshContent()
	assertLinesWithinWidth(t, m.contentPlain, 40, "markdown long token")

	m.blocks = nil
	m.resetLiveBlock()
	m.blocks = append(m.blocks, resultStyle.Render("short line"))
	m.refreshContent()
	if len(m.contentPlain) != 1 || m.contentPlain[0] != "short line" {
		t.Fatalf("short content must not re-wrap, got %q", m.contentPlain)
	}
}

func assertLinesWithinWidth(t *testing.T, lines []string, width int, label string) {
	t.Helper()
	for i, line := range lines {
		if got := uniseg.StringWidth(line); got > width {
			t.Fatalf("%s line %d width = %d, want <= %d:\n%s", label, i, got, width, line)
		}
	}
}

func TestTruncateIsRuneSafe(t *testing.T) {
	if got := truncate("ação executada", 4); got != "açã…" {
		t.Fatalf("truncate = %q, want açã…", got)
	}
	if got := truncate("abc", 4); got != "abc" {
		t.Fatalf("short string must not truncate, got %q", got)
	}
	if got := truncate("abcdef", 0); got != "" {
		t.Fatalf("zero limit must produce empty string, got %q", got)
	}
}

func TestChatMarginsMatchOpencodePattern(t *testing.T) {
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 60, Height: 30})
	if want := 60 - 2*chatInset; m.vp.Width != want {
		t.Fatalf("viewport width = %d, want %d", m.vp.Width, want)
	}

	m.blocks = append(m.blocks, userBlock("pergunta do usuário", nil))
	m.blocks = append(m.blocks, indentLines(toolStyle.Render("● bash(ls)"), statusIndent))
	m.blocks = append(m.blocks, renderMarkdown("resposta do assistente", m.mdWidth, m.dark))
	m.refreshContent()

	view := stripANSI(m.View())
	for i, line := range strings.Split(view, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.HasPrefix(line, strings.Repeat(" ", chatInset)) {
			t.Fatalf("line %d must start with the chat inset:\n%s", i, line)
		}
		if got := uniseg.StringWidth(line); got > 60-chatInset {
			t.Fatalf("line %d width = %d, exceeds the right margin:\n%s", i, got, line)
		}
	}

	for _, want := range []string{
		"  ┃  pergunta do usuário",
		"     ● bash(ls)",
		"     resposta do assistente",
		"  ╹",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing aligned %q:\n%s", want, view)
		}
	}
	if !strings.HasPrefix(stripANSI(m.hintBar()), strings.Repeat(" ", chatInset)) {
		t.Fatalf("hint bar must start with the chat inset:\n%s", stripANSI(m.hintBar()))
	}
}

func TestHistoryNavigation(t *testing.T) {
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	send := func(prompt string) {
		t.Helper()
		m.input.SetValue(prompt)
		m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		m = step(t, m, agentEventMsg{event: <-m.agent.Events})
		if m.busy {
			t.Fatalf("expected busy=false after sending %q", prompt)
		}
	}
	for i := 1; i <= 3; i++ {
		send(fmt.Sprintf("prompt %02d", i))
	}
	if len(m.promptHistory) != 3 {
		t.Fatalf("history = %d entries, want 3", len(m.promptHistory))
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if got := m.input.Value(); got != "prompt 03" {
		t.Fatalf("up with empty input = %q, want prompt 03 (newest)", got)
	}
	m = step(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if got := m.input.Value(); got != "prompt 02" {
		t.Fatalf("second up = %q, want prompt 02", got)
	}
	m = step(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if got := m.input.Value(); got != "prompt 03" {
		t.Fatalf("down = %q, want prompt 03 back", got)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("!")})
	if m.histIdx != -1 {
		t.Fatalf("typing must reset history navigation, histIdx=%d", m.histIdx)
	}
	if got := m.input.Value(); got != "prompt 03!" {
		t.Fatalf("input after typing = %q, want prompt 03!", got)
	}
	m = step(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if got := m.input.Value(); got != "prompt 03!" {
		t.Fatalf("up after typing must move the cursor, not history, got %q", got)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyCtrlU})
	if got := m.input.Value(); got != "" {
		t.Fatalf("ctrl+u must clear the line, got %q", got)
	}
	m = step(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if got := m.input.Value(); got != "" {
		t.Fatalf("down without navigation = %q, want empty", got)
	}
	m = step(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if got := m.input.Value(); got != "prompt 03" {
		t.Fatalf("up with empty input = %q, want prompt 03", got)
	}
	m = step(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if got := m.input.Value(); got != "" {
		t.Fatalf("down past newest = %q, want empty input", got)
	}
	if m.histIdx != -1 {
		t.Fatalf("down past newest must end navigation, histIdx=%d", m.histIdx)
	}

	for i := 4; i <= 21; i++ {
		send(fmt.Sprintf("prompt %02d", i))
	}
	if len(m.promptHistory) != maxPromptHistory {
		t.Fatalf("history = %d entries, want %d (FIFO cap)", len(m.promptHistory), maxPromptHistory)
	}
	if m.promptHistory[0] != "prompt 02" {
		t.Fatalf("oldest = %q, want prompt 02 (prompt 01 discarded)", m.promptHistory[0])
	}
	if m.promptHistory[len(m.promptHistory)-1] != "prompt 21" {
		t.Fatalf("newest = %q, want prompt 21", m.promptHistory[len(m.promptHistory)-1])
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if got := m.input.Value(); got != "prompt 21" {
		t.Fatalf("up after cap = %q, want prompt 21", got)
	}
}

func TestHistorySkipsBusyRejected(t *testing.T) {
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m.input.SetValue("first prompt")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.busy {
		t.Fatal("expected busy=true after first send")
	}

	m.input.SetValue("rejected prompt")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.promptHistory) != 1 || m.promptHistory[0] != "first prompt" {
		t.Fatalf("history = %v, want only [first prompt] (busy-rejected send must not push)", m.promptHistory)
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func contentChunks(text string, promptTokens, completionTokens int64) []string {
	return []string{
		fmt.Sprintf(`{"choices":[{"delta":{"role":"assistant","content":%s}}]}`, mustJSON(text)),
		`{"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":` + fmt.Sprint(promptTokens) + `,"completion_tokens":` + fmt.Sprint(completionTokens) + `}}`,
	}
}

func toolCallChunks(id, name, args string) []string {
	return []string{
		fmt.Sprintf(`{"choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":%s,"function":{"name":%s,"arguments":%s}}]}}]}`, mustJSON(id), mustJSON(name), mustJSON(args)),
		`{"choices":[{"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`,
	}
}

func gatewayModelsHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		data := []map[string]string{}
		for _, m := range cat.Models {
			data = append(data, map[string]string{"id": m.Name})
		}
		json.NewEncoder(w).Encode(map[string]any{"data": data})
	}
}

func blockingGateway(t *testing.T, firstChunk string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", gatewayModelsHandler(t))
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		chunk := fmt.Sprintf(`{"choices":[{"delta":{"role":"assistant","content":%s}}]}`, mustJSON(firstChunk))
		fmt.Fprintf(w, "data: %s\n\n", chunk)
		flusher.Flush()
		<-r.Context().Done()
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func confirmGateway(t *testing.T) *httptest.Server {
	t.Helper()
	responses := [][]string{
		toolCallChunks("call_1", "bash", `{"command":"echo hi"}`),
		contentChunks("done", 20, 1),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", gatewayModelsHandler(t))
	var call int
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if call >= len(responses) {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		chunks := responses[call]
		call++
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for _, c := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", c)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func captureGateway(t *testing.T) (*httptest.Server, <-chan string) {
	t.Helper()
	bodies := make(chan string, 8)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", gatewayModelsHandler(t))
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		select {
		case bodies <- string(data):
		default:
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for _, c := range contentChunks("ok", 10, 1) {
			fmt.Fprintf(w, "data: %s\n\n", c)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, bodies
}

func newAgentModel(t *testing.T, gwURL string, confirm bool) (Model, *agent.Agent) {
	t.Helper()
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	ag := agent.New(llm.New(gwURL, "gw-key", false), nil, nil, cat, tools.NewRegistry(), nil, confirm)
	ag.SetPinned("glm-5.3")
	return New(ag, &config.Config{}, cat, true), ag
}

func newSessionAgentModel(t *testing.T, gwURL string) (Model, *agent.Agent, *session.Writer) {
	t.Helper()
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	sess, err := session.NewWriter()
	if err != nil {
		t.Fatalf("session writer: %v", err)
	}
	t.Cleanup(func() { sess.Close() })
	ag := agent.New(llm.New(gwURL, "gw-key", false), nil, nil, cat, tools.NewRegistry(), sess, false)
	ag.SetPinned("glm-5.3")
	return New(ag, &config.Config{}, cat, true), ag, sess
}

func transcriptUserContent(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var ev session.Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("unmarshal transcript line: %v", err)
		}
		if ev.Type == "user" {
			return ev.Content
		}
	}
	t.Fatal("transcript has no user event")
	return ""
}

func waitForAgentEvent(t *testing.T, ch chan agent.Event, kinds ...agent.EventKind) agent.Event {
	t.Helper()
	for {
		select {
		case e := <-ch:
			if e.Kind == agent.EventError {
				t.Fatalf("unexpected error event: %s", e.Text)
			}
			for _, k := range kinds {
				if e.Kind == k {
					return e
				}
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for event %v", kinds)
		}
	}
}

func TestEscCancelsWhenBusy(t *testing.T) {
	gw := blockingGateway(t, "partial answer")
	m, ag := newAgentModel(t, gw.URL, false)

	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m.input.SetValue("long running question")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.busy {
		t.Fatal("expected busy=true after enter")
	}

	delta := waitForAgentEvent(t, ag.Events, agent.EventDelta)
	m = step(t, m, agentEventMsg{event: delta})

	m = step(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	aborted := waitForAgentEvent(t, ag.Events, agent.EventTurnAborted)
	if aborted.Text != "partial answer" {
		t.Fatalf("aborted text = %q, want %q", aborted.Text, "partial answer")
	}
	if aborted.Model != "glm-5.3" {
		t.Fatalf("aborted model = %q, want glm-5.3", aborted.Model)
	}

	m = step(t, m, agentEventMsg{event: aborted})
	if m.busy {
		t.Fatal("expected busy=false after turn aborted")
	}
}

func TestEscNoOpWhenIdle(t *testing.T) {
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m.input.SetValue("draft text")
	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventRoute, Model: "glm-5.3", Router: "jev", Confidence: 0.9}})

	before := m
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	if m.state != before.state {
		t.Fatalf("state changed: %q -> %q", before.state, m.state)
	}
	if m.busy != before.busy {
		t.Fatalf("busy changed: %v -> %v", before.busy, m.busy)
	}
	if m.input.Value() != before.input.Value() {
		t.Fatalf("input value changed: %q -> %q", before.input.Value(), m.input.Value())
	}
	if len(m.blocks) != len(before.blocks) {
		t.Fatalf("blocks changed: %d -> %d", len(before.blocks), len(m.blocks))
	}
	if m.stream.String() != before.stream.String() {
		t.Fatalf("stream changed: %q -> %q", before.stream.String(), m.stream.String())
	}
	if m.vp.View() != before.vp.View() {
		t.Fatal("viewport content changed")
	}
}

func TestEscInConfirmDeclinesAndCancels(t *testing.T) {
	ch := make(chan bool, 1)
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventConfirm, Tool: "bash", Args: `{"command":"echo hi"}`, ApproveCh: ch}})
	if m.state != stateConfirm || m.pendingConfirm == nil {
		t.Fatalf("state=%q pendingConfirm=%v, want confirm state", m.state, m.pendingConfirm)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	select {
	case v := <-ch:
		if v != false {
			t.Fatalf("approve channel got %v, want false", v)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("approve channel not answered after esc")
	}
	if m.state != stateChat {
		t.Fatalf("state = %q, want chat", m.state)
	}
	if m.pendingConfirm != nil {
		t.Fatal("pendingConfirm not cleared after esc")
	}

	gw := confirmGateway(t)
	m2, ag := newAgentModel(t, gw.URL, true)
	m2 = step(t, m2, tea.WindowSizeMsg{Width: 100, Height: 30})
	m2.input.SetValue("run echo hi")
	m2 = step(t, m2, tea.KeyMsg{Type: tea.KeyEnter})
	if !m2.busy {
		t.Fatal("expected busy=true after enter")
	}
	confirm := waitForAgentEvent(t, ag.Events, agent.EventConfirm)
	m2 = step(t, m2, agentEventMsg{event: confirm})
	if m2.state != stateConfirm {
		t.Fatalf("state = %q, want confirm", m2.state)
	}

	m2 = step(t, m2, tea.KeyMsg{Type: tea.KeyEsc})

	var aborted agent.Event
	for {
		e := waitForAgentEvent(t, ag.Events, agent.EventTurnAborted, agent.EventToolResult, agent.EventTurnDone)
		if e.Kind == agent.EventTurnAborted {
			aborted = e
			break
		}
		t.Fatalf("esc in confirm should abort the turn, got %s first", e.Kind)
	}
	if aborted.Model != "glm-5.3" {
		t.Fatalf("aborted model = %q, want glm-5.3", aborted.Model)
	}

	m2 = step(t, m2, agentEventMsg{event: aborted})
	if m2.busy {
		t.Fatal("expected busy=false after abort")
	}
	if m2.state != stateChat {
		t.Fatalf("state = %q, want chat", m2.state)
	}

	gw2 := confirmGateway(t)
	m3, ag2 := newAgentModel(t, gw2.URL, true)
	m3 = step(t, m3, tea.WindowSizeMsg{Width: 100, Height: 30})
	m3.input.SetValue("run echo hi")
	m3 = step(t, m3, tea.KeyMsg{Type: tea.KeyEnter})
	confirm2 := waitForAgentEvent(t, ag2.Events, agent.EventConfirm)
	m3 = step(t, m3, agentEventMsg{event: confirm2})

	m3 = step(t, m3, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

	res := waitForAgentEvent(t, ag2.Events, agent.EventToolResult)
	if !strings.Contains(res.Result, "declined") {
		t.Fatalf("tool result = %q, want declined", res.Result)
	}
	done := waitForAgentEvent(t, ag2.Events, agent.EventTurnDone)
	m3 = step(t, m3, agentEventMsg{event: done})
	if m3.busy {
		t.Fatal("expected busy=false after turn done")
	}
}

func TestHintBarShowsEscToInterruptWhenBusy(t *testing.T) {
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	if strings.Contains(m.View(), "esc to interrupt") {
		t.Fatal("idle hint bar should not show esc to interrupt")
	}

	m.busy = true
	if !strings.Contains(m.View(), "esc to interrupt") {
		t.Fatalf("busy hint bar missing esc to interrupt:\n%s", m.View())
	}

	m.busy = false
	if strings.Contains(m.View(), "esc to interrupt") {
		t.Fatal("idle hint bar should not show esc to interrupt")
	}
}

func TestTurnAbortedRendersMarker(t *testing.T) {
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m.busy = true

	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventDelta, Model: "glm-5.3", Text: "partial "}})
	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventDelta, Model: "glm-5.3", Text: "answer"}})
	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventTurnAborted, Model: "glm-5.3", Text: "partial answer"}})

	content := m.vp.View()
	plain := stripANSI(content)
	if !strings.Contains(plain, "partial answer") {
		t.Fatalf("rendered content missing partial text:\n%s", content)
	}
	if !strings.Contains(plain, "interrompido") {
		t.Fatalf("rendered content missing abort marker:\n%s", content)
	}
	if !strings.Contains(plain, "⊘") {
		t.Fatalf("rendered content missing abort mark:\n%s", content)
	}
	if !strings.Contains(strings.Join(m.contentPlain, "\n"), "interrompido") {
		t.Fatalf("copiable plain content missing abort marker:\n%s", strings.Join(m.contentPlain, "\n"))
	}
	if m.stream.Len() != 0 {
		t.Fatalf("stream not reset after abort, len=%d", m.stream.Len())
	}
	if m.busy {
		t.Fatal("busy should be false after turn aborted")
	}
}

func forceTrueColor(t *testing.T) {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
}

func countDiffLines(plain string) int {
	n := 0
	for _, l := range strings.Split(plain, "\n") {
		l = strings.TrimLeft(l, " ")
		if strings.HasPrefix(l, "+") || strings.HasPrefix(l, "-") {
			n++
		}
	}
	return n
}

func TestDiffBlockRendersColors(t *testing.T) {
	forceTrueColor(t)
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	diff := []tools.DiffLine{
		{Kind: ' ', Text: "context line"},
		{Kind: '-', Text: "removed line"},
		{Kind: '+', Text: "added line"},
	}

	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventToolResult, Tool: "edit", Result: "edit done", Diff: diff}})

	content := m.vp.View()
	if !strings.Contains(content, "\x1b[38;2;79;214;190m+added line") {
		t.Fatalf("diff block missing colDiffAdded ANSI:\n%q", content)
	}
	if !strings.Contains(content, "\x1b[38;2;197;59;83m-removed line") {
		t.Fatalf("diff block missing colDiffRemoved ANSI:\n%q", content)
	}
	if !strings.Contains(content, "\x1b[38;2;128;128;128m context line") {
		t.Fatalf("diff block missing colTextMuted context ANSI:\n%q", content)
	}
	plain := stripANSI(content)
	resultIdx := strings.Index(plain, "edit done")
	addedIdx := strings.Index(plain, "+added line")
	if resultIdx == -1 || addedIdx == -1 {
		t.Fatalf("content missing result or diff line:\n%s", plain)
	}
	if resultIdx > addedIdx {
		t.Fatalf("diff block must render after tool result line:\n%s", plain)
	}
}

func TestDiffBlockTruncatesMiddle(t *testing.T) {
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 120})
	diff := make([]tools.DiffLine, 100)
	for i := range diff {
		diff[i] = tools.DiffLine{Kind: '+', Text: fmt.Sprintf("line %d", i)}
	}

	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventToolResult, Tool: "write", Result: "wrote file", Diff: diff}})

	plain := stripANSI(m.vp.View())
	if !strings.Contains(plain, "… 60 more lines …") {
		t.Fatalf("truncated diff missing indicator:\n%s", plain)
	}
	if n := countDiffLines(plain); n != 40 {
		t.Fatalf("visible diff lines = %d, want 40:\n%s", n, plain)
	}
	for _, want := range []string{"+line 0", "+line 19", "+line 80", "+line 99"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("truncated diff missing %q:\n%s", want, plain)
		}
	}
	for _, hidden := range []string{"+line 20", "+line 50", "+line 79"} {
		if strings.Contains(plain, hidden) {
			t.Fatalf("truncated diff should hide %q:\n%s", hidden, plain)
		}
	}
}

func TestDiffBlockKeepsShortDiffs(t *testing.T) {
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 120})
	diff := make([]tools.DiffLine, 40)
	for i := range diff {
		diff[i] = tools.DiffLine{Kind: '+', Text: fmt.Sprintf("line %d", i)}
	}

	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventToolResult, Tool: "write", Result: "wrote file", Diff: diff}})

	plain := stripANSI(m.vp.View())
	if strings.Contains(plain, "more lines") {
		t.Fatalf("40-line diff should not truncate:\n%s", plain)
	}
	if n := countDiffLines(plain); n != 40 {
		t.Fatalf("visible diff lines = %d, want 40:\n%s", n, plain)
	}
	if !strings.Contains(plain, "+line 0") || !strings.Contains(plain, "+line 39") {
		t.Fatalf("diff missing edge lines:\n%s", plain)
	}
}

func TestEmptyDiffRendersNoBlock(t *testing.T) {
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventToolResult, Tool: "bash", Result: "hi"}})
	if len(m.blocks) != 1 {
		t.Fatalf("blocks = %d, want 1 (result line only)", len(m.blocks))
	}

	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventToolResult, Tool: "edit", Result: "no changes", Diff: []tools.DiffLine{}}})
	if len(m.blocks) != 2 {
		t.Fatalf("blocks = %d, want 2 (empty diff adds no block)", len(m.blocks))
	}
	if strings.Contains(stripANSI(m.vp.View()), "\n+") {
		t.Fatalf("empty diff should not render diff lines:\n%s", stripANSI(m.vp.View()))
	}
}

func TestConfirmViewShowsDiff(t *testing.T) {
	forceTrueColor(t)
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	diff := []tools.DiffLine{
		{Kind: '-', Text: "old line"},
		{Kind: '+', Text: "new line"},
	}

	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventConfirm, Tool: "edit", Args: `{"path":"a.txt"}`, Diff: diff, ApproveCh: make(chan bool, 1)}})
	if m.state != stateConfirm {
		t.Fatalf("state = %q, want confirm", m.state)
	}

	view := m.View()
	plain := stripANSI(view)
	addedIdx := strings.Index(plain, "+new line")
	removedIdx := strings.Index(plain, "-old line")
	hintIdx := strings.Index(plain, "y approve")
	if addedIdx == -1 || removedIdx == -1 {
		t.Fatalf("confirm view missing diff lines:\n%s", plain)
	}
	if hintIdx == -1 {
		t.Fatalf("confirm view missing y/n hint:\n%s", plain)
	}
	if addedIdx > hintIdx || removedIdx > hintIdx {
		t.Fatalf("diff must render before y/n hint:\n%s", plain)
	}
	if !strings.Contains(view, "\x1b[38;2;79;214;190m") || !strings.Contains(view, "\x1b[38;2;197;59;83m") {
		t.Fatalf("confirm view missing diff colors:\n%q", view)
	}
}

func TestCompactionLineRendersMuted(t *testing.T) {
	forceTrueColor(t)
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m.busy = true

	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventDelta, Model: "glm-5.3", Text: "partial "}})
	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventCompaction, TokensBefore: 12400, TokensAfter: 3100}})

	content := m.vp.View()
	if !strings.Contains(content, "\x1b[38;2;128;128;128m⚡ context compacted (12.4k → 3.1k tokens)") {
		t.Fatalf("compaction line missing colTextMuted ANSI:\n%q", content)
	}
	plain := stripANSI(content)
	if !strings.Contains(plain, "⚡ context compacted (12.4k → 3.1k tokens)") {
		t.Fatalf("rendered content missing compaction line:\n%s", plain)
	}
	if !strings.Contains(strings.Join(m.contentPlain, "\n"), "⚡ context compacted (12.4k → 3.1k tokens)") {
		t.Fatalf("copiable plain content missing compaction line:\n%s", strings.Join(m.contentPlain, "\n"))
	}
	if m.stream.String() != "partial " {
		t.Fatalf("compaction must not reset the stream, got %q", m.stream.String())
	}
	if m.state != stateChat {
		t.Fatalf("compaction must not change state, got %q", m.state)
	}
	if !m.busy {
		t.Fatal("compaction must not change busy")
	}
	if m.pendingConfirm != nil {
		t.Fatal("compaction must not trigger confirmation")
	}
}

func TestWithResumedStoresMessages(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Content: "previous question"},
		{Role: "assistant", Content: "previous answer"},
	}
	m := newTestModel(t, WithResumed(msgs))
	if len(m.resumed) != len(msgs) {
		t.Fatalf("resumed = %d messages, want %d", len(m.resumed), len(msgs))
	}
	if m.resumed[0].Content != "previous question" || m.resumed[1].Content != "previous answer" {
		t.Fatalf("resumed messages mismatch: %+v", m.resumed)
	}
}

func TestWithFreshWarningRendersMuted(t *testing.T) {
	forceTrueColor(t)
	m := newTestModel(t, WithFreshWarning())
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	content := m.vp.View()
	if !strings.Contains(content, "\x1b[38;2;128;128;128mno previous session — starting fresh") {
		t.Fatalf("fresh warning missing colTextMuted ANSI:\n%q", content)
	}
	plain := stripANSI(content)
	if !strings.Contains(plain, "no previous session — starting fresh") {
		t.Fatalf("rendered content missing fresh warning:\n%s", plain)
	}
	if !strings.Contains(strings.Join(m.contentPlain, "\n"), "no previous session — starting fresh") {
		t.Fatalf("copiable plain content missing fresh warning:\n%s", strings.Join(m.contentPlain, "\n"))
	}

	fresh := newTestModel(t)
	fresh = step(t, fresh, tea.WindowSizeMsg{Width: 100, Height: 30})
	if strings.Contains(stripANSI(fresh.vp.View()), "no previous session") {
		t.Fatal("fresh warning should not render without the option")
	}
}

func resumedFixture(n int) []llm.Message {
	msgs := make([]llm.Message, 0, n)
	for i := 0; i < n; i++ {
		switch i % 3 {
		case 0:
			msgs = append(msgs, llm.Message{Role: "user", Content: fmt.Sprintf("question %02d", i)})
		case 1:
			msgs = append(msgs, llm.Message{Role: "assistant", Content: fmt.Sprintf("answer %02d", i)})
		default:
			msgs = append(msgs, llm.Message{Role: "tool", ToolCallID: fmt.Sprintf("call_%02d", i), Content: fmt.Sprintf("result %02d", i)})
		}
	}
	return msgs
}

func TestResumedReconstructsBlocks(t *testing.T) {
	m := newTestModel(t, WithResumed(resumedFixture(25)))
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	plain := stripANSI(m.contentRaw)
	for _, want := range []string{"question 06", "answer 07", "result 08", "question 24", "▣"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("resumed content missing %q:\n%s", want, plain)
		}
	}
	for _, hidden := range []string{"question 00", "answer 01", "result 02", "question 03", "answer 04"} {
		if strings.Contains(plain, hidden) {
			t.Fatalf("resumed content must omit messages older than the last 20, found %q:\n%s", hidden, plain)
		}
	}
	if len(m.blocks) != 26 {
		t.Fatalf("blocks = %d, want 26 (7 user boxes, 6 markdown, 6 marks, 7 tool lines)", len(m.blocks))
	}
	if m.resumedCount != 25 {
		t.Fatalf("resumedCount = %d, want 25 (snapshot total)", m.resumedCount)
	}
	if m.resumed != nil {
		t.Fatal("resumed messages must be consumed after reconstruction")
	}
	visible := stripANSI(m.vp.View())
	if !strings.Contains(visible, "question 24") {
		t.Fatalf("viewport must show the latest resumed block at start:\n%s", visible)
	}

	m = step(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	if len(m.blocks) != 26 {
		t.Fatalf("resize must not rebuild resumed blocks, blocks = %d, want 26", len(m.blocks))
	}
}

func TestHintBarShowsResumed(t *testing.T) {
	m := newTestModel(t, WithResumed(resumedFixture(3)))
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	view := stripANSI(m.View())
	if !strings.Contains(view, "resumed · 3 mensagens") {
		t.Fatalf("hint bar missing resumed indicator:\n%s", view)
	}

	m = step(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	view = stripANSI(m.View())
	if !strings.Contains(view, "resumed · 3 mensagens") {
		t.Fatalf("hint bar must keep the resumed indicator after resize:\n%s", view)
	}

	fresh := newTestModel(t)
	fresh = step(t, fresh, tea.WindowSizeMsg{Width: 100, Height: 30})
	if strings.Contains(stripANSI(fresh.View()), "resumed") {
		t.Fatal("hint bar must not show resumed without loaded messages")
	}
}

func TestResumedSkipsSystemMessages(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Content: "question 00"},
		{Role: "assistant", Content: "answer 01"},
		{Role: "system", Content: "compaction summary of earlier conversation"},
	}
	m := newTestModel(t, WithResumed(msgs))
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	plain := stripANSI(m.contentRaw)
	if strings.Contains(plain, "compaction summary") {
		t.Fatalf("system message must not render a block:\n%s", plain)
	}
	if !strings.Contains(plain, "question 00") || !strings.Contains(plain, "answer 01") {
		t.Fatalf("resumed content missing user/assistant blocks:\n%s", plain)
	}
	if len(m.blocks) != 3 {
		t.Fatalf("blocks = %d, want 3 (user box, markdown, mark)", len(m.blocks))
	}
}

func TestResumedRendersAssistantToolCalls(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Content: "list files"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call_1", Type: "function", Function: llm.FuncCall{Name: "bash", Arguments: `{"command":"ls"}`}}}},
		{Role: "tool", Content: "file1\nfile2", ToolCallID: "call_1"},
		{Role: "assistant", Content: "here are your files"},
	}
	m := newTestModel(t, WithResumed(msgs))
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	plain := stripANSI(m.contentRaw)
	if !strings.Contains(plain, `● bash({"command":"ls"})`) {
		t.Fatalf("resumed content missing tool call line:\n%s", plain)
	}
	if !strings.Contains(plain, "file1 ⏎ file2") {
		t.Fatalf("resumed content missing tool result line:\n%s", plain)
	}
	if n := strings.Count(plain, "▣"); n != 1 {
		t.Fatalf("tool-call assistant must not render a turn mark, ▣ count = %d:\n%s", n, plain)
	}
	if !strings.Contains(plain, "here are your files") {
		t.Fatalf("resumed content missing final answer:\n%s", plain)
	}
}

func TestFormatTokensK(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0"},
		{12, "12"},
		{999, "999"},
		{1000, "1.0k"},
		{3100, "3.1k"},
		{12400, "12.4k"},
		{15500, "15.5k"},
		{200000, "200.0k"},
	}
	for _, c := range cases {
		if got := formatTokensK(c.in); got != c.want {
			t.Fatalf("formatTokensK(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSendExpandsButChatShowsOriginal(t *testing.T) {
	dir := chdirTemp(t)
	writeFile(t, dir, "a.txt", "conteúdo do arquivo")

	gw, bodies := captureGateway(t)
	m, ag, sess := newSessionAgentModel(t, gw.URL)

	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m.input.SetValue("@a.txt o que é?")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.busy {
		t.Fatal("expected busy=true after enter")
	}

	done := waitForAgentEvent(t, ag.Events, agent.EventTurnDone)
	m = step(t, m, agentEventMsg{event: done})
	if m.busy {
		t.Fatal("expected busy=false after turn done")
	}

	plain := stripANSI(m.contentRaw)
	if !strings.Contains(plain, "@a.txt o que é?") {
		t.Fatalf("chat missing original text:\n%s", plain)
	}
	if strings.Contains(plain, "--- Arquivo @a.txt ---") {
		t.Fatalf("chat must not show the expanded block:\n%s", plain)
	}
	if strings.Contains(plain, "conteúdo do arquivo") {
		t.Fatalf("chat must not show the file content:\n%s", plain)
	}

	want := "@a.txt o que é?\n\n--- Arquivo @a.txt ---\nconteúdo do arquivo\n"
	if got := transcriptUserContent(t, sess.Path()); got != want {
		t.Fatalf("transcript user event = %q, want %q", got, want)
	}

	var body string
	select {
	case body = <-bodies:
	default:
		t.Fatal("gateway captured no request body")
	}
	if !strings.Contains(body, "--- Arquivo @a.txt ---") || !strings.Contains(body, "conteúdo do arquivo") {
		t.Fatalf("agent request missing the expanded content:\n%s", body)
	}
}

func TestWarningRendersInline(t *testing.T) {
	forceTrueColor(t)
	dir := chdirTemp(t)
	for _, name := range []string{"f1.txt", "f2.txt", "f3.txt", "f4.txt", "f5.txt", "f6.txt"} {
		writeFile(t, dir, name, "conteúdo "+name)
	}

	gw, _ := captureGateway(t)
	m, ag := newAgentModel(t, gw.URL, false)

	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m.input.SetValue("@f1.txt @f2.txt @f3.txt @f4.txt @f5.txt @f6.txt")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.busy {
		t.Fatal("warning must not block the send, busy=false after enter")
	}

	done := waitForAgentEvent(t, ag.Events, agent.EventTurnDone)
	m = step(t, m, agentEventMsg{event: done})

	view := m.View()
	if !strings.Contains(view, "\x1b[38;2;245;167;65mmax 5 file mentions") {
		t.Fatalf("warning missing colWarning ANSI:\n%q", view)
	}
	plain := stripANSI(view)
	chatIdx := strings.Index(plain, "@f1.txt")
	warnIdx := strings.Index(plain, "max 5 file mentions")
	fadeIdx := strings.Index(plain, "╹")
	if chatIdx < 0 || warnIdx < 0 || fadeIdx < 0 {
		t.Fatalf("view missing chat/warning/fade markers:\n%s", plain)
	}
	if !(chatIdx < warnIdx && warnIdx < fadeIdx) {
		t.Fatalf("warning must render between chat and prompt (chat=%d warn=%d fade=%d):\n%s", chatIdx, warnIdx, fadeIdx, plain)
	}

	m.input.SetValue("plain question")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	done = waitForAgentEvent(t, ag.Events, agent.EventTurnDone)
	m = step(t, m, agentEventMsg{event: done})
	if strings.Contains(stripANSI(m.View()), "max 5 file mentions") {
		t.Fatal("warning should clear on the next send without mentions")
	}
}

func TestPopupListsFilesOnAtSign(t *testing.T) {
	forceTrueColor(t)
	dir := chdirTemp(t)
	for i := 1; i <= 12; i++ {
		writeFile(t, dir, fmt.Sprintf("f%02d.txt", i), "x")
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("@")})

	if !m.mentionOpen {
		t.Fatal("expected popup open after typing @")
	}
	want := []string{"f01.txt", "f02.txt", "f03.txt", "f04.txt", "f05.txt", "f06.txt", "f07.txt", "f08.txt", "f09.txt", "f10.txt"}
	if len(m.mentionItems) != maxMentionSuggestions {
		t.Fatalf("items = %d, want %d (max suggestions)", len(m.mentionItems), maxMentionSuggestions)
	}
	if strings.Join(m.mentionItems, "|") != strings.Join(want, "|") {
		t.Fatalf("items = %v, want sorted %v", m.mentionItems, want)
	}
	if m.mentionSelected != 0 {
		t.Fatalf("selected = %d, want 0 on open", m.mentionSelected)
	}
	for _, item := range m.mentionItems {
		if item == "subdir" {
			t.Fatal("popup must list files only, got directory subdir")
		}
	}

	view := m.View()
	if !strings.Contains(view, "\x1b[38;2;250;178;131m▸ f01.txt") {
		t.Fatalf("selected item missing colPrimary ANSI:\n%q", view)
	}
	if !strings.Contains(view, "\x1b[38;2;128;128;128m  f02.txt") {
		t.Fatalf("popup items missing colTextMuted ANSI:\n%q", view)
	}
	plain := stripANSI(view)
	chatIdx := strings.Index(plain, "No LLM gateway configured.")
	popupIdx := strings.Index(plain, "f01.txt")
	promptIdx := strings.LastIndex(plain, "@")
	if chatIdx < 0 || popupIdx < 0 || promptIdx < 0 {
		t.Fatalf("view missing chat/popup/prompt markers:\n%s", plain)
	}
	if !(chatIdx < popupIdx && popupIdx < promptIdx) {
		t.Fatalf("popup must render between viewport and prompt (chat=%d popup=%d prompt=%d):\n%s", chatIdx, popupIdx, promptIdx, plain)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.mentionSelected != 1 {
		t.Fatalf("down must move selection, got %d", m.mentionSelected)
	}
	m = step(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if m.mentionSelected != 0 {
		t.Fatalf("up must move selection, got %d", m.mentionSelected)
	}
	m = step(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if m.mentionSelected != len(m.mentionItems)-1 {
		t.Fatalf("up must wrap to last item, got %d", m.mentionSelected)
	}
	m = step(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.mentionSelected != 0 {
		t.Fatalf("down must wrap to first item, got %d", m.mentionSelected)
	}
	if got := len(textBeforeCursor(m.input)); got != 1 {
		t.Fatalf("popup navigation must not move the input cursor, pos=%d", got)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f1")})
	if !m.mentionOpen {
		t.Fatal("typing must re-glob and keep the popup open")
	}
	if strings.Join(m.mentionItems, "|") != "f10.txt|f11.txt|f12.txt" {
		t.Fatalf("items after typing 1 = %v, want [f10.txt f11.txt f12.txt]", m.mentionItems)
	}
	if m.mentionSelected != 0 {
		t.Fatalf("selection must reset on re-glob, got %d", m.mentionSelected)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	if m.mentionOpen {
		t.Fatal("prefix without matches must close the popup")
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
	if m.mentionOpen {
		t.Fatal("invalid glob pattern must close the popup")
	}
}

func TestTabCompletesFirstResult(t *testing.T) {
	dir := chdirTemp(t)
	writeFile(t, dir, "a1.txt", "x")
	writeFile(t, dir, "a2.txt", "x")
	writeFile(t, dir, "b.txt", "x")

	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("@a")})
	if !m.mentionOpen {
		t.Fatal("expected popup open after typing @a")
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.mentionOpen {
		t.Fatal("popup must close after tab")
	}
	if len(m.mentionItems) != 0 {
		t.Fatalf("items must clear after tab, got %v", m.mentionItems)
	}
	if got := m.input.Value(); got != "@a1.txt" {
		t.Fatalf("input = %q, want @a1.txt (first result)", got)
	}
	if got := len(textBeforeCursor(m.input)); got != len("@a1.txt") {
		t.Fatalf("cursor = %d, want %d after completion", got, len("@a1.txt"))
	}

	m2 := newTestModel(t)
	m2 = step(t, m2, tea.WindowSizeMsg{Width: 100, Height: 30})
	m2 = step(t, m2, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("olha @b fim")})
	if m2.mentionOpen {
		t.Fatal("space after the token must close the popup")
	}
	for i := 0; i < 4; i++ {
		m2 = step(t, m2, tea.KeyMsg{Type: tea.KeyLeft})
	}
	if !m2.mentionOpen {
		t.Fatal("cursor over @b must reopen the popup")
	}
	m2 = step(t, m2, tea.KeyMsg{Type: tea.KeyTab})
	if got := m2.input.Value(); got != "olha @b.txt fim" {
		t.Fatalf("input = %q, want olha @b.txt fim (token under cursor replaced)", got)
	}
	if got := len(textBeforeCursor(m2.input)); got != len("olha @b.txt") {
		t.Fatalf("cursor = %d, want %d after mid-text completion", got, len("olha @b.txt"))
	}

	m3 := newTestModel(t)
	m3 = step(t, m3, tea.WindowSizeMsg{Width: 100, Height: 30})
	m3 = step(t, m3, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("@a")})
	m3 = step(t, m3, tea.KeyMsg{Type: tea.KeyDown})
	m3 = step(t, m3, tea.KeyMsg{Type: tea.KeyTab})
	if got := m3.input.Value(); got != "@a2.txt" {
		t.Fatalf("input = %q, want @a2.txt (selected item)", got)
	}
}

func TestMentionCompletionMultiline(t *testing.T) {
	dir := chdirTemp(t)
	writeFile(t, dir, "b1.txt", "x")
	writeFile(t, dir, "b2.txt", "x")

	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("olha @b")})
	if !m.mentionOpen {
		t.Fatal("expected popup open after typing @b")
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	if m.mentionOpen {
		t.Fatal("newline must close the popup (token stays on the previous line)")
	}
	if got := m.input.Value(); got != "olha @b\n" {
		t.Fatalf("input = %q, want olha @b + newline", got)
	}
	if m.input.Height() != defaultPromptHeight {
		t.Fatalf("prompt height = %d, want %d", m.input.Height(), defaultPromptHeight)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("fim")})
	for i := 0; i < len("fim")+1; i++ {
		m = step(t, m, tea.KeyMsg{Type: tea.KeyLeft})
	}
	if !m.mentionOpen {
		t.Fatal("cursor back over @b must reopen the popup across lines")
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.mentionOpen {
		t.Fatal("popup must close after tab")
	}
	if got := m.input.Value(); got != "olha @b1.txt\nfim" {
		t.Fatalf("input = %q, want olha @b1.txt\\nfim", got)
	}
	if got := len(textBeforeCursor(m.input)); got != len("olha @b1.txt") {
		t.Fatalf("cursor = %d, want %d after mid-text completion on the first line", got, len("olha @b1.txt"))
	}
	if m.input.Line() != 0 {
		t.Fatalf("cursor row = %d, want 0 after completion", m.input.Line())
	}
	if m.input.Height() != defaultPromptHeight {
		t.Fatalf("prompt height = %d, want %d after completion", m.input.Height(), defaultPromptHeight)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if got := m.input.Value(); got != "olha @b1.txt\nfim" {
		t.Fatalf("down with non-empty input must move the cursor, not history, got %q", got)
	}
	if m.input.Line() != 1 {
		t.Fatalf("cursor row = %d, want 1 after down", m.input.Line())
	}
}

func TestEnterWithPopupCompletesNotSends(t *testing.T) {
	dir := chdirTemp(t)
	writeFile(t, dir, "a.txt", "conteúdo do arquivo")

	gw, bodies := captureGateway(t)
	m, ag := newAgentModel(t, gw.URL, false)

	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("@a")})
	if !m.mentionOpen {
		t.Fatal("expected popup open after typing @a")
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.busy {
		t.Fatal("enter with popup open must not send")
	}
	if m.mentionOpen {
		t.Fatal("popup must close after enter completion")
	}
	if got := m.input.Value(); got != "@a.txt" {
		t.Fatalf("input = %q, want @a.txt completed", got)
	}
	select {
	case body := <-bodies:
		t.Fatalf("enter with popup open must not call agent.Run, got body: %s", body)
	default:
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.busy {
		t.Fatal("enter with popup closed must send")
	}
	done := waitForAgentEvent(t, ag.Events, agent.EventTurnDone)
	m = step(t, m, agentEventMsg{event: done})
	if m.busy {
		t.Fatal("expected busy=false after turn done")
	}

	var body string
	select {
	case body = <-bodies:
	default:
		t.Fatal("gateway captured no request body after send")
	}
	if !strings.Contains(body, "--- Arquivo @a.txt ---") {
		t.Fatalf("sent message missing expanded mention: %s", body)
	}
}

func TestEscClosesPopup(t *testing.T) {
	dir := chdirTemp(t)
	writeFile(t, dir, "a.txt", "x")

	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventRoute, Model: "glm-5.3", Router: "jev", Confidence: 0.9}})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("@")})
	if !m.mentionOpen {
		t.Fatal("expected popup open after typing @")
	}

	before := m
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	if m.mentionOpen {
		t.Fatal("esc must close the popup")
	}
	if len(m.mentionItems) != 0 {
		t.Fatalf("items must clear after esc, got %v", m.mentionItems)
	}
	if m.mentionSelected != 0 {
		t.Fatalf("selected must reset after esc, got %d", m.mentionSelected)
	}
	if m.busy != before.busy {
		t.Fatalf("esc with popup open must not cancel the turn, busy %v -> %v", before.busy, m.busy)
	}
	if m.state != before.state {
		t.Fatalf("state changed: %q -> %q", before.state, m.state)
	}
	if m.input.Value() != before.input.Value() {
		t.Fatalf("input changed: %q -> %q", before.input.Value(), m.input.Value())
	}
	if len(m.blocks) != len(before.blocks) {
		t.Fatalf("blocks changed: %d -> %d", len(before.blocks), len(m.blocks))
	}
	if m.vp.View() != before.vp.View() {
		t.Fatal("viewport content changed after esc")
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if !m.mentionOpen {
		t.Fatal("typing after esc must re-glob and reopen the popup")
	}
	if strings.Join(m.mentionItems, "|") != "a.txt" {
		t.Fatalf("items after reopen = %v, want [a.txt]", m.mentionItems)
	}

	gw, _ := captureGateway(t)
	mb, agb := newAgentModel(t, gw.URL, false)
	mb = step(t, mb, tea.WindowSizeMsg{Width: 100, Height: 30})
	mb.input.SetValue("long running question")
	mb = step(t, mb, tea.KeyMsg{Type: tea.KeyEnter})
	if !mb.busy {
		t.Fatal("expected busy=true after enter")
	}

	mb = step(t, mb, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("@")})
	if !mb.mentionOpen {
		t.Fatal("popup must open while a turn is running")
	}
	mb = step(t, mb, tea.KeyMsg{Type: tea.KeyEsc})
	if mb.mentionOpen {
		t.Fatal("esc must close the popup while busy")
	}
	if !mb.busy {
		t.Fatal("esc with popup open must not cancel the running turn")
	}

	done := waitForAgentEvent(t, agb.Events, agent.EventTurnDone)
	mb = step(t, mb, agentEventMsg{event: done})
	if mb.busy {
		t.Fatal("expected busy=false after turn done")
	}
}

func TestImageCommandAttaches(t *testing.T) {
	forceTrueColor(t)
	dir := chdirTemp(t)
	writeFile(t, dir, "shot.png", "pngdata")
	writeFile(t, dir, "Camel.PNG", "upper")

	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m.input.SetValue("/image " + filepath.Join(dir, "shot.png"))
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if len(m.pendingAttachments) != 1 {
		t.Fatalf("pendingAttachments = %d, want 1", len(m.pendingAttachments))
	}
	att := m.pendingAttachments[0]
	if att.name != "shot.png" {
		t.Fatalf("attachment name = %q, want shot.png", att.name)
	}
	if att.size != int64(len("pngdata")) {
		t.Fatalf("attachment size = %d, want %d", att.size, len("pngdata"))
	}
	wantURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("pngdata"))
	if att.dataURI != wantURI {
		t.Fatalf("attachment dataURI = %q, want %q", att.dataURI, wantURI)
	}

	view := m.View()
	if !strings.Contains(view, "\x1b[38;2;92;156;245m🖼 shot.png ×") {
		t.Fatalf("chip missing colSecondary ANSI:\n%q", view)
	}
	plain := stripANSI(view)
	chatIdx := strings.Index(plain, "No LLM gateway configured.")
	chipIdx := strings.Index(plain, "🖼 shot.png ×")
	fadeIdx := strings.Index(plain, "╹")
	if chatIdx < 0 || chipIdx < 0 || fadeIdx < 0 {
		t.Fatalf("view missing chat/chip/fade markers:\n%s", plain)
	}
	if !(chatIdx < chipIdx && chipIdx < fadeIdx) {
		t.Fatalf("chip must render between chat and prompt (chat=%d chip=%d fade=%d):\n%s", chatIdx, chipIdx, fadeIdx, plain)
	}

	m.input.SetValue("/image " + filepath.Join(dir, "Camel.PNG"))
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.pendingAttachments) != 2 {
		t.Fatalf("pendingAttachments after uppercase attach = %d, want 2", len(m.pendingAttachments))
	}
	if !strings.HasPrefix(m.pendingAttachments[1].dataURI, "data:image/png;base64,") {
		t.Fatalf("uppercase .PNG must map to image/png, got %q", m.pendingAttachments[1].dataURI)
	}
	if !strings.Contains(stripANSI(m.View()), "🖼 Camel.PNG ×") {
		t.Fatalf("view missing second chip:\n%s", stripANSI(m.View()))
	}
	if want := 30 - defaultPromptHeight - viewChromeRows - 1; m.vp.Height != want {
		t.Fatalf("viewport height with chips = %d, want %d (one extra chrome row)", m.vp.Height, want)
	}
}

func TestImageRejectsInvalid(t *testing.T) {
	forceTrueColor(t)
	dir := chdirTemp(t)
	missing := filepath.Join(dir, "missing.png")
	txt := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(txt, []byte("just text"), 0o644); err != nil {
		t.Fatalf("write notes.txt: %v", err)
	}
	big := filepath.Join(dir, "big.png")
	if err := os.WriteFile(big, []byte(strings.Repeat("x", 6*1024*1024+1)), 0o644); err != nil {
		t.Fatalf("write big.png: %v", err)
	}
	subdir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	for _, path := range []string{missing, txt, big, subdir} {
		m.input.SetValue("/image " + path)
		m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		if len(m.pendingAttachments) != 0 {
			t.Fatalf("%s must not attach, got %d pending", path, len(m.pendingAttachments))
		}
	}
	if len(m.blocks) != 4 {
		t.Fatalf("blocks = %d, want 4 (one inline error per rejection)", len(m.blocks))
	}

	view := m.View()
	if !strings.Contains(view, "\x1b[38;2;224;108;117m") {
		t.Fatalf("attachment errors missing colError ANSI:\n%q", view)
	}
	plain := strings.ReplaceAll(stripANSI(m.contentRaw), "\n", "")
	for _, want := range []string{
		"cannot attach " + missing,
		"unsupported image format .txt — allowed: png, jpg, jpeg, gif, webp",
		"big.png is 6.0MB — the limit is 5MB",
		"cannot attach " + subdir + ": not a file",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("inline error missing %q:\n%s", want, plain)
		}
	}
	if m.state != stateChat {
		t.Fatalf("state = %q, want chat (prompt not aborted)", m.state)
	}
	if strings.Contains(stripANSI(m.View()), "🖼") {
		t.Fatalf("no chip should render for rejected attachments:\n%s", stripANSI(m.View()))
	}
	if want := 30 - defaultPromptHeight - viewChromeRows; m.vp.Height != want {
		t.Fatalf("viewport height = %d, want %d (no chips chrome)", m.vp.Height, want)
	}
}

func TestUnimageRemovesAttachment(t *testing.T) {
	dir := chdirTemp(t)
	writeFile(t, dir, "one.png", "a")
	writeFile(t, dir, "two.png", "b")

	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m.input.SetValue("/image " + filepath.Join(dir, "one.png"))
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m.input.SetValue("/image " + filepath.Join(dir, "two.png"))
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.pendingAttachments) != 2 {
		t.Fatalf("pendingAttachments = %d, want 2", len(m.pendingAttachments))
	}

	m.input.SetValue("/unimage 1")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if len(m.pendingAttachments) != 1 {
		t.Fatalf("pendingAttachments after /unimage 1 = %d, want 1", len(m.pendingAttachments))
	}
	if m.pendingAttachments[0].name != "two.png" {
		t.Fatalf("remaining attachment = %q, want two.png", m.pendingAttachments[0].name)
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "🖼 two.png ×") {
		t.Fatalf("chip for two.png missing:\n%s", view)
	}
	if strings.Contains(view, "🖼 one.png ×") {
		t.Fatalf("chip for one.png must be gone:\n%s", view)
	}
	if !strings.Contains(stripANSI(m.contentRaw), "removed one.png") {
		t.Fatalf("chat missing removal confirmation:\n%s", stripANSI(m.contentRaw))
	}

	for _, bad := range []string{"0", "3", "abc"} {
		m.input.SetValue("/unimage " + bad)
		m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		if len(m.pendingAttachments) != 1 {
			t.Fatalf("/unimage %s must not remove anything, pendings = %d", bad, len(m.pendingAttachments))
		}
	}
	if !strings.Contains(stripANSI(m.contentRaw), "no attachment 3") {
		t.Fatalf("chat missing out-of-range error:\n%s", stripANSI(m.contentRaw))
	}

	m.input.SetValue("/unimage")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.pendingAttachments) != 1 {
		t.Fatal("/unimage without args must not remove anything")
	}
	if !strings.Contains(stripANSI(m.contentRaw), "/unimage <n>") {
		t.Fatalf("chat missing usage error:\n%s", stripANSI(m.contentRaw))
	}
}

func TestSendWithAttachmentsBuildsParts(t *testing.T) {
	forceTrueColor(t)
	dir := chdirTemp(t)
	writeFile(t, dir, "shot.png", "pngdata")

	gw, bodies := captureGateway(t)
	m, ag := newAgentModel(t, gw.URL, false)

	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m.input.SetValue("/image " + filepath.Join(dir, "shot.png"))
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.pendingAttachments) != 1 {
		t.Fatalf("pendingAttachments = %d, want 1 before send", len(m.pendingAttachments))
	}

	m.input.SetValue("what is in this image?")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.busy {
		t.Fatal("expected busy=true after enter with attachments")
	}

	done := waitForAgentEvent(t, ag.Events, agent.EventTurnDone)
	m = step(t, m, agentEventMsg{event: done})

	var body string
	select {
	case body = <-bodies:
	default:
		t.Fatal("gateway captured no request body")
	}
	if !strings.Contains(body, `"content":[{"type":"text","text":"what is in this image?"},{"type":"image_url","image_url":{"url":"data:image/png;base64,`) {
		t.Fatalf("request content must be a parts array with text then image_url:\n%s", body)
	}

	var req struct {
		Messages []llm.Message `json:"messages"`
	}
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	var userMsg *llm.Message
	for i := range req.Messages {
		if req.Messages[i].Role == "user" {
			userMsg = &req.Messages[i]
		}
	}
	if userMsg == nil {
		t.Fatal("request has no user message")
	}
	if len(userMsg.ContentParts) != 2 {
		t.Fatalf("user message parts = %d, want 2 (text + image)", len(userMsg.ContentParts))
	}
	if userMsg.ContentParts[0].Type != "text" || userMsg.ContentParts[0].Text != "what is in this image?" {
		t.Fatalf("first part = %+v, want text part with the prompt", userMsg.ContentParts[0])
	}
	second := userMsg.ContentParts[1]
	if second.Type != "image_url" || second.ImageURL == nil {
		t.Fatalf("second part = %+v, want image_url part", second)
	}
	wantURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("pngdata"))
	if second.ImageURL.URL != wantURI {
		t.Fatalf("image url = %q, want %q", second.ImageURL.URL, wantURI)
	}

	plain := stripANSI(m.contentRaw)
	if !strings.Contains(plain, "what is in this image?") {
		t.Fatalf("chat missing sent text:\n%s", plain)
	}
	if !strings.Contains(plain, "🖼 shot.png ×") {
		t.Fatalf("chat user block missing attachment chip:\n%s", plain)
	}
	if !strings.Contains(m.contentRaw, "\x1b[38;2;92;156;245m🖼 shot.png ×") {
		t.Fatalf("user block chip missing colSecondary ANSI:\n%q", m.contentRaw)
	}
	if len(m.pendingAttachments) != 0 {
		t.Fatalf("pendingAttachments = %d, want 0 after send", len(m.pendingAttachments))
	}
	if want := 30 - defaultPromptHeight - viewChromeRows; m.vp.Height != want {
		t.Fatalf("viewport height after send = %d, want %d (chips chrome gone)", m.vp.Height, want)
	}
}

func TestImageWithoutArgsLists(t *testing.T) {
	dir := chdirTemp(t)
	writeFile(t, dir, "pic.jpg", "jpgdata")

	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m.input.SetValue("/image")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	plain := stripANSI(m.contentRaw)
	if !strings.Contains(plain, "no pending attachments") {
		t.Fatalf("empty list missing no-pending message:\n%s", plain)
	}

	m.input.SetValue("/image " + filepath.Join(dir, "pic.jpg"))
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.pendingAttachments) != 1 {
		t.Fatalf("pendingAttachments = %d, want 1", len(m.pendingAttachments))
	}
	if !strings.HasPrefix(m.pendingAttachments[0].dataURI, "data:image/jpeg;base64,") {
		t.Fatalf("jpg must map to image/jpeg, got %q", m.pendingAttachments[0].dataURI)
	}

	m.input.SetValue("/image")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	plain = stripANSI(m.contentRaw)
	if !strings.Contains(plain, "1. pic.jpg (7B)") {
		t.Fatalf("list missing pending entry:\n%s", plain)
	}
}

func noVisionGateway(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "glm-5.2"}}})
	})
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("chat completions must not be called when no vision model is available")
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func failingVisionGateway(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", gatewayModelsHandler(t))
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func pumpUntilAgentError(t *testing.T, m Model, ch chan agent.Event) (Model, agent.Event) {
	t.Helper()
	for {
		select {
		case e := <-ch:
			m = step(t, m, agentEventMsg{event: e})
			if e.Kind == agent.EventError {
				return m, e
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for error event")
		}
	}
}

func TestRoutingErrorPreservesAttachments(t *testing.T) {
	dir := chdirTemp(t)
	writeFile(t, dir, "shot.png", "pngdata")

	gw := noVisionGateway(t)
	m, ag := newAgentModel(t, gw.URL, false)

	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m.input.SetValue("/image " + filepath.Join(dir, "shot.png"))
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m.input.SetValue("what is this?")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.busy {
		t.Fatal("expected busy=true after enter with attachments")
	}
	if len(m.pendingAttachments) != 0 {
		t.Fatalf("pendingAttachments = %d, want 0 right after send", len(m.pendingAttachments))
	}

	m, errEv := pumpUntilAgentError(t, m, ag.Events)

	if errEv.Text != "no vision-capable model available" {
		t.Fatalf("error = %q, want no vision-capable model available", errEv.Text)
	}
	if m.busy {
		t.Fatal("busy must be false after the routing error")
	}
	if len(m.pendingAttachments) != 1 {
		t.Fatalf("pendingAttachments = %d, want 1 restored after the routing error", len(m.pendingAttachments))
	}
	if m.pendingAttachments[0].name != "shot.png" {
		t.Fatalf("restored attachment = %q, want shot.png", m.pendingAttachments[0].name)
	}
	if !strings.Contains(stripANSI(m.View()), "🖼 shot.png ×") {
		t.Fatalf("chip must render again after restore:\n%s", stripANSI(m.View()))
	}
	if want := 30 - defaultPromptHeight - viewChromeRows - 1; m.vp.Height != want {
		t.Fatalf("viewport height = %d, want %d (chips chrome restored)", m.vp.Height, want)
	}
	if !strings.Contains(stripANSI(m.contentRaw), "no vision-capable model available") {
		t.Fatalf("chat missing the friendly error:\n%s", stripANSI(m.contentRaw))
	}
}

func TestStreamErrorAfterRouteKeepsAttachmentsConsumed(t *testing.T) {
	dir := chdirTemp(t)
	writeFile(t, dir, "shot.png", "pngdata")

	gw := failingVisionGateway(t)
	m, ag := newAgentModel(t, gw.URL, false)

	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m.input.SetValue("/image " + filepath.Join(dir, "shot.png"))
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m.input.SetValue("describe it")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	m, _ = pumpUntilAgentError(t, m, ag.Events)

	if len(m.pendingAttachments) != 0 {
		t.Fatalf("pendingAttachments = %d, want 0 — the turn was consumed before the error", len(m.pendingAttachments))
	}
	if m.busy {
		t.Fatal("busy must be false after the error")
	}
}

func TestNestedEventsRenderIndented(t *testing.T) {
	forceTrueColor(t)
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventRoute, Model: "main-model", Router: "jev", Confidence: 0.9}})
	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventRoute, Model: "sub-model", Router: "jev", Confidence: 0.8, Depth: 1}})
	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventDelta, Model: "sub-model", Text: "sub text", Depth: 1}})
	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventToolStart, Tool: "bash", Args: `{"command":"echo hi"}`, Depth: 1}})
	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventToolResult, Tool: "bash", Result: "hi", Depth: 1}})

	content := m.vp.View()
	if !strings.Contains(content, "\x1b[38;2;92;156;245m  ⚡ sub-model · jev 0.80") {
		t.Fatalf("nested route missing 2-space colSecondary ANSI:\n%q", content)
	}
	if !strings.Contains(content, "\x1b[38;2;92;156;245m  ● bash({\"command\":\"echo hi\"})") {
		t.Fatalf("nested tool line missing 2-space colSecondary ANSI:\n%q", content)
	}
	if !strings.Contains(content, "\x1b[38;2;92;156;245m    hi") {
		t.Fatalf("nested result missing colSecondary indent ANSI:\n%q", content)
	}
	if !strings.Contains(content, "\x1b[38;2;128;128;128m⚡ main-model · jev 0.90") {
		t.Fatalf("main route must keep colTextMuted without indent:\n%q", content)
	}

	plain := stripANSI(content)
	mainIdx := strings.Index(plain, "⚡ main-model")
	nestedIdx := strings.Index(plain, "  ⚡ sub-model")
	if mainIdx < 0 || nestedIdx < 0 {
		t.Fatalf("content missing main or nested route:\n%s", plain)
	}
	if mainIdx > nestedIdx {
		t.Fatalf("nested route must render after the main route:\n%s", plain)
	}
	if strings.Contains(plain, "sub text") {
		t.Fatalf("nested deltas must not leak into the chat:\n%s", plain)
	}
	if m.stream.Len() != 0 {
		t.Fatalf("nested deltas must not append to the main stream, len=%d", m.stream.Len())
	}
	if m.currentModel != "main-model" {
		t.Fatalf("nested route clobbered currentModel = %q, want main-model", m.currentModel)
	}
	if m.routerInfo != "jev 0.90" {
		t.Fatalf("nested route clobbered routerInfo = %q, want jev 0.90", m.routerInfo)
	}

	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventRoute, Model: "deep-model", Router: "jev", Confidence: 0.7, Depth: 2}})
	if !strings.Contains(m.vp.View(), "\x1b[38;2;92;156;245m    ⚡ deep-model · jev 0.70") {
		t.Fatalf("depth 2 must indent 4 spaces in colSecondary:\n%q", m.vp.View())
	}
}

func TestHintBarShowsSubagentRunning(t *testing.T) {
	forceTrueColor(t)
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m.busy = true

	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventToolStart, Tool: "task", Args: `{"description":"do stuff"}`}})
	if hint := m.hintBar(); !strings.Contains(hint, "subagent running") {
		t.Fatalf("hint bar missing subagent running during task:\n%s", hint)
	}
	if !strings.Contains(m.View(), "subagent running") {
		t.Fatalf("view missing subagent running during task:\n%s", m.View())
	}
	if !strings.Contains(m.vp.View(), "\x1b[38;2;86;182;194m● task(") {
		t.Fatalf("task tool line must use the normal tool style without indent:\n%q", m.vp.View())
	}
	if !m.busy || !m.subagentActive {
		t.Fatal("busy and subagentActive must be set during the task tool")
	}

	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventToolResult, Tool: "task", Result: "done"}})
	if hint := m.hintBar(); strings.Contains(hint, "subagent running") {
		t.Fatalf("hint bar must drop subagent running after the task result:\n%s", hint)
	}
	if !strings.Contains(m.hintBar(), "Thinking") {
		t.Fatalf("hint bar must show Thinking again while the turn continues:\n%s", m.hintBar())
	}
	if !m.busy {
		t.Fatal("busy must remain true after the task result — the turn continues")
	}

	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventToolStart, Tool: "task", Args: `{"description":"more"}`}})
	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventTurnAborted, Model: "glm-5.2"}})
	if hint := m.hintBar(); strings.Contains(hint, "subagent running") {
		t.Fatalf("hint bar must drop subagent running when the turn aborts without a task result:\n%s", hint)
	}
	if m.busy {
		t.Fatal("busy must be false after turn aborted")
	}
	if m.subagentActive {
		t.Fatal("subagentActive must be cleared when the turn aborts without a task result")
	}
}

func TestNestedConfirmPausesApproval(t *testing.T) {
	ch := make(chan bool, 1)
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventConfirm, Tool: "bash", Args: `{"command":"rm -rf x"}`, ApproveCh: ch, Depth: 1}})

	if m.state != stateConfirm {
		t.Fatalf("state = %q, want confirm for nested tool", m.state)
	}
	if m.pendingConfirm == nil || m.pendingConfirm.Tool != "bash" {
		t.Fatalf("pendingConfirm must hold the nested tool call: %+v", m.pendingConfirm)
	}
	if !strings.Contains(m.View(), "Confirm tool execution") {
		t.Fatalf("view missing confirm dialog:\n%s", m.View())
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

	if m.state != stateChat {
		t.Fatalf("state = %q, want chat after approval", m.state)
	}
	select {
	case approved := <-ch:
		if !approved {
			t.Fatal("approval must send true on the nested confirm channel")
		}
	default:
		t.Fatal("approval must send on the nested confirm channel")
	}
}

func askGateway(t *testing.T, askArgs string) *httptest.Server {
	t.Helper()
	responses := [][]string{
		toolCallChunks("call_1", "ask_user", askArgs),
		contentChunks("noted", 20, 1),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", gatewayModelsHandler(t))
	var call int
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if call >= len(responses) {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		chunks := responses[call]
		call++
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for _, c := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", c)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func wizardQuestions() []agent.AskQuestion {
	return []agent.AskQuestion{
		{
			Question: "Which platform do you target?",
			Header:   "Platform",
			Options: []agent.AskOption{
				{Label: "Claude Code", Description: "native skills"},
				{Label: "Cursor"},
			},
		},
		{
			Question: "Which versions?",
			Multiple: true,
			Options: []agent.AskOption{
				{Label: "v1"},
				{Label: "v2"},
			},
		},
	}
}

func newAskWizardModel(t *testing.T, questions []agent.AskQuestion) (Model, chan []string) {
	t.Helper()
	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	ch := make(chan []string, 1)
	m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventAskUser, Questions: questions, AnswerCh: ch}})
	return m, ch
}

func TestAskWizardOpensOnEvent(t *testing.T) {
	m, _ := newAskWizardModel(t, wizardQuestions())
	if m.state != stateAsk || m.pendingAsk == nil || m.ask == nil {
		t.Fatalf("state=%q pendingAsk=%v ask=%v, want ask state", m.state, m.pendingAsk, m.ask)
	}
	view := stripANSI(m.View())
	for _, want := range []string{
		"Platform · 1/2",
		"Which platform do you target?",
		"Claude Code",
		"native skills",
		"Cursor",
		"Type your own answer",
		"↑↓ move",
		"enter confirm",
		"esc cancel",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("ask view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "space toggle") {
		t.Fatalf("single-select question must not show the space hint:\n%s", view)
	}
}

func TestAskWizardSequentialFlow(t *testing.T) {
	m, ch := newAskWizardModel(t, wizardQuestions())

	m = step(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.ask.cursor != 1 {
		t.Fatalf("cursor = %d, want 1 after down", m.ask.cursor)
	}
	m = step(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if m.ask.cursor != 0 {
		t.Fatalf("cursor = %d, want 0 after up", m.ask.cursor)
	}
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.ask.current != 1 {
		t.Fatalf("current = %d, want 1 after the first confirm", m.ask.current)
	}
	if len(m.ask.answers) != 1 || m.ask.answers[0] != "Claude Code" {
		t.Fatalf("answers = %v, want [Claude Code]", m.ask.answers)
	}
	if m.state != stateAsk {
		t.Fatalf("state = %q, want the wizard to stay open for the second question", m.state)
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "Which versions?") || !strings.Contains(view, "space toggle") {
		t.Fatalf("second question not rendered with the multi-select hint:\n%s", view)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = step(t, m, tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	select {
	case answers := <-ch:
		if len(answers) != 2 || answers[0] != "Claude Code" || answers[1] != "v1, v2" {
			t.Fatalf("answers = %v, want [Claude Code, v1, v2]", answers)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("answers not sent after the last confirm")
	}
	if m.state != stateChat || m.pendingAsk != nil || m.ask != nil {
		t.Fatalf("state=%q pendingAsk=%v ask=%v, want chat cleared", m.state, m.pendingAsk, m.ask)
	}
}

func TestAskWizardMultiSelectEnterFallback(t *testing.T) {
	m, ch := newAskWizardModel(t, []agent.AskQuestion{{
		Question: "Which versions?",
		Multiple: true,
		Options: []agent.AskOption{
			{Label: "v1"},
			{Label: "v2"},
		},
	}})

	m = step(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	select {
	case answers := <-ch:
		if len(answers) != 1 || answers[0] != "v2" {
			t.Fatalf("answers = %v, want the highlighted option when nothing is toggled", answers)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("answers not sent")
	}
}

func TestAskWizardFreeText(t *testing.T) {
	m, ch := newAskWizardModel(t, []agent.AskQuestion{{
		Question: "What is your name?",
		Options:  []agent.AskOption{{Label: "Anonymous"}},
	}})

	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Zed")})
	if !m.ask.onTextRow() {
		t.Fatal("typing must move the cursor to the free-text row")
	}
	if m.ask.text.Value() != "Zed" {
		t.Fatalf("text = %q, want Zed", m.ask.text.Value())
	}
	m = step(t, m, tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Dev")})
	if m.ask.text.Value() != "Zed Dev" {
		t.Fatalf("text = %q, want 'Zed Dev'", m.ask.text.Value())
	}
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	select {
	case answers := <-ch:
		if len(answers) != 1 || answers[0] != "Zed Dev" {
			t.Fatalf("answers = %v, want [Zed Dev]", answers)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("free-text answer not sent")
	}
}

func TestAskWizardEscDeclines(t *testing.T) {
	m, ch := newAskWizardModel(t, wizardQuestions())

	m = step(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	select {
	case answers := <-ch:
		if answers != nil {
			t.Fatalf("answers = %v, want nil on esc", answers)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("decline not sent after esc")
	}
	if m.state != stateChat || m.pendingAsk != nil || m.ask != nil {
		t.Fatalf("state=%q pendingAsk=%v ask=%v, want chat cleared", m.state, m.pendingAsk, m.ask)
	}
}

func TestAskWizardEnterOnEmptyTextNoOp(t *testing.T) {
	m, ch := newAskWizardModel(t, []agent.AskQuestion{{
		Question: "Name?",
		Options:  []agent.AskOption{{Label: "Ada"}},
	}})

	m = step(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if !m.ask.onTextRow() {
		t.Fatal("down must reach the free-text row")
	}
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	select {
	case answers := <-ch:
		t.Fatalf("channel received %v, want nothing on empty-text enter", answers)
	default:
	}
	if m.state != stateAsk {
		t.Fatalf("state = %q, want the wizard to stay open on empty-text enter", m.state)
	}
	if len(m.ask.answers) != 0 {
		t.Fatalf("answers = %v, want none recorded", m.ask.answers)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Grace")})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	select {
	case answers := <-ch:
		if len(answers) != 1 || answers[0] != "Grace" {
			t.Fatalf("answers = %v, want [Grace]", answers)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("answer not sent after typing")
	}
}

func TestAskUserTurnFlow(t *testing.T) {
	askArgs := `{"questions":[{"question":"Which platform?","options":[{"label":"Claude Code"},{"label":"Cursor"}]}]}`
	gw := askGateway(t, askArgs)
	m, ag := newAgentModel(t, gw.URL, false)
	ag.AttachAskUserTool()

	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m.input.SetValue("ask me")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.busy {
		t.Fatal("expected busy=true after enter")
	}

	ask := waitForAgentEvent(t, ag.Events, agent.EventAskUser)
	m = step(t, m, agentEventMsg{event: ask})
	if m.state != stateAsk {
		t.Fatalf("state = %q, want ask", m.state)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	res := waitForAgentEvent(t, ag.Events, agent.EventToolResult)
	if res.Tool != "ask_user" || res.Result != "Q: Which platform?\nA: Cursor" {
		t.Fatalf("tool result = %q %q, want the formatted ask_user answer", res.Tool, res.Result)
	}
	m = step(t, m, agentEventMsg{event: res})

	done := waitForAgentEvent(t, ag.Events, agent.EventTurnDone)
	m = step(t, m, agentEventMsg{event: done})
	if m.busy {
		t.Fatal("expected busy=false after turn done")
	}
	if m.state != stateChat {
		t.Fatalf("state = %q, want chat", m.state)
	}
}
