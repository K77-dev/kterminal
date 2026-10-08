package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func skipWithoutGo(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not in PATH")
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func newTempModule(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "go.mod"), "module example.com/tmp\n\ngo 1.21\n")
	return root
}

func TestGoModuleRootFound(t *testing.T) {
	root := newTempModule(t)
	path := filepath.Join(root, "internal", "x", "a.go")
	writeTestFile(t, path, "package x\n")

	gotRoot, gotRel, ok := goModuleRoot(path)

	if !ok {
		t.Fatal("ok = false, want true")
	}
	if gotRoot != root {
		t.Fatalf("root = %q, want %q", gotRoot, root)
	}
	wantRel := filepath.Join("internal", "x", "a.go")
	if gotRel != wantRel {
		t.Fatalf("rel = %q, want %q", gotRel, wantRel)
	}
}

func TestGoModuleRootNotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.go")
	writeTestFile(t, path, "package main\n")

	_, _, ok := goModuleRoot(path)

	if ok {
		t.Fatal("ok = true, want false outside module")
	}
}

func TestVetHookReturnsDiagnostic(t *testing.T) {
	skipWithoutGo(t)
	root := newTempModule(t)
	rel := filepath.Join("internal", "x", "broken.go")
	writeTestFile(t, filepath.Join(root, rel), "package x\n\nfunc F() int {\n\treturn Foo\n}\n")

	got := GoVetHook(filepath.Join(root, rel))

	if !strings.Contains(got, "\n\nDiagnostics:\n") {
		t.Fatalf("GoVetHook = %q, want Diagnostics block", got)
	}
	if !strings.Contains(got, "undefined: Foo") {
		t.Fatalf("GoVetHook = %q, want undefined: Foo", got)
	}
	if !strings.Contains(got, rel) {
		t.Fatalf("GoVetHook = %q, want line mentioning %q", got, rel)
	}
}

func TestVetHookCleanOnValidFile(t *testing.T) {
	skipWithoutGo(t)
	root := newTempModule(t)
	writeTestFile(t, filepath.Join(root, "internal", "x", "ok.go"), "package x\n\nfunc F() int {\n\treturn 1\n}\n")

	got := GoVetHook(filepath.Join(root, "internal", "x", "ok.go"))

	if got != "\n\nDiagnostics: clean" {
		t.Fatalf("GoVetHook = %q, want clean", got)
	}
}

func TestVetHookNoModuleEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.go")
	writeTestFile(t, path, "package main\n\nfunc main() {}\n")

	got := GoVetHook(path)

	if got != "" {
		t.Fatalf("GoVetHook = %q, want empty outside module", got)
	}
}

func TestVetHookFiltersOtherFiles(t *testing.T) {
	skipWithoutGo(t)
	root := newTempModule(t)
	writeTestFile(t, filepath.Join(root, "internal", "x", "other.go"), "package x\n\nfunc G() int {\n\treturn Bar\n}\n")
	writeTestFile(t, filepath.Join(root, "internal", "x", "ok.go"), "package x\n\nfunc F() int {\n\treturn 1\n}\n")

	got := GoVetHook(filepath.Join(root, "internal", "x", "ok.go"))

	if got != "\n\nDiagnostics: clean" {
		t.Fatalf("GoVetHook = %q, want clean with other-file errors filtered", got)
	}
}

func TestVetHookTimeoutKillsProcess(t *testing.T) {
	skipWithoutGo(t)
	root := newTempModule(t)
	writeTestFile(t, filepath.Join(root, "a.go"), "package main\n\nfunc main() {}\n")
	fakeDir := t.TempDir()
	fake := filepath.Join(fakeDir, "go")
	writeTestFile(t, fake, "#!/bin/sh\nexec /bin/sleep 120\n")
	if err := os.Chmod(fake, 0o755); err != nil {
		t.Fatalf("chmod fake go: %v", err)
	}
	t.Setenv("PATH", fakeDir)

	start := time.Now()
	got := GoVetHook(filepath.Join(root, "a.go"))

	if got != "" {
		t.Fatalf("GoVetHook = %q, want empty on timeout", got)
	}
	if elapsed := time.Since(start); elapsed >= vetTimeout+10*time.Second {
		t.Fatalf("GoVetHook took %v, want within timeout plus margin", elapsed)
	}
}

func TestVetHookNoGoInPath(t *testing.T) {
	root := newTempModule(t)
	writeTestFile(t, filepath.Join(root, "a.go"), "package main\n\nfunc main() {}\n")
	t.Setenv("PATH", "")

	start := time.Now()
	got := GoVetHook(filepath.Join(root, "a.go"))

	if got != "" {
		t.Fatalf("GoVetHook = %q, want empty without go in PATH", got)
	}
	if elapsed := time.Since(start); elapsed >= 2*time.Second {
		t.Fatalf("GoVetHook took %v, want immediate return", elapsed)
	}
}
