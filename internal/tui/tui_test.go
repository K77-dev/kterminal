package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"kterminal/internal/agent"
	"kterminal/internal/catalog"
	"kterminal/internal/config"
)

func newTestModel(t *testing.T) Model {
	t.Helper()
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	cfg := &config.Config{}
	ag := agent.New(nil, nil, nil, cat, nil, nil, false)
	return New(ag, cfg, cat, true)
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
