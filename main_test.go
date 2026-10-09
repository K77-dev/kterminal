package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kterminal/internal/catalog"
	"kterminal/internal/config"
	"kterminal/internal/kspec"
	"kterminal/internal/llm"
	"kterminal/internal/telemetry"
)

func sessionDir(t *testing.T) string {
	t.Helper()
	base := filepath.Join(t.TempDir(), "data")
	t.Setenv("XDG_DATA_HOME", base)
	dir := filepath.Join(base, "kterminal", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}
	return dir
}

func writeSnapshotFile(t *testing.T, dir, name, messagesJSON string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	line := `{"type":"snapshot","messages":[` + messagesJSON + `]}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		t.Fatalf("write session file: %v", err)
	}
	return path
}

func writeEventFile(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	line := `{"type":"user","content":"hello"}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		t.Fatalf("write session file: %v", err)
	}
	return path
}

func TestResolveSessionContinueLoadsLatest(t *testing.T) {
	dir := sessionDir(t)
	writeSnapshotFile(t, dir, "20240101-000000.jsonl", `{"role":"user","content":"old session"}`)
	latest := writeSnapshotFile(t, dir, "20240102-000000.jsonl", `{"role":"user","content":"latest session"}`)
	original, err := os.ReadFile(latest)
	if err != nil {
		t.Fatalf("read latest: %v", err)
	}

	w, snap, freshWarning, err := resolveSession(true, "")
	if err != nil {
		t.Fatalf("resolveSession: %v", err)
	}
	t.Cleanup(func() { w.Close() })
	if w.Path() != latest {
		t.Fatalf("path = %s, want %s", w.Path(), latest)
	}
	if freshWarning {
		t.Fatal("freshWarning = true, want false")
	}
	if len(snap.Messages) != 1 || snap.Messages[0].Content != "latest session" {
		t.Fatalf("messages = %+v, want latest session", snap.Messages)
	}
	if snap.Skill != "" {
		t.Fatalf("snapshot skill = %q, want empty", snap.Skill)
	}

	resumed := []llm.Message{
		{Role: "user", Content: "latest session"},
		{Role: "assistant", Content: "resumed answer"},
	}
	if err := w.WriteSnapshot(resumed, "", ""); err != nil {
		t.Fatalf("append snapshot: %v", err)
	}
	data, err := os.ReadFile(latest)
	if err != nil {
		t.Fatalf("read latest after append: %v", err)
	}
	if !bytes.HasPrefix(data, original) {
		t.Fatalf("original content not preserved on resume: %s", data)
	}
	if got := bytes.Count(data, []byte("\n")); got != 2 {
		t.Fatalf("transcript lines = %d, want 2", got)
	}
}

func TestResolveSessionContinueNoSessions(t *testing.T) {
	sessionDir(t)
	w, snap, freshWarning, err := resolveSession(true, "")
	if err != nil {
		t.Fatalf("resolveSession: %v", err)
	}
	if w != nil {
		t.Fatalf("writer = %v, want nil", w)
	}
	if len(snap.Messages) != 0 {
		t.Fatalf("messages = %+v, want none", snap.Messages)
	}
	if !freshWarning {
		t.Fatal("freshWarning = false, want true")
	}

	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "missing"))
	if _, _, freshWarning, err := resolveSession(true, ""); err != nil || !freshWarning {
		t.Fatalf("missing sessions dir: err=%v freshWarning=%v, want no error and freshWarning=true", err, freshWarning)
	}
}

func TestResolveSessionContinueCorruptLatest(t *testing.T) {
	dir := sessionDir(t)
	writeEventFile(t, dir, "20240101-000000.jsonl")
	if _, _, _, err := resolveSession(true, ""); err == nil || err.Error() != "sessão sem snapshot" {
		t.Fatalf("got error %v, want 'sessão sem snapshot'", err)
	}
}

func TestResolveSessionSpecificPath(t *testing.T) {
	dir := sessionDir(t)
	writeSnapshotFile(t, dir, "20240101-000000.jsonl", `{"role":"user","content":"latest session"}`)
	path := writeSnapshotFile(t, dir, "specific.jsonl", `{"role":"user","content":"specific session"}`)

	w, snap, freshWarning, err := resolveSession(false, path)
	if err != nil {
		t.Fatalf("resolveSession: %v", err)
	}
	t.Cleanup(func() { w.Close() })
	if w.Path() != path {
		t.Fatalf("path = %s, want %s", w.Path(), path)
	}
	if freshWarning {
		t.Fatal("freshWarning = true, want false")
	}
	if len(snap.Messages) != 1 || snap.Messages[0].Content != "specific session" {
		t.Fatalf("messages = %+v, want specific session", snap.Messages)
	}
}

func TestResolveSessionSpecificPathErrors(t *testing.T) {
	if _, _, _, err := resolveSession(false, filepath.Join(t.TempDir(), "missing.jsonl")); err == nil {
		t.Fatal("want error for missing session file")
	}

	dir := sessionDir(t)
	path := writeEventFile(t, dir, "no-snapshot.jsonl")
	if _, _, _, err := resolveSession(false, path); err == nil || err.Error() != "sessão sem snapshot" {
		t.Fatalf("got error %v, want 'sessão sem snapshot'", err)
	}
}

