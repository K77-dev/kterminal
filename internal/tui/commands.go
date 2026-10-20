package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"kterminal/internal/agent"
	"kterminal/internal/kspec"
	"kterminal/internal/procmon"
	"kterminal/internal/router"
)

const kspecCommandPrefix = "/kspec-"
const maxCommandSuggestions = 24
const maxHelpLineRunes = 72
const kspecKickoffPrefix = "Begin the "
const kspecKickoffSuffix = " workflow now."

var nativeCommands = []string{"/bg", "/clear", "/config", "/exit", "/help", "/image", "/mode", "/model", "/quit", "/router", "/rtk", "/unimage"}

var modeOptions = []string{"sdd", "squad"}

const modeCommand = "/mode"

// togglePermission switches between plan and build (opencode-style Tab /
// Alt+Tab). In squad mode it is inert and shows a hint instead.
func (m Model) togglePermission() (tea.Model, tea.Cmd) {
	next, err := m.agent.TogglePermission()
	if err != nil {
		m.blocks = append(m.blocks, warningStyle.Render(err.Error()))
		m.refreshContent()
		return m, nil
	}
	m.blocks = append(m.blocks, permissionBlock(next))
	m.refreshContent()
	return m, nil
}

func permissionBlock(permission string) string {
	switch permission {
	case agent.PermissionPlan:
		return planModeStyle.Render("◐ plan mode — research and plan only; tab to switch to build")
	default:
		return buildModeStyle.Render("◆ build mode — full toolset; tab to switch to plan")
	}
}

func (m *Model) closeCommandPopup() {
	m.cmdOpen = false
	m.cmdItems = nil
	m.cmdSelected = 0
}

func (m *Model) refreshSuggestions() {
	m.refreshMentionSuggestions()
	m.refreshCommandSuggestions()
}

func (m *Model) refreshCommandSuggestions() {
	wasOpen := m.cmdOpen
	m.closeCommandPopup()
	value := m.input.Value()
	if !strings.HasPrefix(value, "/") || strings.ContainsAny(value, " \t\n") {
		m.cmdSkillCache = nil
		return
	}
	if !wasOpen {
		m.cmdSkillCache = skillCommands(m.kspecStore)
	}
	items := commandSuggestions(value, m.cmdSkillCache)
	if len(items) == 0 {
		return
	}
	m.cmdItems = items
	m.cmdOpen = true
}

func skillCommands(store *kspec.Store) []string {
	if store == nil {
		return nil
	}
	skills := store.List()
	commands := make([]string, 0, len(skills))
	for _, sk := range skills {
		commands = append(commands, "/"+sk.Name)
	}
	sort.Strings(commands)
	return commands
}

func commandSuggestions(prefix string, skills []string) []string {
	items := prefixMatches(nativeCommands, prefix)
	for _, cmd := range skills {
		if len(items) == maxCommandSuggestions {
			break
		}
		if strings.HasPrefix(cmd, prefix) {
			items = append(items, cmd)
		}
	}
	sort.Strings(items)
	return items
}

func prefixMatches(commands []string, prefix string) []string {
	var items []string
	for _, cmd := range commands {
		if strings.HasPrefix(cmd, prefix) {
			items = append(items, cmd)
		}
	}
	return items
}

func (m *Model) completeCommand() {
	if !m.cmdOpen || len(m.cmdItems) == 0 {
		return
	}
	value := m.cmdItems[m.cmdSelected] + " "
	m.input.SetValue(value)
	m.setPromptCursor(value, len(value))
	m.histIdx = -1
	m.closeCommandPopup()
}

func (m Model) commandPopupView() string {
	if !m.cmdOpen || len(m.cmdItems) == 0 {
		return ""
	}
	var b strings.Builder
	for i, item := range m.cmdItems {
		if i > 0 {
			b.WriteString("\n")
		}
		if i == m.cmdSelected {
			b.WriteString(mentionSelectedStyle.Render("▸ " + item))
		} else {
			b.WriteString(mentionItemStyle.Render("  " + item))
		}
	}
	return mentionPopupStyle.Render(b.String())
}

