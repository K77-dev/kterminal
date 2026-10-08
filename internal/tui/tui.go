package tui

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"kterminal/internal/agent"
	"kterminal/internal/catalog"
	"kterminal/internal/clipboard"
	"kterminal/internal/config"
	"kterminal/internal/jev"
	"kterminal/internal/kspec"
	"kterminal/internal/llm"
	"kterminal/internal/router"
	"kterminal/internal/session"
	"kterminal/internal/tools"
)

const stateChat = "chat"
const stateConfig = "config"
const stateConfirm = "confirm"
const stateAsk = "ask"

const taskTool = "task"

const maxDiffVisibleLines = 40
const diffEdgeLines = 20
const resumedRenderLimit = 20
const liveBlockMaxLines = 15
const defaultPromptHeight = 3
const maxPromptHeight = 8
const minViewportRows = 3
const chatInset = 2
const statusIndent = 3
const maxPromptHistory = 20
const viewChromeRows = 5
const maxAttachmentBytes = 5 * 1024 * 1024

var imageMimes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
}

type attachment struct {
	name    string
	size    int64
	dataURI string
}

var thinkingFrames = spinner.Spinner{
	Frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
	FPS:    time.Second / 12,
}

type agentEventMsg struct{ event agent.Event }
type agentClosedMsg struct{}

type Model struct {
	agent      *agent.Agent
	cfg        *config.Config
	catalog    *catalog.Catalog
	kspecStore *kspec.Store

	state  string
	vp     viewport.Model
	input  textarea.Model
	spin   spinner.Model
	width  int
	height int
	cwd    string

	promptHistory []string
	histIdx       int

	blocks       []string
	stream       *strings.Builder
	liveLines    []string
	liveOmitted  int
	mdWidth      int
	resumed      []llm.Message
	resumedCount int

	mentionWarnings []string

	pendingAttachments []attachment
	sentAttachments    []attachment
	turnConsumed       bool

	mentionOpen     bool
	mentionItems    []string
	mentionSelected int

	cmdOpen       bool
	cmdItems      []string
	cmdSelected   int
	cmdSkillCache []string

	currentModel   string
	routerInfo     string
	sessionCost    float64
	lastTPS        float64
	busy           bool
	subagentActive bool

	cfgInputs []textinput.Model
	cfgFocus  int

	pendingConfirm *agent.Event
	pendingAsk     *agent.Event
	ask            *askWizard

	blink  tea.Cmd
	dark   bool
	follow bool

	contentRaw   string
	contentPlain []string
	selActive    bool
	selAnchor    selPos
	selCur       selPos
	copyFlash    string
}

type Option func(*Model)

func WithResumed(messages []llm.Message) Option {
	return func(m *Model) {
		m.resumed = messages
		m.resumedCount = len(messages)
	}
}

func WithKspec(s *kspec.Store) Option {
	return func(m *Model) {
		m.kspecStore = s
	}
}

func WithFreshWarning() Option {
	return func(m *Model) {
		m.blocks = append(m.blocks, resultStyle.Render("no previous session — starting fresh"))
	}
}

func New(ag *agent.Agent, cfg *config.Config, cat *catalog.Catalog, dark bool, opts ...Option) Model {
	input := textarea.New()
	input.Prompt = ""
	input.ShowLineNumbers = false
	input.Cursor.SetMode(cursor.CursorStatic)
	input.Cursor.Style = lipgloss.NewStyle().Background(colBg).Foreground(colPrimary)
	input.FocusedStyle.CursorLine = lipgloss.NewStyle()
	input.FocusedStyle.Text = lipgloss.NewStyle()
	input.SetHeight(defaultPromptHeight)
	blink := input.Focus()

	spin := spinner.New(spinner.WithSpinner(thinkingFrames))

	cwd, _ := os.Getwd()

	m := Model{
		agent:   ag,
		cfg:     cfg,
		catalog: cat,
		state:   stateChat,
		input:   input,
		spin:    spin,
		cwd:     cwd,
		stream:  &strings.Builder{},
		mdWidth: 80,
		blink:   blink,
		dark:    dark,
		follow:  true,
		histIdx: -1,
	}
	m.cfgInputs = newConfigInputs(cfg)
	for _, opt := range opts {
		opt(&m)
	}
	return m
}

func (m *Model) reconstructResumed() {
	if len(m.resumed) == 0 {
		return
	}
	msgs := m.resumed
	if len(msgs) > resumedRenderLimit {
		msgs = msgs[len(msgs)-resumedRenderLimit:]
	}
	for _, msg := range msgs {
		m.blocks = append(m.blocks, resumedMessageBlocks(msg, m.mdWidth, m.dark)...)
	}
	m.resumed = nil
}

func resumedMessageBlocks(msg llm.Message, width int, dark bool) []string {
	switch msg.Role {
	case "user":
		return []string{userBoxStyle.Render(msg.Content)}
	case "assistant":
		var blocks []string
		if md := renderMarkdown(msg.Content, width, dark); md != "" {
			blocks = append(blocks, md)
		}
		for _, tc := range msg.ToolCalls {
			blocks = append(blocks, indentLines(toolStyle.Render(fmt.Sprintf("● %s(%s)", tc.Function.Name, firstLine(tc.Function.Arguments, 100))), statusIndent))
		}
		if len(msg.ToolCalls) == 0 {
			blocks = append(blocks, indentLines(metaMarkStyle.Render("▣"), statusIndent))
		}
		return blocks
	case "tool":
		return []string{indentLines(resultStyle.Render(truncate(strings.ReplaceAll(msg.Content, "\n", " ⏎ "), 160)), statusIndent)}
	}
	return nil
}

