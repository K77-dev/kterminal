package tools

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func assertDiffLines(t *testing.T, got, want []DiffLine) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("diff length = %d, want %d (got %+v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("diff[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func countKind(diff []DiffLine, kind byte) int {
	count := 0
	for _, line := range diff {
		if line.Kind == kind {
			count++
		}
	}
	return count
}

func TestLineDiffInsertion(t *testing.T) {
	old := "alpha\nbeta\n"
	new := "alpha\nbeta\ngamma\n"

	got := LineDiff(old, new)

	want := []DiffLine{
		{Kind: ' ', Text: "alpha"},
		{Kind: ' ', Text: "beta"},
		{Kind: '+', Text: "gamma"},
	}
	assertDiffLines(t, got, want)
	if n := countKind(got, '+'); n != 1 {
		t.Fatalf("additions = %d, want 1", n)
	}
}

func TestLineDiffRemoval(t *testing.T) {
	old := "alpha\nbeta\ngamma\n"
	new := "alpha\ngamma\n"

	got := LineDiff(old, new)

	want := []DiffLine{
		{Kind: ' ', Text: "alpha"},
		{Kind: '-', Text: "beta"},
		{Kind: ' ', Text: "gamma"},
	}
	assertDiffLines(t, got, want)
	if n := countKind(got, '-'); n != 1 {
		t.Fatalf("removals = %d, want 1", n)
	}
}

func TestLineDiffMiddleChange(t *testing.T) {
	old := "func main() {\n\told()\n}\n"
	new := "func main() {\n\tnew()\n}\n"

	got := LineDiff(old, new)

	want := []DiffLine{
		{Kind: ' ', Text: "func main() {"},
		{Kind: '-', Text: "\told()"},
		{Kind: '+', Text: "\tnew()"},
		{Kind: ' ', Text: "}"},
	}
	assertDiffLines(t, got, want)
	minusIdx := -1
	plusIdx := -1
	for i, line := range got {
		switch line.Kind {
		case '-':
			if minusIdx != -1 {
				t.Fatalf("expected exactly one removal, got another at index %d", i)
			}
			minusIdx = i
		case '+':
			if plusIdx != -1 {
				t.Fatalf("expected exactly one addition, got another at index %d", i)
			}
			plusIdx = i
		}
	}
	if plusIdx != minusIdx+1 {
		t.Fatalf("removal at %d and addition at %d are not adjacent", minusIdx, plusIdx)
	}
	if got[0].Kind != ' ' || got[len(got)-1].Kind != ' ' {
		t.Fatalf("context not preserved around the change: %+v", got)
	}
}

func TestLineDiffEqual(t *testing.T) {
	content := "line one\nline two\nline three\n"

	got := LineDiff(content, content)

	if len(got) != 0 {
		t.Fatalf("diff length = %d, want 0 for equal contents (got %+v)", len(got), got)
	}

	got = LineDiff("alpha\nbeta\n", "alpha\nbeta")

	if len(got) != 0 {
		t.Fatalf("diff length = %d, want 0 for line-identical contents (got %+v)", len(got), got)
	}
}

func TestLineDiffEmpty(t *testing.T) {
	got := LineDiff("", "alpha\nbeta\n")

	want := []DiffLine{
		{Kind: '+', Text: "alpha"},
		{Kind: '+', Text: "beta"},
	}
	assertDiffLines(t, got, want)
	if n := countKind(got, '+'); n != len(got) {
		t.Fatalf("additions = %d, want all %d lines to be additions", n, len(got))
	}

	got = LineDiff("alpha\nbeta\n", "")

	want = []DiffLine{
		{Kind: '-', Text: "alpha"},
		{Kind: '-', Text: "beta"},
	}
	assertDiffLines(t, got, want)
	if n := countKind(got, '-'); n != len(got) {
		t.Fatalf("removals = %d, want all %d lines to be removals", n, len(got))
	}
}

func TestLineDiffLargeFallback(t *testing.T) {
	oldLines := make([]string, maxDiffLines+1)
	for i := range oldLines {
		oldLines[i] = fmt.Sprintf("line %d", i)
	}
	newLines := slices.Clone(oldLines)
	newLines[maxDiffLines] = "line final"
	old := strings.Join(oldLines, "\n") + "\n"
	new := strings.Join(newLines, "\n") + "\n"

	got := LineDiff(old, new)

	if len(got) != len(oldLines)+len(newLines) {
		t.Fatalf("degraded diff length = %d, want %d", len(got), len(oldLines)+len(newLines))
	}
	for i, text := range oldLines {
		if got[i] != (DiffLine{Kind: '-', Text: text}) {
			t.Fatalf("degraded diff[%d] = %+v, want removal of %q", i, got[i], text)
		}
	}
	for i, text := range newLines {
		if got[len(oldLines)+i] != (DiffLine{Kind: '+', Text: text}) {
			t.Fatalf("degraded diff[%d] = %+v, want addition of %q", len(oldLines)+i, got[len(oldLines)+i], text)
		}
	}
	if n := countKind(got, ' '); n != 0 {
		t.Fatalf("degraded diff has %d context lines, want 0", n)
	}

	got = LineDiff(old, "tiny\n")

	if len(got) != len(oldLines)+1 {
		t.Fatalf("diff length = %d, want %d when only the old side exceeds the limit", len(got), len(oldLines)+1)
	}
	if got[0].Kind != '-' || got[len(oldLines)-1].Kind != '-' {
		t.Fatalf("old lines not all removals when only the old side exceeds the limit")
	}
	if got[len(oldLines)] != (DiffLine{Kind: '+', Text: "tiny"}) {
		t.Fatalf("new line not added when only the old side exceeds the limit: %+v", got[len(oldLines)])
	}

	got = LineDiff("tiny\n", new)

	if len(got) != 1+len(newLines) {
		t.Fatalf("diff length = %d, want %d when only the new side exceeds the limit", len(got), 1+len(newLines))
	}
	if got[0] != (DiffLine{Kind: '-', Text: "tiny"}) {
		t.Fatalf("old line not removed when only the new side exceeds the limit: %+v", got[0])
	}
	for i, line := range got[1:] {
		if line.Kind != '+' {
			t.Fatalf("new lines not all additions when only the new side exceeds the limit: index %d is %+v", i, line)
		}
	}

	got = LineDiff(old, old)

	if len(got) != 0 {
		t.Fatalf("diff length = %d, want 0 for equal large contents", len(got))
	}
}