func (m *Model) closeModePopup() {
	m.modeOpen = false
	m.modeSelected = 0
}

func (m *Model) openModePopup() {
	m.closeCommandPopup()
	m.modeOpen = true
	m.modeSelected = 0
	for i, opt := range modeOptions {
		if opt == m.agent.Mode() {
			m.modeSelected = i
		}
	}
}

func (m Model) selectMode() (tea.Model, tea.Cmd) {
	if !m.modeOpen || len(modeOptions) == 0 {
		return m, nil
	}
	mode := modeOptions[m.modeSelected]
	m.closeModePopup()
	if err := m.agent.ActivateMode(mode); err != nil {
		m.blocks = append(m.blocks, errorBox(err.Error()))
	} else {
		m.blocks = append(m.blocks, primaryStyle.Render("mode: "+mode))
	}
	m.fitChatColumn()
	m.refreshContent()
	return m, nil
}

func (m Model) modePopupView() string {
	if !m.modeOpen {
		return ""
	}
	var b strings.Builder
	b.WriteString(chromeLabelStyle.Render("MODE"))
	b.WriteString("\n")
	for i, item := range modeOptions {
		if i > 0 {
			b.WriteString("\n")
		}
		if i == m.modeSelected {
			b.WriteString(mentionSelectedStyle.Render("▸ " + item))
		} else {
			b.WriteString(mentionItemStyle.Render("  " + item))
		}
	}
	return mentionPopupStyle.Render(b.String())
}

func (m *Model) closeBgPopup() {
	m.bgOpen = false
	m.bgSelected = 0
}

func (m *Model) openBgPopup() {
	m.closeCommandPopup()
	m.bgOpen = true
	m.bgSelected = 0
}

func (m Model) handleBgCommand() (tea.Model, tea.Cmd) {
	if m.bgCount() == 0 {
		m.blocks = append(m.blocks, resultStyle.Render("no background processes running"))
		m.refreshContent()
		return m, nil
	}
	m.openBgPopup()
	return m, nil
}

func (m Model) bgEntries() []procmon.Entry {
	if m.procMon == nil {
		return nil
	}
	return m.procMon.List()
}

func (m *Model) clampBgSelection() {
	if n := m.bgCount(); n > 0 {
		if m.bgSelected >= n {
			m.bgSelected = n - 1
		}
	} else {
		m.bgSelected = 0
	}
}

func (m Model) selectBg() (tea.Model, tea.Cmd) {
	if !m.bgOpen || m.procMon == nil {
		return m, nil
	}
	entries := m.procMon.List()
	if m.bgSelected >= len(entries) {
		return m, nil
	}
	m.procMon.Stop(entries[m.bgSelected].ID)
	m.clampBgSelection()
	return m, nil
}