func newConfigInputs(cfg *config.Config) []textinput.Model {
	url := textinput.New()
	url.Placeholder = "LiteLLM base URL (e.g. http://localhost:4000)"
	url.SetValue(cfg.LLM.BaseURL)
	url.CharLimit = 0

	llmKey := textinput.New()
	llmKey.Placeholder = "LiteLLM API key"
	llmKey.SetValue(cfg.LLM.APIKey)
	llmKey.CharLimit = 0
	llmKey.EchoMode = textinput.EchoPassword
	llmKey.EchoCharacter = '•'

	jevKey := textinput.New()
	jevKey.Placeholder = "Typesafe (Jev) API key"
	jevKey.SetValue(cfg.Typesafe.APIKey)
	jevKey.CharLimit = 0
	jevKey.EchoMode = textinput.EchoPassword
	jevKey.EchoCharacter = '•'

	skipTLS := textinput.New()
	skipTLS.Placeholder = "yes/no"
	skipTLS.SetValue(boolToYesNo(cfg.LLM.SkipTLSVerify))
	skipTLS.CharLimit = 3

	return []textinput.Model{url, llmKey, jevKey, skipTLS}
}

func boolToYesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func yesNoToBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "yes", "y", "true", "1":
		return true
	}
	return false
}

func listenAgent(ch chan agent.Event) tea.Cmd {
	return func() tea.Msg {
		e, ok := <-ch
		if !ok {
			return agentClosedMsg{}
		}
		return agentEventMsg{event: e}
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.blink, listenAgent(m.agent.Events))
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.width > 6 {
			m.mdWidth = m.width - 6
		}
		if w := m.width - 2*chatInset - 5; w > 0 {
			m.input.MaxWidth = w
			m.input.SetWidth(w)
		}
		m.fitPromptHeight()
		m.vp = viewport.New(maxInt(msg.Width-2*chatInset, 1), m.viewportHeight())
		if m.ask != nil {
			m.ask.text.Width = maxInt(msg.Width-14, 20)
		}
		m.reconstructResumed()
		m.refreshContent()
		return m, nil

	case agentClosedMsg:
		return m, nil

	case agentEventMsg:
		return m.handleAgentEvent(msg.event)

	case spinner.TickMsg:
		if m.busy {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.KeyMsg:
		switch m.state {
		case stateChat:
			return m.handleChatKey(msg)
		case stateConfig:
			return m.handleConfigKey(msg)
		case stateConfirm:
			return m.handleConfirmKey(msg)
		case stateAsk:
			return m.handleAskKey(msg)
		}

	case tea.MouseMsg:
		if m.state != stateChat {
			return m, nil
		}
		if tea.MouseEvent(msg).IsWheel() {
			var cmd tea.Cmd
			m.vp, cmd = m.vp.Update(msg)
			m.follow = m.vp.AtBottom()
			return m, cmd
		}
		switch msg.Button {
		case tea.MouseButtonLeft:
			switch msg.Action {
			case tea.MouseActionPress:
				m.copyFlash = ""
				if m.inViewport(msg.Y) {
					m.selActive = true
					m.selAnchor = m.contentPos(msg.X, msg.Y)
					m.selCur = m.selAnchor
				} else {
					m.selActive = false
				}
				m.refreshContent()
				return m, nil
			case tea.MouseActionMotion:
				if m.selActive && m.inViewport(msg.Y) {
					m.selCur = m.contentPos(msg.X, msg.Y)
					m.refreshContent()
				}
				return m, nil
			case tea.MouseActionRelease:
				if m.selActive {
					if m.inViewport(msg.Y) {
						m.selCur = m.contentPos(msg.X, msg.Y)
					}
					m.copySelection()
				}
				return m, nil
			}
		}
		return m, nil
	}

	var cmd tea.Cmd
	if m.state == stateChat {
		before := m.input.Value()
		m.input, cmd = m.input.Update(msg)
		m.handlePromptContentChange(before)
	}
	return m, cmd
}

