package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"kterminal/internal/agent"
)

const askTextRowLabel = "Type your own answer"

type askWizard struct {
	questions []agent.AskQuestion
	answers   []string
	current   int
	cursor    int
	toggles   map[int]bool
	text      textinput.Model
}

func newAskWizard(questions []agent.AskQuestion, width int) *askWizard {
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = "type your answer"
	input.CharLimit = 0
	input.Cursor.SetMode(cursor.CursorStatic)
	input.Cursor.Style = lipgloss.NewStyle().Background(colBgElement).Foreground(colText)
	if width > 0 {
		input.Width = maxInt(width-14, 20)
	}
	input.Focus()
	return &askWizard{
		questions: questions,
		toggles:   map[int]bool{},
		text:      input,
	}
}

func (w *askWizard) question() agent.AskQuestion {
	return w.questions[w.current]
}

func (w *askWizard) onTextRow() bool {
	return w.cursor >= len(w.question().Options)
}

func (w *askWizard) move(up bool) {
	n := len(w.question().Options) + 1
	if up {
		w.cursor = (w.cursor - 1 + n) % n
		return
	}
	w.cursor = (w.cursor + 1) % n
}

func (w *askWizard) toggle() {
	if w.toggles[w.cursor] {
		delete(w.toggles, w.cursor)
		return
	}
	w.toggles[w.cursor] = true
}

func (w *askWizard) next() {
	w.current++
	w.cursor = 0
	w.toggles = map[int]bool{}
	w.text.SetValue("")
}

func (w *askWizard) answer() (string, bool) {
	q := w.question()
	if w.onTextRow() {
		text := strings.TrimSpace(w.text.Value())
		if text == "" {
			return "", false
		}
		return text, true
	}
	if q.Multiple {
		labels := make([]string, 0, len(q.Options))
		for i, opt := range q.Options {
			if w.toggles[i] {
				labels = append(labels, opt.Label)
			}
		}
		if len(labels) == 0 {
			return q.Options[w.cursor].Label, true
		}
		return strings.Join(labels, ", "), true
	}
	return q.Options[w.cursor].Label, true
}

func (m Model) handleAskKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.pendingAsk == nil || m.ask == nil {
		m.state = stateChat
		return m, nil
	}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		return m.declineAsk()
	case "up":
		m.ask.move(true)
		return m, nil
	case "down":
		m.ask.move(false)
		return m, nil
	case " ":
		if m.ask.onTextRow() {
			var cmd tea.Cmd
			m.ask.text, cmd = m.ask.text.Update(msg)
			return m, cmd
		}
		if m.ask.question().Multiple {
			m.ask.toggle()
		}
		return m, nil
	case "enter":
		return m.confirmAsk()
	}
	if len(msg.Runes) > 0 && !m.ask.onTextRow() {
		m.ask.cursor = len(m.ask.question().Options)
	}
	var cmd tea.Cmd
	m.ask.text, cmd = m.ask.text.Update(msg)
	return m, cmd
}

func (m Model) confirmAsk() (tea.Model, tea.Cmd) {
	answer, ok := m.ask.answer()
	if !ok {
		return m, nil
	}
	m.ask.answers = append(m.ask.answers, answer)
	if m.ask.current+1 < len(m.ask.questions) {
		m.ask.next()
		return m, nil
	}
	return m.finishAsk(m.ask.answers)
}

func (m Model) declineAsk() (tea.Model, tea.Cmd) {
	m.pendingAsk.AnswerCh <- nil
	m.pendingAsk = nil
	m.ask = nil
	m.state = stateChat
	return m, nil
}

func (m Model) finishAsk(answers []string) (tea.Model, tea.Cmd) {
	m.pendingAsk.AnswerCh <- answers
	m.pendingAsk = nil
	m.ask = nil
	m.state = stateChat
	return m, nil
}

func (m Model) askView() string {
	if m.pendingAsk == nil || m.ask == nil {
		return ""
	}
	w := m.ask
	q := w.question()
	var b strings.Builder
	b.WriteString(titleStyle.Render(askHeader(w)))
	b.WriteString("\n\n")
	b.WriteString(textStyle.Render(q.Question))
	b.WriteString("\n\n")
	for i, opt := range q.Options {
		b.WriteString(askOptionLine(w, q, i))
		b.WriteString("\n")
		if opt.Description != "" {
			b.WriteString(helpStyle.Render("    " + opt.Description))
			b.WriteString("\n")
		}
	}
	b.WriteString(askTextRowLine(w))
	b.WriteString("\n")
	if w.onTextRow() {
		b.WriteString(configBoxStyle.Render(w.text.View()))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render(askHints(q)))
	return confirmBoxStyle.Render(b.String())
}

func askHeader(w *askWizard) string {
	q := w.question()
	if q.Header == "" {
		return fmt.Sprintf("Question %d of %d", w.current+1, len(w.questions))
	}
	if len(w.questions) > 1 {
		return fmt.Sprintf("%s · %d/%d", q.Header, w.current+1, len(w.questions))
	}
	return q.Header
}

func askOptionLine(w *askWizard, q agent.AskQuestion, i int) string {
	text := "  "
	if i == w.cursor {
		text = "▸ "
	}
	if q.Multiple {
		if w.toggles[i] {
			text += "[●] "
		} else {
			text += "[ ] "
		}
	}
	text += q.Options[i].Label
	if i == w.cursor {
		return mentionSelectedStyle.Render(text)
	}
	return mentionItemStyle.Render(text)
}

func askTextRowLine(w *askWizard) string {
	text := "  " + askTextRowLabel
	if w.onTextRow() {
		text = "▸ " + askTextRowLabel
		return mentionSelectedStyle.Render(text)
	}
	return mentionItemStyle.Render(text)
}

func askHints(q agent.AskQuestion) string {
	hints := []string{"↑↓ move"}
	if q.Multiple {
		hints = append(hints, "space toggle")
	}
	hints = append(hints, "enter confirm", "esc cancel")
	return strings.Join(hints, " · ")
}
