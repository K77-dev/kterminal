package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustArgs(t *testing.T, v map[string]any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	return string(b)
}

func TestGlobRecursiveDoublestar(t *testing.T) {
	dir := t.TempDir()
	write := func(rel string) {
		t.Helper()
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	write("root.tsx")
	write("frontend/src/modules/a.tsx")
	write("frontend/src/deep/nested/b.tsx")
	write("frontend/notes.txt")
	write("node_modules/pkg/c.tsx")
	write(".git/hooks/d.tsx")
	t.Chdir(dir)

	reg := NewRegistry()
	ctx := context.Background()

	res, err := reg.Execute(ctx, "glob", mustArgs(t, map[string]any{"pattern": "**/*.tsx"}))
	if err != nil {
		t.Fatalf("glob **/*.tsx: %v", err)
	}
	got := strings.Split(res.Output, "\n")
	want := []string{"frontend/src/deep/nested/b.tsx", "frontend/src/modules/a.tsx", "root.tsx"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("glob **/*.tsx = %v, want %v (recursive, skipping .git and node_modules)", got, want)
	}

	res, err = reg.Execute(ctx, "glob", mustArgs(t, map[string]any{"pattern": "frontend/src/**/*.tsx"}))
	if err != nil {
		t.Fatalf("glob frontend/src/**/*.tsx: %v", err)
	}
	got = strings.Split(res.Output, "\n")
	want = []string{"frontend/src/deep/nested/b.tsx", "frontend/src/modules/a.tsx"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("glob frontend/src/**/*.tsx = %v, want %v", got, want)
	}

	res, err = reg.Execute(ctx, "glob", mustArgs(t, map[string]any{"pattern": "*.tsx"}))
	if err != nil {
		t.Fatalf("glob *.tsx: %v", err)
	}
	if res.Output != "root.tsx" {
		t.Fatalf("glob *.tsx = %q, want single-level root.tsx only", res.Output)
	}

	res, err = reg.Execute(ctx, "glob", mustArgs(t, map[string]any{"pattern": "**/nested/*.tsx"}))
	if err != nil {
		t.Fatalf("glob **/nested/*.tsx: %v", err)
	}
	if res.Output != "frontend/src/deep/nested/b.tsx" {
		t.Fatalf("glob **/nested/*.tsx = %q, want the nested file", res.Output)
	}
}

func TestWriteToolDiffOnExistingFile(t *testing.T) {
	reg := NewRegistry()
	path := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	res, err := reg.Execute(context.Background(), "write", mustArgs(t, map[string]any{
		"path":    path,
		"content": "alpha\ngamma\n",
	}))

	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(res.Output, "wrote") || !strings.Contains(res.Output, path) {
		t.Fatalf("Output = %q, want write confirmation mentioning %s", res.Output, path)
	}
	want := []DiffLine{
		{Kind: ' ', Text: "alpha"},
		{Kind: '-', Text: "beta"},
		{Kind: '+', Text: "gamma"},
	}
	assertDiffLines(t, res.Diff, want)
	if n := countKind(res.Diff, '-'); n != 1 {
		t.Fatalf("removals = %d, want 1", n)
	}
	if n := countKind(res.Diff, '+'); n != 1 {
		t.Fatalf("additions = %d, want 1", n)
	}

	res, err = reg.Execute(context.Background(), "write", mustArgs(t, map[string]any{
		"path":    path,
		"content": "alpha\ngamma\n",
	}))

	if err != nil {
		t.Fatalf("Execute identical content: %v", err)
	}
	if len(res.Diff) != 0 {
		t.Fatalf("diff = %+v, want empty for identical content", res.Diff)
	}
}

func TestWriteToolDiffNewFile(t *testing.T) {
	reg := NewRegistry()
	path := filepath.Join(t.TempDir(), "new.txt")

	res, err := reg.Execute(context.Background(), "write", mustArgs(t, map[string]any{
		"path":    path,
		"content": "alpha\nbeta\n",
	}))

	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := []DiffLine{
		{Kind: '+', Text: "alpha"},
		{Kind: '+', Text: "beta"},
	}
	assertDiffLines(t, res.Diff, want)
	if n := countKind(res.Diff, '+'); n != len(res.Diff) {
		t.Fatalf("additions = %d, want all %d lines to be additions", n, len(res.Diff))
	}
}

func TestEditToolDiff(t *testing.T) {
	reg := NewRegistry()
	path := filepath.Join(t.TempDir(), "code.txt")
	if err := os.WriteFile(path, []byte("func main() {\n\told()\n}\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	res, err := reg.Execute(context.Background(), "edit", mustArgs(t, map[string]any{
		"path":       path,
		"old_string": "\told()",
		"new_string": "\tnew()",
	}))

	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Output != "edited "+path {
		t.Fatalf("Output = %q, want %q", res.Output, "edited "+path)
	}
	want := []DiffLine{
		{Kind: ' ', Text: "func main() {"},
		{Kind: '-', Text: "\told()"},
		{Kind: '+', Text: "\tnew()"},
		{Kind: ' ', Text: "}"},
	}
	assertDiffLines(t, res.Diff, want)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "func main() {\n\tnew()\n}\n" {
		t.Fatalf("file content = %q, want edited content", string(data))
	}

	res, err = reg.Execute(context.Background(), "edit", mustArgs(t, map[string]any{
		"path":       path,
		"old_string": "\tnew()",
		"new_string": "\tnew()",
	}))

	if err != nil {
		t.Fatalf("Execute no-op edit: %v", err)
	}
	if len(res.Diff) != 0 {
		t.Fatalf("diff = %+v, want empty for edit without changes", res.Diff)
	}
}