func (m Model) handleAgentEvent(e agent.Event) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{listenAgent(m.agent.Events)}
	if e.Depth > 0 {
		m.handleNestedEvent(e)
		return m, tea.Batch(cmds...)
	}
	switch e.Kind {
	case agent.EventRoute:
		m.turnConsumed = true
		m.currentModel = e.Model
		m.routerInfo = fmt.Sprintf("%s %.2f", e.Router, e.Confidence)
		route := fmt.Sprintf("⚡ %s · %s", e.Model, m.routerInfo)
		if e.Reason != "" {
			route += " · " + truncate(e.Reason, 80)
		}
		line := indentLines(routeStyle.Render(route), statusIndent)
		m.blocks = append(m.blocks, line)
		m.refreshContent()
	case agent.EventCompaction:
		line := indentLines(compactionStyle.Render(fmt.Sprintf("⚡ context compacted (%s → %s tokens)", formatTokensK(e.TokensBefore), formatTokensK(e.TokensAfter))), statusIndent)
		m.blocks = append(m.blocks, line)
		m.refreshContent()
	case agent.EventDelta:
		m.stream.WriteString(e.Text)
		m.currentModel = e.Model
		m.refreshContent()
	case agent.EventToolStart:
		line := indentLines(toolStyle.Render(fmt.Sprintf("● %s(%s)", e.Tool, firstLine(e.Args, 100))), statusIndent)
		m.blocks = append(m.blocks, line)
		if e.Tool == taskTool {
			m.subagentActive = true
		}
		m.stream.Reset()
		m.resetLiveBlock()
		m.refreshContent()
	case agent.EventToolOutput:
		m.appendLiveOutput(e.Text)
		m.refreshContent()
	case agent.EventToolResult:
		m.resetLiveBlock()
		line := indentLines(resultStyle.Render(truncate(strings.ReplaceAll(e.Result, "\n", " ⏎ "), 160)), statusIndent)
		m.blocks = append(m.blocks, line)
		if block := renderDiffBlock(e.Diff); block != "" {
			m.blocks = append(m.blocks, block)
		}
		if e.Tool == taskTool {
			m.subagentActive = false
		}
		m.refreshContent()
	case agent.EventConfirm:
		ev := e
		m.pendingConfirm = &ev
		m.state = stateConfirm
	case agent.EventAskUser:
		ev := e
		m.pendingAsk = &ev
		m.ask = newAskWizard(e.Questions, m.width)
		m.state = stateAsk
	case agent.EventTurnDone:
		m.sentAttachments = nil
		turnCost := e.SessionCost - m.sessionCost
		m.sessionCost = e.SessionCost
		m.lastTPS = e.TPS
		if md := renderMarkdown(m.stream.String(), m.mdWidth, m.dark); md != "" {
			m.blocks = append(m.blocks, md)
		}
		meta := indentLines(metaMarkStyle.Render("▣ ")+metaStyle.Render(turnMeta(e, turnCost, m.routerInfo)), statusIndent)
		m.blocks = append(m.blocks, meta)
		m.stream.Reset()
		m.busy = false
		m.subagentActive = false
		m.refreshContent()
	case agent.EventTurnAborted:
		m.sentAttachments = nil
		if md := renderMarkdown(m.stream.String(), m.mdWidth, m.dark); md != "" {
			m.blocks = append(m.blocks, md)
		}
		meta := indentLines(metaMarkStyle.Render("⊘ ")+metaStyle.Render(abortMeta(e)), statusIndent)
		m.blocks = append(m.blocks, meta)
		m.stream.Reset()
		m.resetLiveBlock()
		m.busy = false
		m.subagentActive = false
		m.refreshContent()
	case agent.EventError:
		if !m.turnConsumed && len(m.sentAttachments) > 0 {
			m.pendingAttachments = append(m.pendingAttachments, m.sentAttachments...)
			m.fitViewportHeight()
		}
		m.sentAttachments = nil
		m.blocks = append(m.blocks, errorBoxStyle.Render(e.Text))
		m.stream.Reset()
		m.resetLiveBlock()
		m.busy = false
		m.subagentActive = false
		m.refreshContent()
	}
	return m, tea.Batch(cmds...)
}

func (m *Model) handleNestedEvent(e agent.Event) {
	switch e.Kind {
	case agent.EventRoute:
		m.blocks = append(m.blocks, indentLines(nestedStyle.Render(nestedPrefix(e.Depth)+fmt.Sprintf("⚡ %s · %s %.2f", e.Model, e.Router, e.Confidence)), statusIndent))
	case agent.EventToolStart:
		m.blocks = append(m.blocks, indentLines(nestedStyle.Render(nestedPrefix(e.Depth)+fmt.Sprintf("● %s(%s)", e.Tool, firstLine(e.Args, 100))), statusIndent))
	case agent.EventToolResult:
		m.blocks = append(m.blocks, indentLines(nestedStyle.Render(nestedPrefix(e.Depth)+"  "+truncate(strings.ReplaceAll(e.Result, "\n", " ⏎ "), 160)), statusIndent))
	case agent.EventConfirm:
		ev := e
		m.pendingConfirm = &ev
		m.state = stateConfirm
		return
	}
	m.refreshContent()
}

func nestedPrefix(depth int) string {
	if depth <= 0 {
		return ""
	}
	return strings.Repeat("  ", depth)
}

func (m *Model) resetLiveBlock() {
	m.liveLines = nil
	m.liveOmitted = 0
}

func (m *Model) appendLiveOutput(text string) {
	m.liveLines = append(m.liveLines, strings.Split(text, "\n")...)
	if len(m.liveLines) <= liveBlockMaxLines {
		return
	}
	overflow := len(m.liveLines) - liveBlockMaxLines
	m.liveOmitted += overflow
	kept := make([]string, liveBlockMaxLines)
	copy(kept, m.liveLines[overflow:])
	m.liveLines = kept
}

func (m Model) renderLiveBlock() string {
	if len(m.liveLines) == 0 {
		return ""
	}
	var b strings.Builder
	if m.liveOmitted > 0 {
		b.WriteString(resultStyle.Render(fmt.Sprintf("… +%d lines", m.liveOmitted)))
		b.WriteString("\n")
	}
	for i, line := range m.liveLines {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(resultStyle.Render(line))
	}
	return indentLines(b.String(), statusIndent)
}

func turnMeta(e agent.Event, cost float64, routerInfo string) string {
	parts := []string{e.Model}
	if routerInfo != "" {
		parts = append(parts, routerInfo)
	}
	if e.TPS > 0 {
		parts = append(parts, fmt.Sprintf("%.0f tok/s", e.TPS))
	}
	if cost > 0 {
		parts = append(parts, formatCost(cost))
	}
	return strings.Join(parts, " · ")
}

func abortMeta(e agent.Event) string {
	parts := []string{"interrompido"}
	if e.Model != "" {
		parts = append(parts, e.Model)
	}
	return strings.Join(parts, " · ")
}

