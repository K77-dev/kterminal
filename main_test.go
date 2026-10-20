package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"kterminal/internal/catalog"
	"kterminal/internal/config"
	"kterminal/internal/kspec"
	"kterminal/internal/llm"
	"kterminal/internal/router"
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
	if err := w.WriteSnapshot(resumed, "", "", "", nil); err != nil {
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

func TestModelsCachePathRespectsXDGDataHome(t *testing.T) {
	base := filepath.Join(t.TempDir(), "data")
	t.Setenv("XDG_DATA_HOME", base)
	want := filepath.Join(base, "kterminal", "models-cache.json")
	if got := modelsCachePath(); got != want {
		t.Fatalf("modelsCachePath() = %s, want %s", got, want)
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

	want := "  llm timeouts: first byte 2m0s, idle 1m30s, total 10m0s, retries 2\n" +
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

func TestValidateModel(t *testing.T) {
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	if err := validateModel("", cat); err != nil {
		t.Fatalf("empty model: err = %v, want nil", err)
	}
	if err := validateModel(cat.DefaultModel, cat); err != nil {
		t.Fatalf("default model: err = %v, want nil", err)
	}
	err = validateModel("not-in-the-catalog", cat)
	if err == nil {
		t.Fatal("unknown model: want error")
	}
	if !strings.Contains(err.Error(), "not-in-the-catalog") {
		t.Fatalf("error %q does not point at the rejected value", err)
	}
}

func TestDoctorRoutingLines(t *testing.T) {
	disabled := false
	cfg := &config.Config{Model: "glm-5.3", Router: config.Router{Enabled: &disabled}}
	if out := doctorRoutingLines(cfg); out != "  router engine: jev\n  routing enabled: no\n  model: glm-5.3\n" {
		t.Fatalf("routing lines = %q, want routing disabled with fixed model", out)
	}
	cfg.Router.Enabled = nil
	cfg.Model = ""
	if out := doctorRoutingLines(cfg); out != "  router engine: jev\n  routing enabled: yes\n  model: auto\n" {
		t.Fatalf("routing lines = %q, want routing enabled with auto model", out)
	}
	cfg.Router.Engine = router.EngineLaya
	if out := doctorRoutingLines(cfg); out != "  router engine: laya\n  routing enabled: yes\n  model: auto\n" {
		t.Fatalf("routing lines = %q, want the laya engine", out)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(r)
		done <- string(data)
	}()
	fn()
	w.Close()
	os.Stdout = old
	return <-done
}

func jevWitnessServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"model":"jev-latest","answers":{"model_choice":{"type":"choice","choice":"dev","confidence":0.87,"probabilities":{"dev":0.87,"sales":0.13}}}}`)
	}))
	t.Cleanup(srv.Close)
	return srv, &requests
}

func doctorFixture(t *testing.T) (*catalog.Catalog, *telemetry.Store, *kspec.Store) {
	t.Helper()
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	store := telemetry.New(filepath.Join(t.TempDir(), "telemetry.json"))
	return cat, store, doctorStoreAt(t, t.TempDir())
}

func TestRunDoctorJevDisabledSkipsProbe(t *testing.T) {
	srv, requests := jevWitnessServer(t)
	cat, store, kspecStore := doctorFixture(t)
	disabled := false
	cfg := &config.Config{
		Model:    cat.DefaultModel,
		Router:   config.Router{Enabled: &disabled},
		Typesafe: config.Typesafe{APIKey: "witness-key", Enabled: &disabled},
	}

	out := captureStdout(t, func() { runDoctor(cfg, cat, store, kspecStore, srv.URL) })

	if !strings.Contains(out, "routing enabled: no") {
		t.Fatalf("output missing 'routing enabled: no':\n%s", out)
	}
	if !strings.Contains(out, "model: "+cat.DefaultModel) {
		t.Fatalf("output missing 'model: %s':\n%s", cat.DefaultModel, out)
	}
	if !strings.Contains(out, "router: disabled") {
		t.Fatalf("output missing 'router: disabled':\n%s", out)
	}
	if n := requests.Load(); n != 0 {
		t.Fatalf("typesafe endpoint received %d requests, want 0", n)
	}
}

func TestRunDoctorJevEnabledProbesEndpoint(t *testing.T) {
	srv, requests := jevWitnessServer(t)
	cat, store, kspecStore := doctorFixture(t)
	enabled := true
	cfg := &config.Config{
		Router:   config.Router{Enabled: &enabled},
		Typesafe: config.Typesafe{APIKey: "witness-key", Enabled: &enabled},
	}

	out := captureStdout(t, func() { runDoctor(cfg, cat, store, kspecStore, srv.URL) })

	if !strings.Contains(out, "routing enabled: yes") {
		t.Fatalf("output missing 'routing enabled: yes':\n%s", out)
	}
	if !strings.Contains(out, "model: auto") {
		t.Fatalf("output missing 'model: auto':\n%s", out)
	}
	if !strings.Contains(out, "jev: reachable") {
		t.Fatalf("output missing 'jev: reachable':\n%s", out)
	}
	if n := requests.Load(); n != 1 {
		t.Fatalf("typesafe endpoint received %d requests, want 1", n)
	}

	cfg.Model = cat.DefaultModel
	out = captureStdout(t, func() { runDoctor(cfg, cat, store, kspecStore, srv.URL) })
	if !strings.Contains(out, "model: "+cat.DefaultModel) {
		t.Fatalf("output missing 'model: %s':\n%s", cat.DefaultModel, out)
	}
	if n := requests.Load(); n != 2 {
		t.Fatalf("typesafe endpoint received %d requests, want 2", n)
	}
}

func TestRunDoctorLayaEngineSkipsJevProbe(t *testing.T) {
	srv, requests := jevWitnessServer(t)
	cat, store, kspecStore := doctorFixture(t)
	enabled := true
	cfg := &config.Config{
		Router:   config.Router{Engine: router.EngineLaya, Enabled: &enabled},
		Typesafe: config.Typesafe{APIKey: "witness-key", Enabled: &enabled},
	}

	out := captureStdout(t, func() { runDoctor(cfg, cat, store, kspecStore, srv.URL) })

	if !strings.Contains(out, "router engine: laya") {
		t.Fatalf("output missing 'router engine: laya':\n%s", out)
	}
	if !strings.Contains(out, "jev: skipped — active engine is laya") {
		t.Fatalf("output missing the jev skip line:\n%s", out)
	}
	if n := requests.Load(); n != 0 {
		t.Fatalf("typesafe endpoint received %d requests, want 0 with laya active", n)
	}
}
