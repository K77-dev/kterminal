package tui

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

//go:embed markdown.json
var markdownStyle []byte

var (
	mdRenderer *glamour.TermRenderer
	mdDark     bool
	mdWidth    int
)

func renderMarkdown(md string, width int, dark bool) string {
	if strings.TrimSpace(md) == "" {
		return ""
	}
	if width <= 0 {
		width = 80
	}
	if mdRenderer == nil || mdDark != dark || mdWidth != width {
		opts := []glamour.TermRendererOption{
			glamour.WithStylesFromJSONBytes(markdownStyle),
			glamour.WithWordWrap(width),
		}
		if lipgloss.ColorProfile() == termenv.TrueColor {
			opts = append(opts, glamour.WithChromaFormatter("terminal16m"))
		}
		if !dark {
			opts = append(opts, glamour.WithStandardStyle("light"))
		}
		r, err := glamour.NewTermRenderer(opts...)
		if err != nil {
			return md
		}
		mdRenderer = r
		mdDark = dark
		mdWidth = width
	}
	out, err := mdRenderer.Render(md)
	if err != nil {
		return md
	}
	return strings.TrimRight(out, "\n")
}

func wrapContent(s string, width int) string {
	if width < 1 || strings.TrimSpace(s) == "" {
		return s
	}
	return ansi.Wrap(s, width, "/\\_.,:;)")
}

func indentLines(s string, n int) string {
	if s == "" || n <= 0 {
		return s
	}
	pad := strings.Repeat(" ", n)
	return pad + strings.ReplaceAll(s, "\n", "\n"+pad)
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n <= 0 {
		return ""
	}
	return string(runes[:n-1]) + "…"
}

func firstLine(s string, n int) string {
	line := s
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		line = s[:i]
	}
	return truncate(strings.TrimSpace(line), n)
}

func formatCost(c float64) string {
	return fmt.Sprintf("$%.4f", c)
}

func backgroundSequence(c lipgloss.Color) string {
	hex := string(c)
	if len(hex) != 7 || hex[0] != '#' {
		return ""
	}
	r, err1 := strconv.ParseUint(hex[1:3], 16, 8)
	g, err2 := strconv.ParseUint(hex[3:5], 16, 8)
	b, err3 := strconv.ParseUint(hex[5:7], 16, 8)
	if err1 != nil || err2 != nil || err3 != nil {
		return ""
	}
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b)
}

func clearsBackground(seq string) bool {
	i := strings.IndexByte(seq, '[')
	j := strings.IndexByte(seq, 'm')
	if i < 0 || j <= i {
		return false
	}
	params := strings.Split(seq[i+1:j], ";")
	if len(params) == 1 && params[0] == "" {
		return true
	}
	for k := 0; k < len(params); k++ {
		switch params[k] {
		case "", "0", "49":
			return true
		case "38", "48":
			if k+1 < len(params) && (params[k+1] == "2" || params[k+1] == "5") {
				step := 4
				if params[k+1] == "5" {
					step = 2
				}
				k += step
			}
		}
	}
	return false
}

func forceBackground(s string, c lipgloss.Color) string {
	seq := backgroundSequence(c)
	if seq == "" || s == "" {
		return s
	}
	var b strings.Builder
	b.WriteString(seq)
	rest := s
	for {
		nl := strings.IndexByte(rest, '\n')
		loc := ansiRe.FindStringIndex(rest)
		if loc == nil {
			b.WriteString(rest)
			return b.String()
		}
		if nl >= 0 && nl < loc[0] {
			b.WriteString(rest[:nl])
			b.WriteString("\n")
			b.WriteString(seq)
			rest = rest[nl+1:]
			continue
		}
		b.WriteString(rest[:loc[0]])
		found := rest[loc[0]:loc[1]]
		b.WriteString(found)
		if clearsBackground(found) {
			b.WriteString(seq)
		}
		rest = rest[loc[1]:]
	}
}