func formatTokensK(n int64) string {
	if n < 1000 {
		return strconv.FormatInt(n, 10)
	}
	return fmt.Sprintf("%.1fk", float64(n)/1000)
}

func renderDiffBlock(diff []tools.DiffLine) string {
	if len(diff) == 0 {
		return ""
	}
	var b strings.Builder
	if len(diff) > maxDiffVisibleLines {
		writeDiffLines(&b, diff[:diffEdgeLines])
		b.WriteString("\n")
		b.WriteString(diffContextStyle.Render(fmt.Sprintf("… %d more lines …", len(diff)-maxDiffVisibleLines)))
		b.WriteString("\n")
		writeDiffLines(&b, diff[len(diff)-diffEdgeLines:])
		return indentLines(b.String(), statusIndent)
	}
	writeDiffLines(&b, diff)
	return indentLines(b.String(), statusIndent)
}

func writeDiffLines(b *strings.Builder, lines []tools.DiffLine) {
	for i, d := range lines {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(renderDiffLine(d))
	}
}

func renderDiffLine(d tools.DiffLine) string {
	switch d.Kind {
	case '+':
		return diffAddedStyle.Render("+" + d.Text)
	case '-':
		return diffRemovedStyle.Render("-" + d.Text)
	default:
		return diffContextStyle.Render(" " + d.Text)
	}
}

func (m Model) handleChatKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.copyFlash = ""
	if m.mentionOpen {
		switch msg.String() {
		case "tab", "enter":
			m.completeMention()
			return m, nil
		case "esc":
			m.closeMentionPopup()
			return m, nil
		case "up":
			if n := len(m.mentionItems); n > 0 {
				m.mentionSelected = (m.mentionSelected - 1 + n) % n
			}
			return m, nil
		case "down":
			if n := len(m.mentionItems); n > 0 {
				m.mentionSelected = (m.mentionSelected + 1) % n
			}
			return m, nil
		}
	}
	if m.cmdOpen {
		switch msg.String() {
		case "tab", "enter":
			m.completeCommand()
			return m, nil
		case "esc":
			m.closeCommandPopup()
			return m, nil
		case "up":
			if n := len(m.cmdItems); n > 0 {
				m.cmdSelected = (m.cmdSelected - 1 + n) % n
			}
			return m, nil
		case "down":
			if n := len(m.cmdItems); n > 0 {
				m.cmdSelected = (m.cmdSelected + 1) % n
			}
			return m, nil
		}
	}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.busy {
			m.agent.Cancel()
		}
		return m, nil
	case "pgup", "ctrl+alt+u":
		m.scrollUp(m.vp.Height / 2)
		return m, nil
	case "pgdown", "ctrl+alt+d":
		m.scrollDown(m.vp.Height / 2)
		return m, nil
	case "ctrl+alt+y":
		m.scrollUp(1)
		return m, nil
	case "ctrl+alt+e":
		m.scrollDown(1)
		return m, nil
	case "ctrl+g", "home":
		m.vp.GotoTop()
		m.follow = false
		return m, nil
	case "ctrl+alt+g", "end":
		m.vp.GotoBottom()
		m.follow = true
		return m, nil
	case "enter":
		value := strings.TrimSpace(m.input.Value())
		m.input.SetValue("")
		m.histIdx = -1
		m.fitPromptHeight()
		m.fitViewportHeight()
		if value == "" {
			return m, nil
		}
		if strings.HasPrefix(value, "/") {
			return m.handleCommand(value)
		}
		if m.busy {
			m.blocks = append(m.blocks, warningStyle.Render("busy — wait for the current task to finish"))
			m.refreshContent()
			return m, nil
		}
		expanded, warnings := ExpandMentions(value)
		m.mentionWarnings = warnings
		m.blocks = append(m.blocks, userBlock(value, m.pendingAttachments))
		m.pushPromptHistory(value)
		m.busy = true
		m.sentAttachments = nil
		m.turnConsumed = false
		if len(m.pendingAttachments) == 0 {
			m.agent.Run(expanded)
		} else {
			m.agent.RunWithAttachments(expanded, attachmentParts(expanded, m.pendingAttachments), attachmentMeta(m.pendingAttachments))
			m.sentAttachments = m.pendingAttachments
			m.pendingAttachments = nil
			m.fitViewportHeight()
		}
		m.refreshContent()
		return m, m.spin.Tick
	case "shift+enter", "alt+enter", "ctrl+j":
		before := m.input.Value()
		m.input.InsertString("\n")
		m.handlePromptContentChange(before)
		m.refreshSuggestions()
		return m, nil
	case "up", "down":
		if m.navigateHistory(msg.String() == "up") {
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.refreshSuggestions()
		return m, cmd
	}
	var cmd tea.Cmd
	before := m.input.Value()
	m.input, cmd = m.input.Update(msg)
	m.handlePromptContentChange(before)
	m.refreshSuggestions()
	return m, cmd
}

func (m *Model) handlePromptContentChange(before string) {
	if m.input.Value() == before {
		return
	}
	m.histIdx = -1
	m.fitPromptHeight()
	m.fitViewportHeight()
}

func (m *Model) fitPromptHeight() {
	h := max(defaultPromptHeight, min(m.input.LineCount(), maxPromptHeight))
	if h != m.input.Height() {
		m.input.SetHeight(h)
	}
}

func (m *Model) viewportHeight() int {
	chrome := viewChromeRows
	if len(m.pendingAttachments) > 0 {
		chrome++
	}
	return max(minViewportRows, m.height-m.input.Height()-chrome)
}

