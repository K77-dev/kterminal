package tui

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/x/ansi"
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
