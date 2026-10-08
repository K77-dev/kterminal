package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegistryPassesContextToExecutor(t *testing.T) {
	reg := NewRegistry()
	var seen context.Context
	reg.Register(Tool{
		Name:     "probe",
		Mutating: false,
		Execute: func(ctx context.Context, args map[string]any) (Result, error) {
			seen = ctx
			return Result{Output: "ok"}, nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, err := reg.Execute(ctx, "probe", `{}`); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if seen != ctx {
		t.Fatal("executor received a different context than the one passed to Execute")
	}
	if seen.Err() != nil {
		t.Fatalf("executor ctx err = %v, want live context", seen.Err())
	}

	cancel()

	if _, err := reg.Execute(ctx, "probe", `{}`); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if seen == nil || seen.Err() != context.Canceled {
		t.Fatalf("executor ctx err = %v, want context.Canceled", seen.Err())
	}
}

func TestExecuteCallsHookOnGoEdit(t *testing.T) {
	reg := NewRegistry()
	var paths []string
	reg.SetOnGoEdit(func(path string) string {
		paths = append(paths, path)
		return "\n\nDiagnostics: fake"
	})
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "main.go")

	_, err := reg.Execute(ctx, "edit", mustArgs(t, map[string]any{
		"path":       path,
		"old_string": "a",
		"new_string": "b",
	}))
	if err == nil {
		t.Fatal("edit on missing file should fail")
	}
	if len(paths) != 0 {
		t.Fatalf("hook paths = %v, want none after failed edit", paths)
	}

	res, err := reg.ExecuteStream(ctx, "write", mustArgs(t, map[string]any{
		"path":    path,
		"content": "package main\n",
	}), func(line string) {})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if len(paths) != 1 || paths[0] != path {
		t.Fatalf("hook paths = %v, want [%s]", paths, path)
	}
	if !strings.Contains(res.Output, "wrote") || !strings.Contains(res.Output, path) {
		t.Fatalf("Output = %q, want write confirmation", res.Output)
	}
	if !strings.HasSuffix(res.Output, "\n\nDiagnostics: fake") {
		t.Fatalf("Output = %q, want hook result appended", res.Output)
	}
	if len(res.Diff) == 0 {
		t.Fatalf("diff = %+v, want write diff preserved alongside hook output", res.Diff)
	}

	res, err = reg.Execute(ctx, "edit", mustArgs(t, map[string]any{
		"path":       path,
		"old_string": "package main",
		"new_string": "package main\n\nfunc main() {}",
	}))
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if len(paths) != 2 || paths[1] != path {
		t.Fatalf("hook paths = %v, want two calls with %s", paths, path)
	}
	if !strings.Contains(res.Output, "edited "+path) {
		t.Fatalf("Output = %q, want edit confirmation", res.Output)
	}
	if !strings.HasSuffix(res.Output, "\n\nDiagnostics: fake") {
		t.Fatalf("Output = %q, want hook result appended", res.Output)
	}
}

func TestExecuteSkipsHookForNonGoOrOtherTools(t *testing.T) {
	reg := NewRegistry()
	calls := 0
	reg.SetOnGoEdit(func(path string) string {
		calls++
		return "\n\nDiagnostics: fake"
	})
	ctx := context.Background()
	dir := t.TempDir()

	txtPath := filepath.Join(dir, "notes.txt")
	if _, err := reg.Execute(ctx, "write", mustArgs(t, map[string]any{
		"path":    txtPath,
		"content": "hello\n",
	})); err != nil {
		t.Fatalf("write txt: %v", err)
	}

	mdPath := filepath.Join(dir, "readme.md")
	if err := os.WriteFile(mdPath, []byte("old\n"), 0o644); err != nil {
		t.Fatalf("setup md: %v", err)
	}
	if _, err := reg.Execute(ctx, "edit", mustArgs(t, map[string]any{
		"path":       mdPath,
		"old_string": "old",
		"new_string": "new",
	})); err != nil {
		t.Fatalf("edit md: %v", err)
	}

	goPath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(goPath, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("setup go: %v", err)
	}
	if _, err := reg.Execute(ctx, "read", mustArgs(t, map[string]any{"path": goPath})); err != nil {
		t.Fatalf("read go: %v", err)
	}

	if _, err := reg.Execute(ctx, "bash", mustArgs(t, map[string]any{"command": "true"})); err != nil {
		t.Fatalf("bash: %v", err)
	}

	if calls != 0 {
		t.Fatalf("hook calls = %d, want 0 for txt/md/read/bash", calls)
	}
}

func TestExecuteNilHookNoop(t *testing.T) {
	reg := NewRegistry()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "main.go")
	content := "package main\n"

	res, err := reg.Execute(ctx, "write", mustArgs(t, map[string]any{
		"path":    path,
		"content": content,
	}))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	want := fmt.Sprintf("wrote %d bytes to %s", len(content), path)
	if res.Output != want {
		t.Fatalf("Output = %q, want %q", res.Output, want)
	}

	res, err = reg.Execute(ctx, "edit", mustArgs(t, map[string]any{
		"path":       path,
		"old_string": "package main",
		"new_string": "package main\n\nfunc main() {}",
	}))
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if res.Output != "edited "+path {
		t.Fatalf("Output = %q, want %q", res.Output, "edited "+path)
	}
}