func TestPendingDiffDoesNotTouchDisk(t *testing.T) {
	reg := NewRegistry()
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	diff, err := reg.PendingDiff(ctx, "edit", mustArgs(t, map[string]any{
		"path":       path,
		"old_string": "beta",
		"new_string": "gamma",
	}))

	if err != nil {
		t.Fatalf("PendingDiff edit: %v", err)
	}
	want := []DiffLine{
		{Kind: ' ', Text: "alpha"},
		{Kind: '-', Text: "beta"},
		{Kind: '+', Text: "gamma"},
	}
	assertDiffLines(t, diff, want)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "alpha\nbeta\n" {
		t.Fatalf("edit PendingDiff mutated disk: %q", string(data))
	}

	diff, err = reg.PendingDiff(ctx, "write", mustArgs(t, map[string]any{
		"path":    path,
		"content": "alpha\ngamma\n",
	}))

	if err != nil {
		t.Fatalf("PendingDiff write: %v", err)
	}
	assertDiffLines(t, diff, want)
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "alpha\nbeta\n" {
		t.Fatalf("write PendingDiff mutated disk: %q", string(data))
	}

	newPath := filepath.Join(dir, "created.txt")
	diff, err = reg.PendingDiff(ctx, "write", mustArgs(t, map[string]any{
		"path":    newPath,
		"content": "brand new\n",
	}))

	if err != nil {
		t.Fatalf("PendingDiff write new file: %v", err)
	}
	if _, statErr := os.Stat(newPath); !os.IsNotExist(statErr) {
		t.Fatalf("PendingDiff created file on disk: stat err = %v", statErr)
	}
	assertDiffLines(t, diff, []DiffLine{{Kind: '+', Text: "brand new"}})
}

func TestPendingEditValidation(t *testing.T) {
	reg := NewRegistry()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(path, []byte("dup\ndup\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	cases := []map[string]any{
		{"path": path, "old_string": "missing", "new_string": "x"},
		{"path": path, "old_string": "dup", "new_string": "x"},
		{"path": filepath.Join(t.TempDir(), "nope.txt"), "old_string": "a", "new_string": "b"},
	}
	for _, args := range cases {
		argsJSON := mustArgs(t, args)
		_, execErr := reg.Execute(ctx, "edit", argsJSON)
		if execErr == nil {
			t.Fatalf("Execute should fail for %v", args)
		}
		_, pendErr := reg.PendingDiff(ctx, "edit", argsJSON)
		if pendErr == nil {
			t.Fatalf("PendingDiff should fail for %v", args)
		}
		if execErr.Error() != pendErr.Error() {
			t.Fatalf("errors differ for %v: Execute = %q, PendingDiff = %q", args, execErr, pendErr)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		if string(data) != "dup\ndup\n" {
			t.Fatalf("PendingDiff mutated disk during validation: %q", string(data))
		}
	}
}

func TestNonEditorToolsReturnNilDiff(t *testing.T) {
	reg := NewRegistry()
	ctx := context.Background()

	res, err := reg.Execute(ctx, "read", mustArgs(t, map[string]any{"path": "diff.go"}))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if res.Diff != nil {
		t.Fatalf("read diff = %+v, want nil", res.Diff)
	}

	res, err = reg.Execute(ctx, "glob", mustArgs(t, map[string]any{"pattern": "*.go"}))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if res.Diff != nil {
		t.Fatalf("glob diff = %+v, want nil", res.Diff)
	}

	res, err = reg.Execute(ctx, "grep", mustArgs(t, map[string]any{"pattern": "LineDiff", "path": "diff.go"}))
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if res.Diff != nil {
		t.Fatalf("grep diff = %+v, want nil", res.Diff)
	}

	res, err = reg.Execute(ctx, "bash", mustArgs(t, map[string]any{"command": "true"}))
	if err != nil {
		t.Fatalf("bash: %v", err)
	}
	if res.Diff != nil {
		t.Fatalf("bash diff = %+v, want nil", res.Diff)
	}

	diff, err := reg.PendingDiff(ctx, "read", `{}`)
	if err != nil {
		t.Fatalf("PendingDiff read: %v", err)
	}
	if diff != nil {
		t.Fatalf("PendingDiff read = %+v, want nil", diff)
	}

	diff, err = reg.PendingDiff(ctx, "bash", mustArgs(t, map[string]any{"command": "touch should-not-exist"}))
	if err != nil {
		t.Fatalf("PendingDiff bash: %v", err)
	}
	if diff != nil {
		t.Fatalf("PendingDiff bash = %+v, want nil", diff)
	}
	if _, statErr := os.Stat("should-not-exist"); !os.IsNotExist(statErr) {
		t.Fatalf("PendingDiff bash executed the command: stat err = %v", statErr)
	}
}
