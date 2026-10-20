package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestContextPrefersAgentsMd(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "AGENTS.md"), "agents guide")
	write(t, filepath.Join(dir, "CLAUDE.md"), "claude guide")
	write(t, filepath.Join(dir, "README.md"), "readme")

	name, content, ok := Context(dir)
	if !ok || name != "AGENTS.md" || content != "agents guide" {
		t.Fatalf("Context = %q/%q/%v, want AGENTS.md/agents guide/true", name, content, ok)
	}
}

func TestContextFallsBackToClaudeThenReadme(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "CLAUDE.md"), "claude guide")
	write(t, filepath.Join(dir, "README.md"), "readme")

	name, content, ok := Context(dir)
	if !ok || name != "CLAUDE.md" || content != "claude guide" {
		t.Fatalf("Context = %q/%q/%v, want CLAUDE.md/claude guide/true", name, content, ok)
	}

	dir = t.TempDir()
	write(t, filepath.Join(dir, "README.md"), "readme")
	name, content, ok = Context(dir)
	if !ok || name != "README.md" || content != "readme" {
		t.Fatalf("Context = %q/%q/%v, want README.md/readme/true", name, content, ok)
	}
}

func TestContextEmptyDir(t *testing.T) {
	_, _, ok := Context(t.TempDir())
	if ok {
		t.Fatal("Context ok = true for empty dir")
	}
	if _, _, ok := Context(""); ok {
		t.Fatal("Context ok = true for empty path")
	}
}

func TestContextTruncates(t *testing.T) {
	dir := t.TempDir()
	big := strings.Repeat("a", maxContextBytes+500)
	write(t, filepath.Join(dir, "AGENTS.md"), big)

	_, content, ok := Context(dir)
	if !ok {
		t.Fatal("Context ok = false")
	}
	if len(content) != maxContextBytes+len("\n... (truncated)") {
		t.Fatalf("content length = %d, want %d", len(content), maxContextBytes+len("\n... (truncated)"))
	}
	if !strings.HasSuffix(content, "... (truncated)") {
		t.Fatal("content missing truncation marker")
	}
}

func TestFilesSkipsNoiseDirs(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "go.mod"), "module x")
	write(t, filepath.Join(dir, "node_modules", "pkg", "a.js"), "x")
	write(t, filepath.Join(dir, ".git", "config"), "x")
	write(t, filepath.Join(dir, "vendor", "dep.go"), "x")
	write(t, filepath.Join(dir, "graphify-out", "graph.json"), "x")
	write(t, filepath.Join(dir, "internal", "a.go"), "x")

	files := Files(dir)
	joined := "\n" + strings.Join(files, "\n") + "\n"
	for _, unwanted := range []string{"node_modules", ".git", "vendor", "graphify-out"} {
		if strings.Contains(joined, unwanted) {
			t.Fatalf("Files leaked %s: %v", unwanted, files)
		}
	}
	want := map[string]bool{"go.mod": false, "internal/a.go": false}
	for _, f := range files {
		if _, ok := want[f]; ok {
			want[f] = true
		}
	}
	for f, found := range want {
		if !found {
			t.Fatalf("Files missing %s: %v", f, files)
		}
	}
}

func TestFilesEmptyDir(t *testing.T) {
	if Files("") != nil {
		t.Fatal("Files(\"\") != nil")
	}
	if files := Files(t.TempDir()); len(files) != 0 {
		t.Fatalf("Files = %v, want empty", files)
	}
}

func TestMatchesAny(t *testing.T) {
	files := []string{"src/a.ts", "prisma/schema.prisma", "main.go"}
	if !MatchesAny([]string{"**/*.ts"}, files) {
		t.Fatal("MatchesAny = false for **/*.ts")
	}
	if !MatchesAny([]string{"**/prisma/**"}, files) {
		t.Fatal("MatchesAny = false for **/prisma/**")
	}
	if MatchesAny([]string{"**/*.java"}, files) {
		t.Fatal("MatchesAny = true for **/*.java")
	}
	if MatchesAny([]string{"**/*.java"}, nil) {
		t.Fatal("MatchesAny = true with no files")
	}
}
