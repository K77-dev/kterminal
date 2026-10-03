package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	colPrimary      = lipgloss.Color("#fab283")
	colSecondary    = lipgloss.Color("#5c9cf5")
	colAccent       = lipgloss.Color("#9d7cd8")
	colError        = lipgloss.Color("#e06c75")
	colWarning      = lipgloss.Color("#f5a742")
	colSuccess      = lipgloss.Color("#7fd88f")
	colInfo         = lipgloss.Color("#56b6c2")
	colYellow       = lipgloss.Color("#e5c07b")
	colText         = lipgloss.Color("#eeeeee")
	colTextMuted    = lipgloss.Color("#808080")
	colBg           = lipgloss.Color("#0a0a0a")
	colBgPanel      = lipgloss.Color("#141414")
	colBgElement    = lipgloss.Color("#1e1e1e")
	colBorder       = lipgloss.Color("#484848")
	colBorderActive = lipgloss.Color("#606060")
	colShadow       = lipgloss.Color("#3c3c3c")
)

var leftBorder = lipgloss.Border{
	Left: "┃",
}

var (
	userBoxStyle = lipgloss.NewStyle().
			Border(leftBorder, false, false, false, true).
			BorderForeground(colPrimary).
			Background(colBgPanel).
			Padding(1, 1, 1, 2)

	errorBoxStyle = lipgloss.NewStyle().
			Border(leftBorder, false, false, false, true).
			BorderForeground(colError).
			Background(colBgPanel).
			Padding(1, 1, 1, 2)

	confirmBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colWarning).
			Background(colBgPanel).
			Padding(1, 2)

	promptBoxStyle = lipgloss.NewStyle().
			Border(leftBorder, false, false, false, true).
			BorderForeground(colBorderActive).
			Background(colBgElement).
			Padding(1, 2)

	routeStyle = lipgloss.NewStyle().
			Foreground(colTextMuted)

	toolStyle = lipgloss.NewStyle().
			Foreground(colInfo)

	resultStyle = lipgloss.NewStyle().
			Foreground(colTextMuted)

	metaMarkStyle = lipgloss.NewStyle().
			Foreground(colAccent)

	metaStyle = lipgloss.NewStyle().
			Foreground(colTextMuted)

	helpStyle = lipgloss.NewStyle().
			Foreground(colTextMuted)

	titleStyle = lipgloss.NewStyle().
			Foreground(colAccent).
			Bold(true)

	labelStyle = lipgloss.NewStyle().
			Foreground(colTextMuted)

	fadeStyle = lipgloss.NewStyle().
			Foreground(colBgElement)

	fadeCornerStyle = lipgloss.NewStyle().
			Foreground(colBorderActive)

	primaryStyle = lipgloss.NewStyle().
			Foreground(colPrimary)

	textStyle = lipgloss.NewStyle().
			Foreground(colText)

	warningStyle = lipgloss.NewStyle().
			Foreground(colWarning)

	successStyle = lipgloss.NewStyle().
			Foreground(colSuccess)

	configBoxStyle = lipgloss.NewStyle().
			Border(leftBorder, false, false, false, true).
			BorderForeground(colBorderActive).
			Background(colBgElement).
			Padding(0, 2)

	configFocusStyle = lipgloss.NewStyle().
				Border(leftBorder, false, false, false, true).
				BorderForeground(colPrimary).
				Background(colBgElement).
				Padding(0, 2)
)

var logoLeft = []string{
	"                        ",
	"█_▀█ ████ █▀▀█ █▀▀█ █▄▄█",
	"███_ _█__ █^^^ █▀▀▄ ████",
	"█_▄█ _█__ ▀▀▀▀ █_▄█ █__█",
}

var logoRight = []string{
	"                   ",
	"_██_ █▀▀▄ ▄██▄ █___",
	"_██_ █__█ █▄▄█ █___",
	"_██_ ▀~~▀ █__█ ████",
}

func renderLogoLine(line string, fg lipgloss.Color, bold bool) string {
	var b strings.Builder
	for _, ch := range line {
		switch ch {
		case '_':
			b.WriteString(lipgloss.NewStyle().Background(colShadow).Render(" "))
		case '^', '~':
			b.WriteString(lipgloss.NewStyle().Foreground(colShadow).Render("▀"))
		case ',':
			b.WriteString(lipgloss.NewStyle().Foreground(colShadow).Render("▄"))
		case ' ':
			b.WriteString(" ")
		default:
			b.WriteString(lipgloss.NewStyle().Foreground(fg).Bold(bold).Render(string(ch)))
		}
	}
	return b.String()
}

func renderLogo() string {
	var b strings.Builder
	for i, left := range logoLeft {
		b.WriteString(renderLogoLine(left, colTextMuted, false))
		b.WriteString(" ")
		b.WriteString(renderLogoLine(logoRight[i], colText, true))
		if i < len(logoLeft)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}
