package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"kterminal/internal/squad"
)

const (
	sidebarMinTerminalWidth = 100
	sidebarWidthFloor       = 24
	sidebarWidthCeiling     = 40
	sidebarChromeWidth      = 3
	sidebarMarker           = "▸ "
	sidebarIndent           = "  "
	sidebarSeparator        = " · "
	sidebarIdleEntry        = "—"
	sidebarMesaIdleText     = "mesa not started"
	sidebarHeaderTitle      = "squad"
	sidebarMaestroName      = "maestro"
)

func (m Model) sidebarWidth() int {
	if m.agent.Mode() != "squad" || m.width < sidebarMinTerminalWidth {
		return 0
	}
	w := sidebarWidthFloor + (m.width-sidebarMinTerminalWidth)*11/40
	return min(max(w, sidebarWidthFloor), sidebarWidthCeiling)
}

func (m Model) sidebarView(width int) string {
	if width < sidebarWidthFloor {
		return ""
	}
	contentWidth := width - sidebarChromeWidth
	var b strings.Builder
	writeSidebarLine(&b, titleStyle.Render(sidebarHeaderTitle), contentWidth)
	m.writeSidebarMaestro(&b, contentWidth)
	writeSidebarLine(&b, sidebarDivider(contentWidth), contentWidth)
	mesa := m.agent.Mesa()
	if mesa == nil || len(mesa.Entries) == 0 {
		writeSidebarLine(&b, helpStyle.Render(truncate(sidebarMesaIdleText, contentWidth)), contentWidth)
	} else {
		for _, entry := range mesa.Entries {
			m.writeSidebarEntry(&b, entry, contentWidth)
		}
		writeSidebarLine(&b, sidebarDivider(contentWidth), contentWidth)
		writeSidebarLine(&b, helpStyle.Render(truncate(sidebarBudgetConvocations(mesa), contentWidth)), contentWidth)
		writeSidebarLine(&b, helpStyle.Render(truncate(sidebarBudgetTokens(mesa), contentWidth)), contentWidth)
	}
	return sidebarPanelStyle.Height(m.height).Render(strings.TrimRight(b.String(), "\n"))
}

func (m Model) writeSidebarMaestro(b *strings.Builder, width int) {
	status := squad.StatusWaiting
	if m.busy {
		status = squad.StatusDeliberating
	}
	writeSidebarLine(b, sidebarHeaderLine(primaryStyle, sidebarMaestroName, status, width), width)
	model := m.currentModel
	if model == "" {
		model = sidebarIdleEntry
	}
	writeSidebarLine(b, helpStyle.Render(truncate(sidebarIndent+model, width)), width)
}

func (m Model) writeSidebarEntry(b *strings.Builder, entry squad.MesaEntry, width int) {
	status := entry.Status
	if status == "" {
		status = squad.StatusWaiting
	}
	writeSidebarLine(b, sidebarHeaderLine(agentLabelStyle(entry.Discipline), entry.Name, status, width), width)
	writeSidebarLine(b, helpStyle.Render(truncate(sidebarIndent+sidebarMetricsText(entry), width)), width)
	writeSidebarLine(b, m.sidebarActivityLine(entry, width), width)
}

func (m Model) sidebarActivityLine(entry squad.MesaEntry, width int) string {
	activity := m.personaActivity[entry.Name]
	if activity == "" {
		return helpStyle.Render(truncate(sidebarIndent+sidebarIdleEntry, width))
	}
	return toolStyle.Render(truncate(sidebarIndent+activity, width))
}

func sidebarHeaderLine(style lipgloss.Style, name, status string, width int) string {
	nameBudget := width - len([]rune(sidebarMarker)) - len([]rune(sidebarSeparator)) - len([]rune(status))
	var b strings.Builder
	b.WriteString(style.Render(sidebarMarker + truncate(name, max(nameBudget, 1))))
	b.WriteString(sidebarStatusStyle(status).Render(sidebarSeparator + status))
	return b.String()
}

func sidebarStatusStyle(status string) lipgloss.Style {
	switch status {
	case squad.StatusDeliberating:
		return warningStyle
	case squad.StatusDone:
		return successStyle
	}
	return helpStyle
}

func sidebarMetricsText(entry squad.MesaEntry) string {
	if entry.Model == "" && entry.Tokens <= 0 && entry.Cost <= 0 {
		return sidebarIdleEntry
	}
	model := entry.Model
	if model == "" {
		model = sidebarIdleEntry
	}
	return fmt.Sprintf("%s · %s tok · %s", model, formatTokensK(entry.Tokens), formatCost(entry.Cost))
}

func sidebarBudgetConvocations(mesa *squad.Mesa) string {
	if mesa.MaxConvocations > 0 {
		return fmt.Sprintf("%d/%d convocations", mesa.Convocations, mesa.MaxConvocations)
	}
	return fmt.Sprintf("%d convocations", mesa.Convocations)
}

func sidebarBudgetTokens(mesa *squad.Mesa) string {
	if mesa.TokenBudget > 0 {
		return fmt.Sprintf("%s/%s tokens", formatTokensK(mesa.Tokens), formatTokensK(mesa.TokenBudget))
	}
	return fmt.Sprintf("%s tokens", formatTokensK(mesa.Tokens))
}

func sidebarDivider(width int) string {
	return sidebarDividerStyle.Render(strings.Repeat("─", width))
}

func writeSidebarLine(b *strings.Builder, line string, width int) {
	b.WriteString(padSidebarLine(line, width))
	b.WriteString("\n")
}

func padSidebarLine(line string, width int) string {
	gap := width - lipgloss.Width(line)
	if gap <= 0 {
		return line
	}
	return line + strings.Repeat(" ", gap)
}
