package tui

import (
	"regexp"
	"strings"

	"github.com/rivo/uniseg"
)

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;?]*[a-zA-Z]|\x1b\\][^\x1b]*(?:\x1b\\\\|\x07)")

type selPos struct {
	line int
	col  int
}

func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

func plainLines(content string) []string {
	return strings.Split(stripANSI(content), "\n")
}

func (m selPos) before(other selPos) bool {
	if m.line != other.line {
		return m.line < other.line
	}
	return m.col < other.col
}

func clampPos(lines []string, p selPos) selPos {
	if p.line < 0 {
		return selPos{line: 0, col: 0}
	}
	if p.line >= len(lines) {
		last := len(lines) - 1
		if last < 0 {
			last = 0
		}
		return selPos{line: last, col: len([]rune(lines[last]))}
	}
	runes := []rune(lines[p.line])
	if p.col > len(runes) {
		p.col = len(runes)
	}
	return p
}

func cellToRuneCol(line string, x int) int {
	if x <= 0 {
		return 0
	}
	width := 0
	i := 0
	for _, r := range line {
		w := uniseg.StringWidth(string(r))
		if width+w > x {
			return i
		}
		width += w
		i++
	}
	return i
}

func normalizeSel(a, b selPos) (selPos, selPos) {
	if b.before(a) {
		return b, a
	}
	return a, b
}

func selectionText(lines []string, start, end selPos) string {
	start = clampPos(lines, start)
	end = clampPos(lines, end)
	start, end = normalizeSel(start, end)
	var out []string
	for l := start.line; l <= end.line && l < len(lines); l++ {
		runes := []rune(lines[l])
		from := 0
		to := len(runes)
		if l == start.line {
			from = start.col
		}
		if l == end.line {
			to = end.col
		}
		if from > to {
			from = to
		}
		out = append(out, string(runes[from:to]))
	}
	return strings.Join(out, "\n")
}

func applySelection(content string, lines []string, start, end selPos) string {
	start = clampPos(lines, start)
	end = clampPos(lines, end)
	start, end = normalizeSel(start, end)
	raw := strings.Split(content, "\n")
	for l := start.line; l <= end.line && l < len(raw); l++ {
		from, to := 0, 1<<30
		if l == start.line {
			from = start.col
		}
		if l == end.line {
			to = end.col
		}
		raw[l] = highlightRange(raw[l], from, to)
	}
	return strings.Join(raw, "\n")
}

func highlightRange(line string, from, to int) string {
	if from >= to {
		return line
	}
	var b strings.Builder
	visible := 0
	rest := line
	for {
		loc := ansiRe.FindStringIndex(rest)
		if loc == nil {
			break
		}
		visible = emitPlain(&b, rest[:loc[0]], visible, from, to)
		b.WriteString(rest[loc[0]:loc[1]])
		rest = rest[loc[1]:]
	}
	emitPlain(&b, rest, visible, from, to)
	return b.String()
}

func emitPlain(b *strings.Builder, s string, visible, from, to int) int {
	for _, r := range s {
		if visible >= from && visible < to {
			b.WriteString("\x1b[7m")
			b.WriteRune(r)
			b.WriteString("\x1b[27m")
		} else {
			b.WriteRune(r)
		}
		visible++
	}
	return visible
}
