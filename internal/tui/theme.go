package tui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	colInk     = lipgloss.Color("#0d0c0a")
	colBone    = lipgloss.Color("#eae4d8")
	colClay    = lipgloss.Color("#8e8878")
	colAmber   = lipgloss.Color("#f0a83f")
	colJade    = lipgloss.Color("#8ec7ac")
	colRed     = lipgloss.Color("#e2655f")
	colCaution = lipgloss.Color("#e08a4a")
	colGold    = lipgloss.Color("#c9a268")
	colSeam    = lipgloss.Color("#3b372e")
	colSeamHi  = lipgloss.Color("#57503f")

	colSlate  = lipgloss.Color("#8fa8c9")
	colCyan   = lipgloss.Color("#7fbfc4")
	colViolet = lipgloss.Color("#a893c9")
	colRose   = lipgloss.Color("#d98a8a")
)

var leftBorder = lipgloss.Border{
	Left: "┃",
}

var (
	userBoxStyle = lipgloss.NewStyle().
			Border(leftBorder, false, false, false, true).
			BorderForeground(colAmber).
			Padding(1, 1, 1, 2)

	errorBoxStyle = lipgloss.NewStyle().
			Border(leftBorder, false, false, false, true).
			BorderForeground(colRed).
			Padding(1, 1, 1, 2)

	confirmBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colCaution).
			Padding(1, 2)

	askBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colAmber).
			Padding(1, 2)

	promptDeckStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colSeam).
			Padding(0, 1)

	promptDeckBusyStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colAmber).
				Padding(0, 1)

	promptChevronStyle = lipgloss.NewStyle().
				Foreground(colAmber)

	routeStyle = lipgloss.NewStyle().
			Foreground(colJade)

	compactionStyle = lipgloss.NewStyle().
			Foreground(colJade)

	toolStyle = lipgloss.NewStyle().
			Foreground(colJade)

	resultStyle = lipgloss.NewStyle().
			Foreground(colClay)

	nestedStyle = lipgloss.NewStyle().
			Foreground(colJade)

	diffAddedStyle = lipgloss.NewStyle().
			Foreground(colJade)

	diffRemovedStyle = lipgloss.NewStyle().
				Foreground(colRed)

	diffContextStyle = lipgloss.NewStyle().
				Foreground(colClay)

	metaMarkStyle = lipgloss.NewStyle().
			Foreground(colJade)

	abortMarkStyle = lipgloss.NewStyle().
			Foreground(colCaution)

	metaStyle = lipgloss.NewStyle().
			Foreground(colClay)

	helpStyle = lipgloss.NewStyle().
			Foreground(colClay)

	titleStyle = lipgloss.NewStyle().
			Foreground(colBone).
			Bold(true)

	labelStyle = lipgloss.NewStyle().
			Foreground(colClay)

	chromeLabelStyle = lipgloss.NewStyle().
				Foreground(colClay).
				Bold(true)

	primaryStyle = lipgloss.NewStyle().
			Foreground(colAmber)

	textStyle = lipgloss.NewStyle().
			Foreground(colBone)

	warningStyle = lipgloss.NewStyle().
			Foreground(colCaution)

	successStyle = lipgloss.NewStyle().
			Foreground(colJade)

	configBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colSeam).
			Padding(0, 1)

	configFocusStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colAmber).
				Padding(0, 1)

	mentionPopupStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colSeam).
				Padding(0, 1)

	mentionItemStyle = lipgloss.NewStyle().
				Foreground(colClay)

	mentionSelectedStyle = lipgloss.NewStyle().
				Foreground(colAmber)

	chipStyle = lipgloss.NewStyle().
			Foreground(colAmber)

	cwdStyle = lipgloss.NewStyle().
			Foreground(colClay).
			Padding(0, 2)

	sidebarPanelStyle = lipgloss.NewStyle().
				Border(leftBorder, false, false, false, true).
				BorderForeground(colSeam).
				Padding(0, 1)

	sidebarDividerStyle = lipgloss.NewStyle().
				Foreground(colSeam)
)

var disciplineColors = map[string]lipgloss.Color{
	"architecture": colGold,
	"backend":      colSlate,
	"database":     colCyan,
	"frontend":     colAmber,
	"qa":           colJade,
	"test":         colViolet,
	"ux":           colRose,
}

func disciplineColor(discipline string) lipgloss.Color {
	if c, ok := disciplineColors[discipline]; ok {
		return c
	}
	return colClay
}

func agentLabelStyle(discipline string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(disciplineColor(discipline))
}

func renderLogo() string {
	return primaryStyle.Render("◆") + " " + titleStyle.Render("K T E R M I N A L")
}