func (m Model) bgPopupView() string {
	if !m.bgOpen {
		return ""
	}
	var b strings.Builder
	b.WriteString(chromeLabelStyle.Render("BACKGROUND"))
	b.WriteString("\n")
	entries := m.bgEntries()
	if len(entries) == 0 {
		b.WriteString(mentionItemStyle.Render("  no background processes running"))
	} else {
		budget := maxInt(m.chatWidth()-2*chatInset-24, 12)
		for i, e := range entries {
			if i > 0 {
				b.WriteString("\n")
			}
			line := fmt.Sprintf("#%d · %s · %s", e.ID, truncate(e.Command, budget), time.Since(e.Started).Round(time.Second))
			if i == m.bgSelected {
				b.WriteString(mentionSelectedStyle.Render("▸ " + line))
			} else {
				b.WriteString(mentionItemStyle.Render("  " + line))
			}
		}
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑↓ move · enter stop · esc close"))
	return mentionPopupStyle.Render(b.String())
}

type modelChoice struct {
	model string
	label string
}

func (m Model) modelChoices() []modelChoice {
	choices := make([]modelChoice, 0, len(m.catalog.Models)+1)
	choices = append(choices, modelChoice{label: "auto"})
	for _, mm := range m.catalog.Models {
		tps := mm.TPSEstimate
		if mm.MeasuredTPS > 0 {
			tps = mm.MeasuredTPS
		}
		choices = append(choices, modelChoice{
			model: mm.Name,
			label: fmt.Sprintf("%s · $%.2f/$%.2f · ~%.0f tok/s", mm.Name, mm.InputPricePerM, mm.OutputPricePerM, tps),
		})
	}
	return choices
}

func (m *Model) closeModelPopup() {
	m.modelOpen = false
	m.modelSelected = 0
}

func (m *Model) openModelPopup() {
	m.closeCommandPopup()
	m.modelOpen = true
	m.modelSelected = 0
	if pin := m.agent.Pinned(); pin != "" {
		for i, choice := range m.modelChoices() {
			if choice.model == pin {
				m.modelSelected = i
			}
		}
	}
}

func (m Model) handleModelCommand(arg string) (tea.Model, tea.Cmd) {
	if arg == "" {
		m.openModelPopup()
		m.refreshContent()
		return m, nil
	}
	if arg == "auto" {
		return m.applyModelChoice("")
	}
	if _, ok := m.catalog.Get(arg); !ok {
		m.blocks = append(m.blocks, errorBox("unknown model "+arg+" — not in catalog"))
		m.refreshContent()
		return m, nil
	}
	return m.applyModelChoice(arg)
}

func (m Model) applyModelChoice(name string) (tea.Model, tea.Cmd) {
	previous := m.cfg.Model
	m.cfg.Model = name
	if err := m.cfg.Save(); err != nil {
		m.cfg.Model = previous
		m.blocks = append(m.blocks, errorBox("failed to save config: "+err.Error()))
		m.refreshContent()
		return m, nil
	}
	m.agent.SetPinned(name)
	m.rebuildAgent()
	if name == "" {
		m.blocks = append(m.blocks, resultStyle.Render("model auto — Jev decides again"))
	} else {
		m.blocks = append(m.blocks, primaryStyle.Render("model pinned: "+name))
	}
	m.blocks = append(m.blocks, resultStyle.Render("config saved"))
	m.refreshContent()
	return m, nil
}

func (m Model) selectModel() (tea.Model, tea.Cmd) {
	if !m.modelOpen {
		return m, nil
	}
	choices := m.modelChoices()
	if m.modelSelected >= len(choices) {
		return m, nil
	}
	name := choices[m.modelSelected].model
	m.closeModelPopup()
	return m.applyModelChoice(name)
}

func (m Model) modelPopupView() string {
	if !m.modelOpen {
		return ""
	}
	var b strings.Builder
	b.WriteString(chromeLabelStyle.Render("MODEL"))
	b.WriteString("\n")
	for i, choice := range m.modelChoices() {
		if i > 0 {
			b.WriteString("\n")
		}
		if i == m.modelSelected {
			b.WriteString(mentionSelectedStyle.Render("▸ " + choice.label))
		} else {
			b.WriteString(mentionItemStyle.Render("  " + choice.label))
		}
	}
	return mentionPopupStyle.Render(b.String())
}

func (m Model) handleRtkCommand(arg string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(arg)
	switch {
	case len(parts) == 0 || (len(parts) == 1 && parts[0] == "status"):
		return m.showRtkStatus()
	case len(parts) == 1 && parts[0] == "on":
		return m.applyRtkState(true)
	case len(parts) == 1 && parts[0] == "off":
		return m.applyRtkState(false)
	default:
		m.blocks = append(m.blocks, errorBox("unknown rtk command "+strings.Join(parts, " ")+" — /rtk [status|on|off]"))
		m.refreshContent()
		return m, nil
	}
}

func (m Model) showRtkStatus() (tea.Model, tea.Cmd) {
	enabled := "no"
	if m.cfg.RTK.EnabledVal() {
		enabled = "yes"
	}
	m.blocks = append(m.blocks, resultStyle.Render("rtk: "+enabled))
	m.refreshContent()
	return m, nil
}

func (m Model) applyRtkState(enabled bool) (tea.Model, tea.Cmd) {
	previous := m.cfg.RTK.Enabled
	m.cfg.RTK.Enabled = &enabled
	if err := m.cfg.Save(); err != nil {
		m.cfg.RTK.Enabled = previous
		m.blocks = append(m.blocks, errorBox("failed to save config: "+err.Error()))
		m.refreshContent()
		return m, nil
	}
	label := "rtk off"
	if enabled {
		label = "rtk on"
	}
	m.blocks = append(m.blocks, primaryStyle.Render(label))
	m.blocks = append(m.blocks, resultStyle.Render("config saved — restart kterminal to apply"))
	m.refreshContent()
	return m, nil
}

func (m Model) handleRouterCommand(arg string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(arg)
	switch {
	case len(parts) == 0 || (len(parts) == 1 && parts[0] == "status"):
		return m.showRouterStatus()
	case len(parts) == 1 && parts[0] == "jev":
		return m.switchRouterEngine(router.EngineJev)
	case len(parts) == 1 && parts[0] == "laya":
		return m.switchRouterEngine(router.EngineLaya)
	case len(parts) == 1 && parts[0] == "on":
		return m.enableRouter()
	case len(parts) == 1 && parts[0] == "off":
		return m.applyRouterState(false)
	default:
		m.blocks = append(m.blocks, errorBox("unknown router command "+strings.Join(parts, " ")+" — /router [status|jev|laya|on|off]"))
		m.refreshContent()
		return m, nil
	}
}

func (m Model) showRouterStatus() (tea.Model, tea.Cmd) {
	engine := m.cfg.Router.Engine
	if engine == "" {
		engine = router.EngineJev
	}
	state := "off"
	if m.cfg.Router.RoutingEnabled() && m.routerEngineReady(engine) {
		state = "on"
	}
	model := m.cfg.Model
	if model == "" {
		model = m.catalog.DefaultModel
	}
	m.blocks = append(m.blocks, resultStyle.Render("engine: "+engine+"\nrouter: "+state+"\nmodel: "+model))
	m.refreshContent()
	return m, nil
}

func (m Model) routerEngineReady(engine string) bool {
	switch engine {
	case router.EngineLaya:
		return m.laya != nil
	default:
		return m.cfg.Typesafe.APIKey != ""
	}
}

func (m Model) switchRouterEngine(engine string) (tea.Model, tea.Cmd) {
	if engine == router.EngineLaya && m.laya == nil {
		m.blocks = append(m.blocks, errorBox("laya engine ainda não disponível"))
		m.refreshContent()
		return m, nil
	}
	if engine == router.EngineJev && m.cfg.Typesafe.APIKey == "" {
		m.blocks = append(m.blocks, errorBox("jev needs a typesafe api key — set one in /config"))
		m.refreshContent()
		return m, nil
	}
	previousEngine := m.cfg.Router.Engine
	previousEnabled := m.cfg.Router.Enabled
	enabled := true
	m.cfg.Router.Engine = engine
	m.cfg.Router.Enabled = &enabled
	if err := m.cfg.Save(); err != nil {
		m.cfg.Router.Engine = previousEngine
		m.cfg.Router.Enabled = previousEnabled
		m.blocks = append(m.blocks, errorBox("failed to save config: "+err.Error()))
		m.refreshContent()
		return m, nil
	}
	m.rebuildAgent()
	m.blocks = append(m.blocks, primaryStyle.Render("engine "+engine+" · router on"))
	m.blocks = append(m.blocks, resultStyle.Render("config saved"))
	m.refreshContent()
	return m, nil
}

func (m Model) enableRouter() (tea.Model, tea.Cmd) {
	engine := m.cfg.Router.Engine
	if engine == "" {
		engine = router.EngineJev
	}
	if !m.routerEngineReady(engine) {
		switch engine {
		case router.EngineLaya:
			m.blocks = append(m.blocks, errorBox("laya engine ainda não disponível"))
		default:
			m.blocks = append(m.blocks, errorBox("jev needs a typesafe api key — set one in /config"))
		}
		m.refreshContent()
		return m, nil
	}
	return m.applyRouterState(true)
}

func (m Model) applyRouterState(enabled bool) (tea.Model, tea.Cmd) {
	previous := m.cfg.Router.Enabled
	m.cfg.Router.Enabled = &enabled
	if err := m.cfg.Save(); err != nil {
		m.cfg.Router.Enabled = previous
		m.blocks = append(m.blocks, errorBox("failed to save config: "+err.Error()))
		m.refreshContent()
		return m, nil
	}
	m.rebuildAgent()
	engine := m.cfg.Router.Engine
	if engine == "" {
		engine = router.EngineJev
	}
	label := engine + " off"
	if enabled {
		label = engine + " on"
	}
	m.blocks = append(m.blocks, primaryStyle.Render(label))
	m.blocks = append(m.blocks, resultStyle.Render("config saved"))
	if !enabled {
		m.openModelPopup()
	}
	m.refreshContent()
	return m, nil
}

func (m Model) handleKspecCommand(cmdline string) (tea.Model, tea.Cmd) {
	cmd := strings.Fields(cmdline)[0]
	if cmd == "/kspec-version" {
		return m.showKspecVersion()
	}
	if m.kspecStore == nil {
		m.blocks = append(m.blocks, errorBox("kspec is not available in this session"))
		m.refreshContent()
		return m, nil
	}
	if m.busy {
		m.blocks = append(m.blocks, warningStyle.Render("busy — wait for the current task to finish"))
		m.refreshContent()
		return m, nil
	}
	name := strings.TrimPrefix(cmd, "/")
	if !kspecSkillListed(m.kspecStore, name) {
		m.blocks = append(m.blocks, errorBox("unknown skill "+name+" — /help"))
		m.refreshContent()
		return m, nil
	}
	if err := m.agent.ActivateSkill(name); err != nil {
		m.blocks = append(m.blocks, errorBox(err.Error()))
		m.refreshContent()
		return m, nil
	}
	kickoff := strings.TrimSpace(strings.TrimPrefix(cmdline, cmd))
	if kickoff == "" {
		kickoff = kspecKickoffPrefix + name + kspecKickoffSuffix
	}
	m.blocks = append(m.blocks, userBlock(cmdline, m.pendingAttachments))
	m.pushPromptHistory(cmdline)
	m.busy = true
	m.sentAttachments = nil
	m.turnConsumed = false
	if len(m.pendingAttachments) == 0 {
		m.agent.Run(kickoff)
	} else {
		m.agent.RunWithAttachments(kickoff, attachmentParts(kickoff, m.pendingAttachments), attachmentMeta(m.pendingAttachments))
		m.sentAttachments = m.pendingAttachments
		m.pendingAttachments = nil
		m.fitViewportHeight()
	}
	m.refreshContent()
	return m, m.spin.Tick
}

func kspecSkillListed(store *kspec.Store, name string) bool {
	for _, sk := range store.List() {
		if sk.Name == name {
			return true
		}
	}
	return false
}

func (m Model) showKspecVersion() (tea.Model, tea.Cmd) {
	if m.kspecStore == nil {
		m.blocks = append(m.blocks, errorBox("kspec is not available in this session"))
		m.refreshContent()
		return m, nil
	}
	m.blocks = append(m.blocks, resultStyle.Render(kspecVersionLine(m.kspecStore)))
	m.refreshContent()
	return m, nil
}

func kspecVersionLine(store *kspec.Store) string {
	source := store.Source()
	if v := store.Version(); v != "" {
		return "kspec v" + v + " (" + source + ")"
	}
	return "kspec version unknown (" + source + ")"
}

func kspecHelpLines(store *kspec.Store) []string {
	if store == nil {
		return nil
	}
	skills := store.List()
	if len(skills) == 0 {
		return nil
	}
	lines := make([]string, 0, len(skills)+1)
	lines = append(lines, "kspec skills:")
	for _, sk := range skills {
		lines = append(lines, kspecHelpLine(sk))
	}
	return lines
}

func kspecHelpLine(sk kspec.Skill) string {
	line := "/" + sk.Name
	if sk.ArgHint != "" {
		line += " " + sk.ArgHint
	}
	return truncate(line+" — "+sk.Description, maxHelpLineRunes)
}
