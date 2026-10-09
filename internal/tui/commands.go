package tui

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"kterminal/internal/kspec"
)

const kspecCommandPrefix = "/kspec-"
const maxCommandSuggestions = 20
const maxHelpLineRunes = 72
const kspecKickoffPrefix = "Begin the "
const kspecKickoffSuffix = " workflow now."

var nativeCommands = []string{"/clear", "/config", "/exit", "/help", "/image", "/mode", "/model", "/quit", "/unimage"}

var modeOptions = []string{"sdd", "squad"}

const modeCommand = "/mode"

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
		m.blocks = append(m.blocks, errorBoxStyle.Render(err.Error()))
	} else {
		m.blocks = append(m.blocks, primaryStyle.Render("mode: "+mode))
	}
	m.refreshContent()
	return m, nil
}

func (m Model) modePopupView() string {
	if !m.modeOpen {
		return ""
	}
	var b strings.Builder
	b.WriteString(labelStyle.Render("mode"))
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

func (m Model) handleKspecCommand(cmdline string) (tea.Model, tea.Cmd) {
	cmd := strings.Fields(cmdline)[0]
	if cmd == "/kspec-version" {
		return m.showKspecVersion()
	}
	if m.kspecStore == nil {
		m.blocks = append(m.blocks, errorBoxStyle.Render("kspec is not available in this session"))
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
		m.blocks = append(m.blocks, errorBoxStyle.Render("unknown skill "+name+" — /help"))
		m.refreshContent()
		return m, nil
	}
	if err := m.agent.ActivateSkill(name); err != nil {
		m.blocks = append(m.blocks, errorBoxStyle.Render(err.Error()))
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
		m.blocks = append(m.blocks, errorBoxStyle.Render("kspec is not available in this session"))
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
