package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"kterminal/internal/agent"
	"kterminal/internal/catalog"
	"kterminal/internal/clipboard"
	"kterminal/internal/config"
	"kterminal/internal/jev"
	"kterminal/internal/llm"
	"kterminal/internal/router"
)

const stateChat = "chat"
const stateConfig = "config"
const stateConfirm = "confirm"

var thinkingFrames = spinner.Spinner{
	Frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
	FPS:    time.Second / 12,
}

type agentEventMsg struct{ event agent.Event }
type agentClosedMsg struct{}

type Model struct {
	agent   *agent.Agent
	cfg     *config.Config
	catalog *catalog.Catalog

	state  string
	vp     viewport.Model
	input  textinput.Model
	spin   spinner.Model
	width  int
	height int
	cwd    string

	blocks  []string
	stream  *strings.Builder
	mdWidth int

	currentModel string
	routerInfo   string
	sessionCost  float64
	lastTPS      float64
	busy         bool

	cfgInputs []textinput.Model
	cfgFocus  int

	pendingConfirm *agent.Event

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

func New(ag *agent.Agent, cfg *config.Config, cat *catalog.Catalog, dark bool) Model {
	input := textinput.New()
	input.Prompt = ""
	input.CharLimit = 0
	input.Cursor.SetMode(cursor.CursorStatic)
	input.Cursor.Style = lipgloss.NewStyle().Background(colBg).Foreground(colText)
	input.TextStyle = lipgloss.NewStyle().Background(colBgElement)
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
	}
	m.cfgInputs = newConfigInputs(cfg)
	return m
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
			m.input.Width = m.width - 7
		}
		vpHeight := msg.Height - 6
		if vpHeight < 3 {
			vpHeight = 3
		}
		m.vp = viewport.New(msg.Width, vpHeight)
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
		m.input, cmd = m.input.Update(msg)
	}
	return m, cmd
}

func (m Model) handleAgentEvent(e agent.Event) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{listenAgent(m.agent.Events)}
	switch e.Kind {
	case agent.EventRoute:
		m.currentModel = e.Model
		m.routerInfo = fmt.Sprintf("%s %.2f", e.Router, e.Confidence)
		line := routeStyle.Render(fmt.Sprintf("⚡ %s · %s", e.Model, m.routerInfo))
		m.blocks = append(m.blocks, line)
		m.refreshContent()
	case agent.EventDelta:
		m.stream.WriteString(e.Text)
		m.currentModel = e.Model
		m.refreshContent()
	case agent.EventToolStart:
		line := toolStyle.Render(fmt.Sprintf("● %s(%s)", e.Tool, firstLine(e.Args, 100)))
		m.blocks = append(m.blocks, line)
		m.stream.Reset()
		m.refreshContent()
	case agent.EventToolResult:
		line := resultStyle.Render("  " + truncate(strings.ReplaceAll(e.Result, "\n", " ⏎ "), 160))
		m.blocks = append(m.blocks, line)
		m.refreshContent()
	case agent.EventConfirm:
		ev := e
		m.pendingConfirm = &ev
		m.state = stateConfirm
	case agent.EventTurnDone:
		turnCost := e.SessionCost - m.sessionCost
		m.sessionCost = e.SessionCost
		m.lastTPS = e.TPS
		if md := renderMarkdown(m.stream.String(), m.mdWidth, m.dark); md != "" {
			m.blocks = append(m.blocks, md)
		}
		meta := metaMarkStyle.Render("▣ ") + metaStyle.Render(turnMeta(e, turnCost, m.routerInfo))
		m.blocks = append(m.blocks, meta)
		m.stream.Reset()
		m.busy = false
		m.refreshContent()
	case agent.EventError:
		m.blocks = append(m.blocks, errorBoxStyle.Render(e.Text))
		m.stream.Reset()
		m.busy = false
		m.refreshContent()
	}
	return m, tea.Batch(cmds...)
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

func (m Model) handleChatKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.copyFlash = ""
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
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
		m.blocks = append(m.blocks, userBoxStyle.Render(value))
		m.busy = true
		m.agent.Run(value)
		m.refreshContent()
		return m, m.spin.Tick
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) handleCommand(cmdline string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(cmdline)
	cmd := parts[0]
	switch cmd {
	case "/quit", "/exit":
		return m, tea.Quit
	case "/help":
		m.blocks = append(m.blocks, helpStyle.Render(strings.Join([]string{
			"/config — configure gateway and API keys",
			"/model <name> — pin a model · /model auto — let Jev decide",
			"/clear — clear the conversation",
			"/help — this help",
			"/quit — exit",
		}, "\n")))
		m.refreshContent()
		return m, nil
	case "/clear":
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
	default:
		m.blocks = append(m.blocks, errorBoxStyle.Render("unknown command "+cmd+" — /help"))
		m.refreshContent()
		return m, nil
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
	case "n", "N", "esc":
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
	if len(m.blocks) == 0 && m.stream.Len() == 0 {
		b.WriteString(m.emptyState())
	} else {
		b.WriteString(strings.Join(m.blocks, "\n\n"))
		if m.stream.Len() > 0 {
			if len(m.blocks) > 0 {
				b.WriteString("\n\n")
			}
			b.WriteString(m.stream.String())
		}
	}
	m.contentRaw = b.String()
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
		col = cellToRuneCol(m.contentPlain[line], x)
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
	return lipgloss.NewStyle().PaddingLeft(2).Render(b.String())
}

func (m Model) hintBar() string {
	left := ""
	if m.busy {
		left = m.spin.View() + helpStyle.Render(" Thinking")
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
	right += textStyle.Render("?") + helpStyle.Render(" commands")
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

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
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
	}
	prompt := promptBoxStyle.Render(m.input.View())
	fade := fadeCornerStyle.Render("╹") + fadeStyle.Render(strings.Repeat("▀", maxInt(m.width-1, 1)))
	var b strings.Builder
	b.WriteString(m.vp.View())
	b.WriteString("\n\n")
	b.WriteString(prompt)
	b.WriteString("\n")
	b.WriteString(fade)
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
	b.WriteString(helpStyle.Render("y approve · n decline"))
	return confirmBoxStyle.Render(b.String())
}
