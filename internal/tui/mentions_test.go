package tui

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	return dir
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestExpandExistingFile(t *testing.T) {
	dir := chdirTemp(t)
	writeFile(t, dir, "a.txt", "conteúdo do arquivo")

	expanded, warnings := ExpandMentions("@a.txt pergunta")
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	want := "@a.txt pergunta\n\n--- Arquivo @a.txt ---\nconteúdo do arquivo\n"
	if expanded != want {
		t.Fatalf("unexpected expansion:\nwant: %q\ngot:  %q", want, expanded)
	}
}

func TestExpandMissingFilePassesThrough(t *testing.T) {
	dir := chdirTemp(t)

	expanded, warnings := ExpandMentions("@nope.txt")
	if expanded != "@nope.txt" {
		t.Fatalf("expected identical output, got %q", expanded)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	midword := "meu email é usuario@host.com"
	expanded, warnings = ExpandMentions(midword)
	if expanded != midword {
		t.Fatalf("mid-word @ must not be a mention, got %q", expanded)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	sockPath := filepath.Join(dir, "sock")
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("socket: %v", err)
	}
	defer listener.Close()
	defer os.Remove(sockPath)

	expanded, warnings = ExpandMentions("leia @sock")
	if expanded != "leia @sock" {
		t.Fatalf("unreadable file must pass through, got %q", expanded)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
}

func TestExpandMultipleMentions(t *testing.T) {
	dir := chdirTemp(t)
	writeFile(t, dir, "a.txt", "conteúdo A")
	writeFile(t, dir, "b.txt", "conteúdo B")
	writeFile(t, dir, "c.txt", "conteúdo C")

	expanded, warnings := ExpandMentions("veja @b.txt e @a.txt e @c.txt")
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if strings.Count(expanded, "--- Arquivo @") != 3 {
		t.Fatalf("expected 3 blocks, got:\n%s", expanded)
	}
	ib := strings.Index(expanded, "--- Arquivo @b.txt ---")
	ia := strings.Index(expanded, "--- Arquivo @a.txt ---")
	ic := strings.Index(expanded, "--- Arquivo @c.txt ---")
	if ib < 0 || ia < 0 || ic < 0 {
		t.Fatalf("missing block(s):\n%s", expanded)
	}
	if !(ib < ia && ia < ic) {
		t.Fatalf("blocks out of appearance order (b=%d a=%d c=%d):\n%s", ib, ia, ic, expanded)
	}
}

func TestExpandLimitFive(t *testing.T) {
	dir := chdirTemp(t)
	for _, name := range []string{"f1.txt", "f2.txt", "f3.txt", "f4.txt", "f5.txt", "f6.txt"} {
		writeFile(t, dir, name, "conteúdo "+name)
	}
	text := "@f1.txt @f2.txt @f3.txt @f4.txt @f5.txt @f6.txt"

	expanded, warnings := ExpandMentions(text)
	if strings.Count(expanded, "--- Arquivo @") != 5 {
		t.Fatalf("expected 5 blocks, got:\n%s", expanded)
	}
	if strings.Contains(expanded, "--- Arquivo @f6.txt ---") {
		t.Fatalf("6th mention should not be expanded:\n%s", expanded)
	}
	if !strings.HasPrefix(expanded, text) {
		t.Fatalf("original text not preserved:\n%s", expanded)
	}
	if len(warnings) != 1 || warnings[0] != "max 5 file mentions" {
		t.Fatalf("expected single limit warning, got %v", warnings)
	}
}

func TestExpandIgnoresCodeFences(t *testing.T) {
	dir := chdirTemp(t)
	writeFile(t, dir, "a.txt", "conteúdo A")
	writeFile(t, dir, "b.txt", "conteúdo B")

	indented := "    @a.txt"
	expanded, warnings := ExpandMentions(indented)
	if expanded != indented {
		t.Fatalf("4-space fence mention should pass through:\n%s", expanded)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	fenced := "```go\n@a.txt\n```\nveja @b.txt"
	expanded, warnings = ExpandMentions(fenced)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if strings.Contains(expanded, "--- Arquivo @a.txt ---") {
		t.Fatalf("fenced mention should not be expanded:\n%s", expanded)
	}
	if !strings.Contains(expanded, "--- Arquivo @b.txt ---") {
		t.Fatalf("mention after fence should be expanded:\n%s", expanded)
	}
}

func TestExpandTruncatesLargeFile(t *testing.T) {
	dir := chdirTemp(t)
	writeFile(t, dir, "big.txt", strings.Repeat("x", 100*1024))

	expanded, warnings := ExpandMentions("@big.txt")
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	header := "--- Arquivo @big.txt ---\n"
	idx := strings.Index(expanded, header)
	if idx < 0 {
		t.Fatalf("missing block header:\n%s", expanded)
	}
	content := expanded[idx+len(header):]
	want := strings.Repeat("x", maxMentionBytes) + "\n... (truncated)\n"
	if content != want {
		t.Fatalf("unexpected truncated content:\nwant: %d bytes\ngot:  %d bytes", len(want), len(content))
	}
}

func TestMentionPrefixExtraction(t *testing.T) {
	if p, ok := mentionPrefix("@int"); !ok || p != "int" {
		t.Fatalf("mentionPrefix(@int) = (%q, %v), want (int, true)", p, ok)
	}
	if p, ok := mentionPrefix("texto @internal/t"); !ok || p != "internal/t" {
		t.Fatalf("mentionPrefix(texto @internal/t) = (%q, %v), want (internal/t, true)", p, ok)
	}
	if p, ok := mentionPrefix("sem arroba"); ok {
		t.Fatalf("mentionPrefix(sem arroba) = (%q, %v), want ok=false", p, ok)
	}
	if p, ok := mentionPrefix("@ com espaço"); ok {
		t.Fatalf("mentionPrefix(@ com espaço) = (%q, %v), want ok=false", p, ok)
	}
	if p, ok := mentionPrefix("email@host"); ok {
		t.Fatalf("mentionPrefix(email@host) = (%q, %v), want ok=false (boundary)", p, ok)
	}
	if p, ok := mentionPrefix("@"); !ok || p != "" {
		t.Fatalf("mentionPrefix(@) = (%q, %v), want (\"\", true)", p, ok)
	}
}