func TestResolveSessionFlagWinsOverContinue(t *testing.T) {
	dir := sessionDir(t)
	writeSnapshotFile(t, dir, "20240101-000000.jsonl", `{"role":"user","content":"latest session"}`)
	path := writeSnapshotFile(t, dir, "specific.jsonl", `{"role":"user","content":"specific session"}`)

	w, snap, freshWarning, err := resolveSession(true, path)
	if err != nil {
		t.Fatalf("resolveSession: %v", err)
	}
	t.Cleanup(func() { w.Close() })
	if w.Path() != path {
		t.Fatalf("path = %s, want %s (--session must win over --continue)", w.Path(), path)
	}
	if freshWarning {
		t.Fatal("freshWarning = true, want false")
	}
	if len(snap.Messages) != 1 || snap.Messages[0].Content != "specific session" {
		t.Fatalf("messages = %+v, want specific session", snap.Messages)
	}
}

func TestResolveSessionNoFlagsStartsFresh(t *testing.T) {
	dir := sessionDir(t)
	writeSnapshotFile(t, dir, "20240101-000000.jsonl", `{"role":"user","content":"latest session"}`)
	w, snap, freshWarning, err := resolveSession(false, "")
	if err != nil {
		t.Fatalf("resolveSession: %v", err)
	}
	if w != nil {
		t.Fatalf("writer = %v, want nil", w)
	}
	if len(snap.Messages) != 0 {
		t.Fatalf("messages = %+v, want none", snap.Messages)
	}
	if freshWarning {
		t.Fatal("freshWarning = true, want false")
	}
}

func TestTelemetryPathRespectsXDGDataHome(t *testing.T) {
	base := filepath.Join(t.TempDir(), "data")
	t.Setenv("XDG_DATA_HOME", base)
	want := filepath.Join(base, "kterminal", "telemetry.json")
	if got := telemetryPath(); got != want {
		t.Fatalf("telemetryPath() = %s, want %s", got, want)
	}
}

func TestDoctorPrintsTelemetryTable(t *testing.T) {
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	store := telemetry.New(filepath.Join(t.TempDir(), "telemetry.json"))
	measured := cat.Models[0]
	for i := 0; i < 3; i++ {
		store.Record(measured.Name, 84)
	}

	out := doctorTelemetryTable(store, cat)

	if !strings.Contains(out, "model | measured (mean tok/s, samples) | estimate (tok/s)") {
		t.Fatalf("table missing measured/estimate column header:\n%s", out)
	}
	for _, m := range cat.Models {
		measuredCol := "no samples"
		if m.Name == measured.Name {
			measuredCol = "84.0 tok/s, 3 samples"
		}
		wantRow := fmt.Sprintf("  %s | %s | %.0f tok/s\n", m.Name, measuredCol, m.TPSEstimate)
		if !strings.Contains(out, wantRow) {
			t.Fatalf("table missing row %q:\n%s", wantRow, out)
		}
	}
}

func TestDoctorLimitsLines(t *testing.T) {
	xdg := filepath.Join(t.TempDir(), "xdg")
	if err := os.MkdirAll(filepath.Join(xdg, "kterminal"), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", xdg)
	content := `
[llm]
idle_timeout = "90s"

[agent]
subagent_timeout = "2m"
`
	if err := os.WriteFile(filepath.Join(xdg, "kterminal", "config.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	out := doctorLimitsLines(cfg)

	want := "  llm timeouts: first byte 1m0s, idle 1m30s, total 10m0s, retries 2\n" +
		"  agent: subagent stall 2m0s\n"
	if out != want {
		t.Fatalf("limits lines = %q, want %q", out, want)
	}
}

func doctorStoreAt(t *testing.T, dir string) *kspec.Store {
	t.Helper()
	t.Chdir(dir)
	return kspec.Load()
}

func writeDoctorProjectSkill(t *testing.T, dir, name, body string) {
	t.Helper()
	skillDir := filepath.Join(dir, ".agents", "skills", name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	skill := "---\nname: " + name + "\nversion: 9.8.7\ndescription: Custom.\n---\n" + body
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skill), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
}

func TestDoctorKspecSectionEmbeddedMode(t *testing.T) {
	vData, err := os.ReadFile(filepath.Join("internal", "kspec", "embed", "VERSION"))
	if err != nil {
		t.Fatalf("read embedded VERSION: %v", err)
	}
	store := doctorStoreAt(t, t.TempDir())

	out := doctorKspecSection(store)

	want := "kspec: v" + strings.TrimSpace(string(vData)) + " (" + kspec.SourceEmbedded + ")\n"
	if out != want {
		t.Fatalf("section = %q, want %q", out, want)
	}
}

func TestDoctorKspecSectionProjectMode(t *testing.T) {
	dir := t.TempDir()
	writeDoctorProjectSkill(t, dir, "kspec-version", "body\n")

	out := doctorKspecSection(doctorStoreAt(t, dir))

	want := "kspec: v9.8.7 (" + kspec.SourceProject + ")\n"
	if out != want {
		t.Fatalf("section = %q, want %q", out, want)
	}
}

func TestDoctorKspecSectionProjectModeUnknownVersion(t *testing.T) {
	dir := t.TempDir()
	writeDoctorProjectSkill(t, dir, "kspec-prd", "body\n")

	out := doctorKspecSection(doctorStoreAt(t, dir))

	want := "kspec: version unknown (" + kspec.SourceProject + ")\n"
	if out != want {
		t.Fatalf("section = %q, want %q", out, want)
	}
}

func TestDoctorKspecSectionReportsInvalidSkills(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, ".agents", "skills", "kspec-broken")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("no frontmatter\n"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	out := doctorKspecSection(doctorStoreAt(t, dir))

	if !strings.HasPrefix(out, "kspec: version unknown ("+kspec.SourceProject+")\n") {
		t.Fatalf("section = %q, want the version line first", out)
	}
	if !strings.Contains(out, "WARNING: invalid skill .agents/skills/kspec-broken:") {
		t.Fatalf("section = %q, want the invalid skill warning", out)
	}
}
