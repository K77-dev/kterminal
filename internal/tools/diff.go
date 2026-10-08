package tools

import "strings"

const maxDiffLines = 10000

type DiffLine struct {
	Kind byte
	Text string
}

func LineDiff(old, new string) []DiffLine {
	if old == new {
		return nil
	}
	oldLines := splitLines(old)
	newLines := splitLines(new)
	if len(oldLines) > maxDiffLines || len(newLines) > maxDiffLines {
		return fallbackDiff(oldLines, newLines)
	}
	return lcsDiff(oldLines, newLines)
}

func splitLines(content string) []string {
	if content == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(content, "\n"), "\n")
}

func fallbackDiff(oldLines, newLines []string) []DiffLine {
	diff := make([]DiffLine, 0, len(oldLines)+len(newLines))
	for _, text := range oldLines {
		diff = append(diff, DiffLine{Kind: '-', Text: text})
	}
	for _, text := range newLines {
		diff = append(diff, DiffLine{Kind: '+', Text: text})
	}
	return diff
}

func lcsDiff(oldLines, newLines []string) []DiffLine {
	n := len(oldLines)
	m := len(newLines)
	dp := make([][]int32, n+1)
	for i := range dp {
		dp[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	diff := make([]DiffLine, 0, n+m)
	changed := false
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case oldLines[i] == newLines[j]:
			diff = append(diff, DiffLine{Kind: ' ', Text: oldLines[i]})
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			diff = append(diff, DiffLine{Kind: '-', Text: oldLines[i]})
			i++
			changed = true
		default:
			diff = append(diff, DiffLine{Kind: '+', Text: newLines[j]})
			j++
			changed = true
		}
	}
	for ; i < n; i++ {
		diff = append(diff, DiffLine{Kind: '-', Text: oldLines[i]})
		changed = true
	}
	for ; j < m; j++ {
		diff = append(diff, DiffLine{Kind: '+', Text: newLines[j]})
		changed = true
	}
	if !changed {
		return nil
	}
	return diff
}