func (m *Model) fitViewportHeight() {
	if h := m.viewportHeight(); h != m.vp.Height {
		m.vp.Height = h
		if m.follow {
			m.vp.GotoBottom()
		} else {
			m.vp.SetYOffset(m.vp.YOffset)
		}
	}
}

func (m *Model) pushPromptHistory(prompt string) {
	m.promptHistory = append(m.promptHistory, prompt)
	if len(m.promptHistory) > maxPromptHistory {
		m.promptHistory = m.promptHistory[len(m.promptHistory)-maxPromptHistory:]
	}
}

func (m *Model) navigateHistory(up bool) bool {
	if len(m.promptHistory) == 0 {
		return false
	}
	if m.histIdx < 0 {
		if !up || m.input.Value() != "" {
			return false
		}
		m.histIdx = len(m.promptHistory) - 1
	} else if up {
		m.histIdx = max(0, m.histIdx-1)
	} else {
		m.histIdx++
		if m.histIdx >= len(m.promptHistory) {
			m.histIdx = -1
		}
	}
	if m.histIdx < 0 {
		m.input.SetValue("")
	} else {
		m.input.SetValue(m.promptHistory[m.histIdx])
	}
	m.closeMentionPopup()
	m.closeCommandPopup()
	m.fitPromptHeight()
	m.fitViewportHeight()
	return true
}

func (m *Model) closeMentionPopup() {
	m.mentionOpen = false
	m.mentionItems = nil
	m.mentionSelected = 0
}

func (m *Model) refreshMentionSuggestions() {
	m.closeMentionPopup()
	before := textBeforeCursor(m.input)
	prefix, ok := mentionPrefix(before)
	if !ok {
		return
	}
	m.mentionItems = mentionSuggestions(prefix)
	if len(m.mentionItems) == 0 {
		return
	}
	m.mentionOpen = true
}

func (m *Model) completeMention() {
	if !m.mentionOpen || len(m.mentionItems) == 0 {
		return
	}
	value := m.input.Value()
	before := textBeforeCursor(m.input)
	prefix, ok := mentionPrefix(before)
	if !ok {
		return
	}
	item := m.mentionItems[m.mentionSelected]
	completed := before[:len(before)-len(prefix)-1] + "@" + item
	next := completed + value[len(before):]
	m.input.SetValue(next)
	m.setPromptCursor(next, len(completed))
	m.histIdx = -1
	m.closeMentionPopup()
}

func (m *Model) setPromptCursor(value string, offset int) {
	row, col := promptRowCol(value, offset)
	for m.input.Line() > row {
		m.input.CursorUp()
	}
	m.input.SetCursor(col)
}

func promptRowCol(value string, offset int) (int, int) {
	seen := 0
	row := 0
	for _, line := range strings.Split(value, "\n") {
		if offset <= seen+len(line) {
			return row, len([]rune(line[:max(0, offset-seen)]))
		}
		seen += len(line) + 1
		row++
	}
	return row, 0
}

func textBeforeCursor(input textarea.Model) string {
	value := input.Value()
	lines := strings.Split(value, "\n")
	row := input.Line()
	if row >= len(lines) {
		row = len(lines) - 1
	}
	info := input.LineInfo()
	col := info.StartColumn + info.ColumnOffset
	runes := []rune(lines[row])
	if col < 0 || col > len(runes) {
		col = len(runes)
	}
	before := strings.Join(lines[:row], "\n")
	if row > 0 {
		before += "\n"
	}
	return before + string(runes[:col])
}

func mentionSuggestions(prefix string) []string {
	matches, err := filepath.Glob(prefix + "*")
	if err != nil {
		return nil
	}
	sort.Strings(matches)
	var items []string
	for _, match := range matches {
		if len(items) == maxMentionSuggestions {
			break
		}
		info, err := os.Stat(match)
		if err != nil || info.IsDir() {
			continue
		}
		items = append(items, match)
	}
	return items
}

func (m Model) mentionPopupView() string {
	if !m.mentionOpen || len(m.mentionItems) == 0 {
		return ""
	}
	var b strings.Builder
	for i, item := range m.mentionItems {
		if i > 0 {
			b.WriteString("\n")
		}
		if i == m.mentionSelected {
			b.WriteString(mentionSelectedStyle.Render("▸ " + item))
		} else {
			b.WriteString(mentionItemStyle.Render("  " + item))
		}
	}
	return mentionPopupStyle.Render(b.String())
}

