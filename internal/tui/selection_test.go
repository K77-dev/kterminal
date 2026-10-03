package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"kterminal/internal/agent"
)

func TestStripANSI(t *testing.T) {
	in := "\x1b[38;2;250;178;131mhello\x1b[0m world"
	want := "hello world"
	if got := stripANSI(in); got != want {
		t.Fatalf("stripANSI = %q, want %q", got, want)
	}
}

func TestCellToRuneCol(t *testing.T) {
	if got := cellToRuneCol("hello", 3); got != 3 {
		t.Fatalf("cellToRuneCol(hello,3) = %d, want 3", got)
	}
	if got := cellToRuneCol("hello", 0); got != 0 {
		t.Fatalf("cellToRuneCol(hello,0) = %d, want 0", got)
	}
	if got := cellToRuneCol("hello", 99); got != 5 {
		t.Fatalf("cellToRuneCol(hello,99) = %d, want 5", got)
	}
}

func TestSelectionText(t *testing.T) {
	lines := []string{"hello world", "second line", "third"}
	text := selectionText(lines, selPos{0, 6}, selPos{1, 6})
	want := "world\nsecond"
	if text != want {
		t.Fatalf("selectionText = %q, want %q", text, want)
	}
	text = selectionText(lines, selPos{1, 6}, selPos{0, 6})
	if text != want {
		t.Fatalf("reversed selectionText = %q, want %q", text, want)
	}
	text = selectionText(lines, selPos{2, 0}, selPos{2, 5})
	if text != "third" {
		t.Fatalf("single line = %q", text)
	}
}

func TestApplySelectionHighlight(t *testing.T) {
	content := "\x1b[38;2;1;2;3mhello\x1b[0m world\nsecond line"
	lines := plainLines(content)
	out := applySelection(content, lines, selPos{0, 1}, selPos{0, 4})
	if !strings.Contains(out, "\x1b[7me\x1b[27m") {
		t.Fatalf("missing reverse highlight:\n%q", out)
	}
	if !strings.Contains(out, "\x1b[38;2;1;2;3m") {
		t.Fatalf("original color codes lost:\n%q", out)
	}
	if !strings.Contains(out, "second line") {
		t.Fatalf("unselected line changed:\n%q", out)
	}
}

func TestMouseSelectionFlow(t *testing.T) {
	orig := copyToClipboard
	var copied string
	copyToClipboard = func(s string) error {
		copied = s
		return nil
	}
	defer func() { copyToClipboard = orig }()

	m := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 120, Height: 20})
	for i := 0; i < 10; i++ {
		m = step(t, m, agentEventMsg{event: agent.Event{Kind: agent.EventRoute, Model: "glm-5.2", Router: "jev", Confidence: 0.9}})
	}

	press := tea.MouseMsg(tea.MouseEvent{X: 0, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = step(t, m, press)
	if !m.selActive {
		t.Fatal("press should start selection")
	}

	drag := tea.MouseMsg(tea.MouseEvent{X: 10, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	m = step(t, m, drag)
	if !strings.Contains(m.vp.View(), "\x1b[7m") {
		t.Fatal("drag should highlight content")
	}

	release := tea.MouseMsg(tea.MouseEvent{X: 10, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
	m = step(t, m, release)
	if copied == "" {
		t.Fatal("release should copy selection")
	}
	if m.copyFlash == "" {
		t.Fatal("release should set copy flash")
	}
	if !strings.Contains(m.hintBar(), "copied") {
		t.Fatal("hint bar should show copy flash")
	}

	m = step(t, m, press)
	if m.copyFlash != "" {
		t.Fatal("new press should clear copy flash")
	}
}
