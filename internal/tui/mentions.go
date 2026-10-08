package tui

import (
	"os"
	"regexp"
	"strings"
)

const maxMentionFiles = 5
const maxMentionBytes = 64 * 1024
const maxMentionSuggestions = 10

var mentionRe = regexp.MustCompile(`@([^\s@]+)`)

func ExpandMentions(text string) (string, []string) {
	var warnings []string
	var blocks []string
	warned := false
	inFence := false
	for _, line := range strings.Split(text, "\n") {
		if isFenceLine(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		for _, m := range mentionRe.FindAllStringSubmatchIndex(line, -1) {
			if m[0] > 0 && !isSpaceByte(line[m[0]-1]) {
				continue
			}
			path := line[m[2]:m[3]]
			info, err := os.Stat(path)
			if err != nil || info.IsDir() {
				continue
			}
			if len(blocks) >= maxMentionFiles {
				if !warned {
					warnings = append(warnings, "max 5 file mentions")
					warned = true
				}
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			content := string(data)
			if len(data) > maxMentionBytes {
				content = string(data[:maxMentionBytes]) + "\n... (truncated)"
			}
			blocks = append(blocks, "--- Arquivo @"+path+" ---\n"+content+"\n")
		}
	}
	if len(blocks) == 0 {
		return text, warnings
	}
	var b strings.Builder
	b.WriteString(text)
	b.WriteString("\n\n")
	for _, block := range blocks {
		b.WriteString(block)
	}
	return b.String(), warnings
}

func mentionPrefix(input string) (string, bool) {
	idx := strings.LastIndex(input, "@")
	if idx < 0 {
		return "", false
	}
	if idx > 0 && !isSpaceByte(input[idx-1]) {
		return "", false
	}
	rest := input[idx+1:]
	if strings.ContainsAny(rest, " \t\n\f\r") {
		return "", false
	}
	return rest, true
}

func isFenceLine(line string) bool {
	return strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "```")
}

func isSpaceByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\f' || b == '\r'
}