func (m Model) handleCommand(cmdline string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(cmdline)
	cmd := parts[0]
	switch cmd {
	case "/quit", "/exit":
		return m, tea.Quit
	case "/help":
		lines := []string{
			"/config — configure gateway and API keys",
			"/model <name> — pin a model · /model auto — let Jev decide",
			"/image <path> — attach an image · /image — list pending · /unimage <n> — remove",
			"/clear — clear the conversation",
			"/help — this help",
			"/quit, /exit — exit",
		}
		if extra := kspecHelpLines(m.kspecStore); len(extra) > 0 {
			lines = append(lines, "")
			lines = append(lines, extra...)
		}
		m.blocks = append(m.blocks, helpStyle.Render(strings.Join(lines, "\n")))
		m.refreshContent()
		return m, nil
	case "/clear":
		if m.busy {
			m.blocks = append(m.blocks, warningStyle.Render("busy — wait for the current task to finish"))
			m.refreshContent()
			return m, nil
		}
		m.agent.Reset()
		m.blocks = append(m.blocks, resultStyle.Render("conversation cleared"))
		m.refreshContent()
		return m, nil
	case "/config":
		m.cfgInputs = newConfigInputs(m.cfg)
		m.cfgFocus = 0
		for i := range m.cfgInputs {
			m.cfgInputs[i].Blur()
		}
		m.cfgInputs[0].Focus()
		m.state = stateConfig
		return m, nil
	case "/model":
		if len(parts) < 2 || parts[1] == "auto" {
			m.agent.SetPinned("")
			m.blocks = append(m.blocks, resultStyle.Render("model pin removed — Jev decides again"))
			m.refreshContent()
			return m, nil
		}
		name := parts[1]
		if _, ok := m.catalog.Get(name); !ok {
			m.blocks = append(m.blocks, errorBoxStyle.Render("unknown model "+name+" — not in catalog"))
			m.refreshContent()
			return m, nil
		}
		m.agent.SetPinned(name)
		m.blocks = append(m.blocks, primaryStyle.Render("model pinned: "+name))
		m.refreshContent()
		return m, nil
	case "/image":
		arg := strings.TrimSpace(strings.TrimPrefix(cmdline, cmd))
		if arg == "" {
			m.listPendingAttachments()
		} else {
			m.attachImage(arg)
		}
		m.refreshContent()
		return m, nil
	case "/unimage":
		if len(parts) < 2 {
			m.blocks = append(m.blocks, errorBoxStyle.Render("/unimage <n> — n is the attachment number (see /image)"))
			m.refreshContent()
			return m, nil
		}
		m.removeAttachment(parts[1])
		m.refreshContent()
		return m, nil
	default:
		if strings.HasPrefix(cmd, kspecCommandPrefix) {
			return m.handleKspecCommand(cmdline)
		}
		m.blocks = append(m.blocks, errorBoxStyle.Render("unknown command "+cmd+" — /help"))
		m.refreshContent()
		return m, nil
	}
}

func (m *Model) attachImage(path string) {
	info, err := os.Stat(path)
	if err != nil {
		m.attachmentError("cannot attach " + path + ": " + err.Error())
		return
	}
	if info.IsDir() {
		m.attachmentError("cannot attach " + path + ": not a file")
		return
	}
	ext := strings.ToLower(filepath.Ext(path))
	mime, ok := imageMimes[ext]
	if !ok {
		label := ext
		if label == "" {
			label = "(no extension)"
		}
		m.attachmentError("unsupported image format " + label + " — allowed: png, jpg, jpeg, gif, webp")
		return
	}
	if info.Size() > maxAttachmentBytes {
		m.attachmentError(fmt.Sprintf("%s is %s — the limit is 5MB", filepath.Base(path), formatBytes(info.Size())))
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		m.attachmentError("cannot read " + path + ": " + err.Error())
		return
	}
	m.pendingAttachments = append(m.pendingAttachments, attachment{
		name:    filepath.Base(path),
		size:    info.Size(),
		dataURI: "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data),
	})
	m.fitViewportHeight()
}

func (m *Model) attachmentError(msg string) {
	m.blocks = append(m.blocks, errorBoxStyle.Render(msg))
}

func (m *Model) listPendingAttachments() {
	if len(m.pendingAttachments) == 0 {
		m.blocks = append(m.blocks, resultStyle.Render("no pending attachments — /image <path> to attach one"))
		return
	}
	lines := make([]string, 0, len(m.pendingAttachments))
	for i, att := range m.pendingAttachments {
		lines = append(lines, fmt.Sprintf("%d. %s (%s)", i+1, att.name, formatBytes(att.size)))
	}
	m.blocks = append(m.blocks, resultStyle.Render(strings.Join(lines, "\n")))
}

func (m *Model) removeAttachment(arg string) {
	n, err := strconv.Atoi(arg)
	if err != nil {
		m.attachmentError("/unimage <n> — n must be the attachment number (see /image)")
		return
	}
	if n < 1 || n > len(m.pendingAttachments) {
		m.attachmentError(fmt.Sprintf("no attachment %d — %d pending (see /image)", n, len(m.pendingAttachments)))
		return
	}
	removed := m.pendingAttachments[n-1]
	m.pendingAttachments = append(m.pendingAttachments[:n-1], m.pendingAttachments[n:]...)
	m.blocks = append(m.blocks, resultStyle.Render("removed "+removed.name))
	m.fitViewportHeight()
}

func renderAttachmentChips(atts []attachment) string {
	if len(atts) == 0 {
		return ""
	}
	chips := make([]string, len(atts))
	for i, att := range atts {
		chips[i] = chipStyle.Render(fmt.Sprintf("🖼 %s ×", att.name))
	}
	return strings.Join(chips, " ")
}

func userBlock(text string, atts []attachment) string {
	if len(atts) == 0 {
		return userBoxStyle.Render(text)
	}
	return userBoxStyle.Render(text + "\n" + renderAttachmentChips(atts))
}

func attachmentParts(text string, atts []attachment) []llm.ContentPart {
	parts := make([]llm.ContentPart, 0, len(atts)+1)
	parts = append(parts, llm.ContentPart{Type: "text", Text: text})
	for _, att := range atts {
		parts = append(parts, llm.ContentPart{Type: "image_url", ImageURL: &llm.ImageURL{URL: att.dataURI}})
	}
	return parts
}

func attachmentMeta(atts []attachment) []session.AttachmentMeta {
	if len(atts) == 0 {
		return nil
	}
	meta := make([]session.AttachmentMeta, len(atts))
	for i, att := range atts {
		meta[i] = session.AttachmentMeta{Name: att.name, Size: att.size}
	}
	return meta
}

func formatBytes(n int64) string {
	const kb = 1024
	const mb = kb * 1024
	switch {
	case n >= mb:
		return fmt.Sprintf("%.1fMB", float64(n)/mb)
	case n >= kb:
		return fmt.Sprintf("%.1fKB", float64(n)/kb)
	default:
		return fmt.Sprintf("%dB", n)
	}
}

func (m Model) handleConfigKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.state = stateChat
		return m, nil
	case "tab", "down":
		m.cfgInputs[m.cfgFocus].Blur()
		m.cfgFocus = (m.cfgFocus + 1) % len(m.cfgInputs)
		return m, m.cfgInputs[m.cfgFocus].Focus()
	case "shift+tab", "up":
		m.cfgInputs[m.cfgFocus].Blur()
		m.cfgFocus = (m.cfgFocus - 1 + len(m.cfgInputs)) % len(m.cfgInputs)
		return m, m.cfgInputs[m.cfgFocus].Focus()
	case "enter":
		if m.cfgFocus < len(m.cfgInputs)-1 {
			m.cfgInputs[m.cfgFocus].Blur()
			m.cfgFocus++
			return m, m.cfgInputs[m.cfgFocus].Focus()
		}
		return m.saveConfig()
	}
	var cmd tea.Cmd
	m.cfgInputs[m.cfgFocus], cmd = m.cfgInputs[m.cfgFocus].Update(msg)
	return m, cmd
}

func (m Model) saveConfig() (tea.Model, tea.Cmd) {
	m.cfg.LLM.BaseURL = strings.TrimSpace(m.cfgInputs[0].Value())
	m.cfg.LLM.APIKey = strings.TrimSpace(m.cfgInputs[1].Value())
	m.cfg.Typesafe.APIKey = strings.TrimSpace(m.cfgInputs[2].Value())
	m.cfg.LLM.SkipTLSVerify = yesNoToBool(m.cfgInputs[3].Value())
	if err := m.cfg.Save(); err != nil {
		m.blocks = append(m.blocks, errorBoxStyle.Render("failed to save config: "+err.Error()))
	} else {
		m.rebuildAgent()
		m.blocks = append(m.blocks, resultStyle.Render("config saved — gateway "+m.cfg.LLM.BaseURL))
	}
	m.state = stateChat
	m.refreshContent()
	return m, nil
}

func (m *Model) rebuildAgent() {
	var llmClient *llm.Client
	if m.cfg.Ready() {
		llmClient = llm.New(m.cfg.LLM.BaseURL, m.cfg.LLM.APIKey, m.cfg.LLM.SkipTLSVerify)
	}
	var jevRouter, fallback router.Router
	if m.cfg.Typesafe.APIKey != "" {
		jevRouter = router.NewJev(jev.New(m.cfg.Typesafe.APIKey))
	}
	fallback = &router.HeuristicRouter{Default: m.catalog.DefaultModel}
	if jevRouter == nil {
		jevRouter = fallback
		fallback = nil
	}
	m.agent.SetLLM(llmClient)
	m.agent.SetRouters(jevRouter, fallback)
}

func (m Model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.pendingConfirm == nil {
		m.state = stateChat
		return m, nil
	}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "y", "Y", "enter":
		m.pendingConfirm.ApproveCh <- true
	case "n", "N":
		m.pendingConfirm.ApproveCh <- false
	case "esc":
		m.agent.Cancel()
		m.pendingConfirm.ApproveCh <- false
	default:
		return m, nil
	}
	m.pendingConfirm = nil
	m.state = stateChat
	return m, listenAgent(m.agent.Events)
}

func (m *Model) refreshContent() {
	var b strings.Builder
	if len(m.blocks) == 0 && m.stream.Len() == 0 && len(m.liveLines) == 0 {
		b.WriteString(m.emptyState())
	} else {
		b.WriteString(strings.Join(m.blocks, "\n\n"))
		live := m.renderLiveBlock()
		if live != "" {
			if len(m.blocks) > 0 {
				b.WriteString("\n\n")
			}
			b.WriteString(live)
		}
		if m.stream.Len() > 0 {
			if len(m.blocks) > 0 || live != "" {
				b.WriteString("\n\n")
			}
			b.WriteString(m.stream.String())
		}
	}
	m.contentRaw = wrapContent(b.String(), m.vp.Width)
	m.contentPlain = plainLines(m.contentRaw)
	display := m.contentRaw
	if m.selActive {
		display = applySelection(m.contentRaw, m.contentPlain, m.selAnchor, m.selCur)
	}
	m.vp.SetContent(display)
	if m.follow {
		m.vp.GotoBottom()
	}
}

func (m *Model) scrollUp(lines int) {
	m.vp.LineUp(lines)
	m.follow = m.vp.AtBottom()
}

func (m *Model) scrollDown(lines int) {
	m.vp.LineDown(lines)
	m.follow = m.vp.AtBottom()
}

func (m *Model) inViewport(y int) bool {
	return y >= 0 && y < m.vp.Height
}

func (m *Model) contentPos(x, y int) selPos {
	line := m.vp.YOffset + y
	if line < 0 {
		line = 0
	}
	if line >= len(m.contentPlain) {
		line = len(m.contentPlain) - 1
	}
	col := 0
	if line < len(m.contentPlain) {
		col = cellToRuneCol(m.contentPlain[line], x-chatInset)
	}
	return selPos{line: line, col: col}
}

var copyToClipboard = clipboard.Copy

func (m *Model) copySelection() {
	text := selectionText(m.contentPlain, m.selAnchor, m.selCur)
	if strings.TrimSpace(text) == "" {
		return
	}
	if err := copyToClipboard(text); err != nil {
		m.copyFlash = "copy failed: " + err.Error()
		return
	}
	n := len([]rune(text))
	if n == 1 {
		m.copyFlash = "✓ copied 1 char"
	} else {
		m.copyFlash = fmt.Sprintf("✓ copied %d chars", n)
	}
}

func (m Model) emptyState() string {
	var b strings.Builder
	b.WriteString(renderLogo())
	b.WriteString("\n\n")
	if !m.cfg.Ready() {
		b.WriteString(textStyle.Render("No LLM gateway configured."))
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("Run "))
		b.WriteString(primaryStyle.Render("/config"))
		b.WriteString(helpStyle.Render(" to set your gateway base URL and API keys."))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("Jev will pick the best model for every single call."))
	} else {
		b.WriteString(textStyle.Render("Jev decides which model handles every call."))
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("quality · cost · speed — balanced per step. No model picking."))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("type /help for commands · /model <name> to pin"))
	}
	return b.String()
}

func (m Model) hintBar() string {
	left := ""
	if m.busy {
		if m.subagentActive {
			left = m.spin.View() + helpStyle.Render(" subagent running")
		} else {
			left = m.spin.View() + helpStyle.Render(" Thinking")
		}
	} else {
		left = helpStyle.Render(truncate(m.cwd, maxInt(m.width/3, 20)))
	}

	right := ""
	if m.copyFlash != "" {
		right += successStyle.Render(m.copyFlash) + helpStyle.Render(" · ")
	}
	if !m.follow {
		right += warningStyle.Render("↑ scrolled") + helpStyle.Render(" · end to jump · ")
	}
	if m.busy {
		right += warningStyle.Render("esc to interrupt") + helpStyle.Render(" · ")
	}
	right += textStyle.Render("?") + helpStyle.Render(" commands")
	if m.resumedCount > 0 {
		right += helpStyle.Render(fmt.Sprintf(" · resumed · %d mensagens", m.resumedCount))
	}
	if s := m.agent.ActiveSkill(); s != "" {
		right += helpStyle.Render(" · ") + primaryStyle.Render("skill "+s)
	}
	if p := m.agent.Pinned(); p != "" {
		right += helpStyle.Render(" · ") + warningStyle.Render("pinned "+p)
	}
	if m.currentModel != "" {
		right += helpStyle.Render(" · ") + textStyle.Render(m.currentModel)
	}
	if m.sessionCost > 0 {
		right += helpStyle.Render(" · " + formatCost(m.sessionCost))
	}
	if m.lastTPS > 0 {
		right += helpStyle.Render(fmt.Sprintf(" · %.0f tok/s", m.lastTPS))
	}

	gap := m.width - 2*chatInset - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return strings.Repeat(" ", chatInset) + left + strings.Repeat(" ", gap) + right
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m Model) View() string {
	if m.width == 0 {
		return "loading…"
	}
	switch m.state {
	case stateConfig:
		return m.configView()
	case stateConfirm:
		return m.confirmView()
	case stateAsk:
		return m.askView()
	}
	prompt := indentLines(promptBoxStyle.Render(m.input.View()), chatInset)
	corner := strings.Repeat(" ", chatInset) + fadeCornerStyle.Render("╹")
	var b strings.Builder
	b.WriteString(indentLines(m.vp.View(), chatInset))
	b.WriteString("\n\n")
	for _, w := range m.mentionWarnings {
		b.WriteString(indentLines(warningStyle.Render(w), chatInset))
		b.WriteString("\n")
	}
	if popup := m.mentionPopupView(); popup != "" {
		b.WriteString(indentLines(popup, chatInset))
		b.WriteString("\n")
	}
	if popup := m.commandPopupView(); popup != "" {
		b.WriteString(indentLines(popup, chatInset))
		b.WriteString("\n")
	}
	if chips := renderAttachmentChips(m.pendingAttachments); chips != "" {
		b.WriteString(indentLines(chips, chatInset))
		b.WriteString("\n")
	}
	b.WriteString(prompt)
	b.WriteString("\n")
	b.WriteString(corner)
	b.WriteString("\n")
	b.WriteString(m.hintBar())
	return b.String()
}

func (m Model) configView() string {
	labels := []string{"LiteLLM base URL", "LiteLLM API key", "Typesafe (Jev) API key", "Skip TLS verification (yes/no)"}
	var b strings.Builder
	b.WriteString(titleStyle.Render("config"))
	b.WriteString("\n\n")
	for i, input := range m.cfgInputs {
		box := configBoxStyle
		if i == m.cfgFocus {
			box = configFocusStyle
		}
		b.WriteString(labelStyle.Render(labels[i]))
		b.WriteString("\n")
		b.WriteString(box.Render(input.View()))
		b.WriteString("\n\n")
	}
	b.WriteString(helpStyle.Render("tab/↑↓ move · enter next/save · esc cancel"))
	return b.String()
}

func (m Model) confirmView() string {
	if m.pendingConfirm == nil {
		return ""
	}
	e := m.pendingConfirm
	var b strings.Builder
	b.WriteString(warningStyle.Bold(true).Render("Confirm tool execution"))
	b.WriteString("\n\n")
	b.WriteString(toolStyle.Render(fmt.Sprintf("%s(%s)", e.Tool, firstLine(e.Args, 200))))
	b.WriteString("\n\n")
	if block := renderDiffBlock(e.Diff); block != "" {
		b.WriteString(block)
		b.WriteString("\n\n")
	}
	b.WriteString(helpStyle.Render("y approve · n decline"))
	return confirmBoxStyle.Render(b.String())
}
