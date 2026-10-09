package session

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"kterminal/internal/llm"
	"kterminal/internal/squad"
)

func newTestWriter(t *testing.T) *Writer {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	w, err := NewWriter()
	if err != nil {
		t.Fatalf("new writer: %v", err)
	}
	t.Cleanup(func() { w.Close() })
	return w
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}

func assertMessagesEqual(t *testing.T, got, want []llm.Message) {
	t.Helper()
	gotJSON := mustJSON(t, got)
	wantJSON := mustJSON(t, want)
	if !bytes.Equal(gotJSON, wantJSON) {
		t.Fatalf("messages mismatch: got %s want %s", gotJSON, wantJSON)
	}
}

func TestSnapshotRoundtrip(t *testing.T) {
	w := newTestWriter(t)
	toolCall := llm.ToolCall{
		ID:       "call_1",
		Type:     "function",
		Function: llm.FuncCall{Name: "bash", Arguments: `{"cmd":"ls"}`},
	}
	turn1 := []llm.Message{{Role: "user", Content: "list the files"}}
	turn2 := []llm.Message{
		{Role: "user", Content: "list the files"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{toolCall}},
	}
	turn3 := []llm.Message{
		{Role: "user", Content: "list the files"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{toolCall}},
		{Role: "tool", ToolCallID: "call_1", Content: "file1\nfile2"},
	}
	for _, turn := range [][]llm.Message{turn1, turn2, turn3} {
		if err := w.WriteSnapshot(turn, "", "", nil); err != nil {
			t.Fatalf("write snapshot: %v", err)
		}
	}
	data, err := os.ReadFile(w.Path())
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d transcript lines, want 3", len(lines))
	}
	var ev Event
	if err := json.Unmarshal([]byte(lines[2]), &ev); err != nil {
		t.Fatalf("unmarshal snapshot event: %v", err)
	}
	if ev.Type != "snapshot" {
		t.Fatalf("got event type %q, want snapshot", ev.Type)
	}
	if ev.TS.IsZero() {
		t.Fatalf("snapshot event has no timestamp")
	}
	got, err := Load(w.Path())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got.Messages) != len(turn3) {
		t.Fatalf("got %d messages, want %d", len(got.Messages), len(turn3))
	}
	if got.Messages[1].ToolCalls[0].ID != "call_1" || got.Messages[2].ToolCallID != "call_1" {
		t.Fatalf("tool call pair did not survive roundtrip: %+v", got.Messages)
	}
	assertMessagesEqual(t, got.Messages, turn3)
}

func TestLoadUsesLastSnapshot(t *testing.T) {
	w := newTestWriter(t)
	stateA := []llm.Message{{Role: "user", Content: "state A"}}
	stateB := []llm.Message{
		{Role: "user", Content: "state B"},
		{Role: "assistant", Content: "answer B"},
	}
	w.Write(Event{Type: "user", Content: "state A"})
	if err := w.WriteSnapshot(stateA, "", "", nil); err != nil {
		t.Fatalf("write snapshot A: %v", err)
	}
	w.Write(Event{Type: "assistant", Content: "answer B"})
	if err := w.WriteSnapshot(stateB, "", "", nil); err != nil {
		t.Fatalf("write snapshot B: %v", err)
	}
	got, err := Load(w.Path())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	assertMessagesEqual(t, got.Messages, stateB)
}

func TestLoadWithoutSnapshot(t *testing.T) {
	w := newTestWriter(t)
	w.Write(Event{Type: "user", Content: "hello"})
	w.Write(Event{Type: "assistant", Content: "hi"})
	if _, err := Load(w.Path()); err == nil || err.Error() != "sessão sem snapshot" {
		t.Fatalf("got error %v, want 'sessão sem snapshot'", err)
	}

	empty := filepath.Join(t.TempDir(), "empty.jsonl")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatalf("write empty file: %v", err)
	}
	if _, err := Load(empty); err == nil || err.Error() != "sessão sem snapshot" {
		t.Fatalf("got error %v, want 'sessão sem snapshot'", err)
	}

	if _, err := Load(filepath.Join(t.TempDir(), "missing.jsonl")); err == nil {
		t.Fatalf("want error for missing file")
	}
}

func TestLoadLatestPicksNewest(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}
	sessions := []struct {
		name    string
		content string
	}{
		{"20240101-000000.jsonl", `{"role":"user","content":"session one"}`},
		{"20240102-000000.jsonl", `{"role":"user","content":"session two"}`},
		{"20240103-000000.jsonl", `{"role":"user","content":"session three"}`},
	}
	for _, s := range sessions {
		line := `{"type":"snapshot","messages":[` + s.content + `]}` + "\n"
		if err := os.WriteFile(filepath.Join(Dir(), s.name), []byte(line), 0o600); err != nil {
			t.Fatalf("write session file: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(Dir(), "notes.txt"), []byte("not a session"), 0o600); err != nil {
		t.Fatalf("write distractor: %v", err)
	}
	if err := os.Mkdir(filepath.Join(Dir(), "folder.jsonl"), 0o755); err != nil {
		t.Fatalf("mkdir distractor: %v", err)
	}

	path, snap, err := LoadLatest()
	if err != nil {
		t.Fatalf("load latest: %v", err)
	}
	wantPath := filepath.Join(Dir(), "20240103-000000.jsonl")
	if path != wantPath {
		t.Fatalf("got path %s, want %s", path, wantPath)
	}
	if len(snap.Messages) != 1 || snap.Messages[0].Content != "session three" {
		t.Fatalf("got messages %+v, want session three", snap.Messages)
	}

	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "empty"))
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		t.Fatalf("mkdir empty sessions: %v", err)
	}
	if _, _, err := LoadLatest(); !errors.Is(err, ErrNoSessions) {
		t.Fatalf("got error %v, want ErrNoSessions", err)
	}

	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "missing"))
	if _, _, err := LoadLatest(); !errors.Is(err, ErrNoSessions) {
		t.Fatalf("got error %v, want ErrNoSessions for missing dir", err)
	}
}

func TestAppendWriterAppends(t *testing.T) {
	w := newTestWriter(t)
	first := []llm.Message{{Role: "user", Content: "first turn"}}
	if err := w.WriteSnapshot(first, "", "", nil); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	original, err := os.ReadFile(w.Path())
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	aw, err := AppendWriter(w.Path())
	if err != nil {
		t.Fatalf("append writer: %v", err)
	}
	if aw.Path() != w.Path() {
		t.Fatalf("got path %s, want %s", aw.Path(), w.Path())
	}
	second := []llm.Message{
		{Role: "user", Content: "first turn"},
		{Role: "assistant", Content: "second turn"},
	}
	if err := aw.WriteSnapshot(second, "", "", nil); err != nil {
		t.Fatalf("append snapshot: %v", err)
	}
	if err := aw.Close(); err != nil {
		t.Fatalf("close append writer: %v", err)
	}

	data, err := os.ReadFile(w.Path())
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	if !bytes.HasPrefix(data, original) {
		t.Fatalf("original content not preserved: %s", data)
	}
	got, err := Load(w.Path())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	assertMessagesEqual(t, got.Messages, second)

	if _, err := AppendWriter(filepath.Join(t.TempDir(), "missing.jsonl")); err == nil {
		t.Fatalf("want error for missing file")
	}
}

func TestLoadSkipsPartialLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "crashed.jsonl")
	snapshot := []llm.Message{{Role: "user", Content: "before crash"}}
	line := `{"type":"snapshot","messages":` + string(mustJSON(t, snapshot)) + `}` + "\n"
	partial := `{"type":"snapshot","mess`
	if err := os.WriteFile(path, []byte(line+partial), 0o600); err != nil {
		t.Fatalf("write crashed transcript: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	assertMessagesEqual(t, got.Messages, snapshot)
}

func TestLoadRejectsCorruptSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.jsonl")
	line := `{"type":"snapshot","messages":{"role":"user"}}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		t.Fatalf("write corrupt transcript: %v", err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "snapshot inválido") {
		t.Fatalf("got error %v, want corrupt snapshot error", err)
	}
}

func TestWriteSnapshotOmitsImageParts(t *testing.T) {
	w := newTestWriter(t)
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("secret image bytes"))
	messages := []llm.Message{{
		Role:    "user",
		Content: "look at this",
		ContentParts: []llm.ContentPart{
			{Type: "text", Text: "look at this"},
			{Type: "image_url", ImageURL: &llm.ImageURL{URL: uri}},
		},
	}}
	if err := w.WriteSnapshot(messages, "", "", nil); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	data, err := os.ReadFile(w.Path())
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	if strings.Contains(string(data), "base64,") {
		t.Fatalf("snapshot leaked base64 image data: %s", data)
	}
	want := `"content":[{"type":"text","text":"look at this"},{"type":"image_url","image_url":{"url":"[omitted]"}}]`
	if !strings.Contains(string(data), want) {
		t.Fatalf("snapshot missing image placeholder:\nwant %s\ngot  %s", want, data)
	}
	if messages[0].ContentParts[1].ImageURL.URL != uri {
		t.Fatalf("WriteSnapshot must not mutate the live message, got %q", messages[0].ContentParts[1].ImageURL.URL)
	}
}

func TestWriteSnapshotKeepsTextParts(t *testing.T) {
	w := newTestWriter(t)
	messages := []llm.Message{{
		Role:    "user",
		Content: "plain",
		ContentParts: []llm.ContentPart{
			{Type: "text", Text: "plain"},
			{Type: "text", Text: "extra"},
		},
	}}
	if err := w.WriteSnapshot(messages, "", "", nil); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	got, err := Load(w.Path())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got.Messages) != 1 || len(got.Messages[0].ContentParts) != 2 {
		t.Fatalf("loaded message = %+v, want text parts preserved", got.Messages)
	}
	if got.Messages[0].ContentParts[0].Text != "plain" || got.Messages[0].ContentParts[1].Text != "extra" {
		t.Fatalf("text parts = %+v, want unchanged", got.Messages[0].ContentParts)
	}
}

func TestSnapshotCarriesSkill(t *testing.T) {
	w := newTestWriter(t)
	messages := []llm.Message{
		{Role: "user", Content: "run the prd flow"},
		{Role: "assistant", Content: "done"},
	}
	if err := w.WriteSnapshot(messages, "kspec-prd", "", nil); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	snap, err := Load(w.Path())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if snap.Skill != "kspec-prd" {
		t.Fatalf("snapshot skill = %q, want kspec-prd", snap.Skill)
	}
	assertMessagesEqual(t, snap.Messages, messages)
}

func TestSnapshotOmitsEmptySkill(t *testing.T) {
	w := newTestWriter(t)
	if err := w.WriteSnapshot([]llm.Message{{Role: "user", Content: "hi"}}, "", "", nil); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	data, err := os.ReadFile(w.Path())
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	if strings.Contains(string(data), `"skill"`) {
		t.Fatalf("snapshot with empty skill leaked the field: %s", data)
	}
	snap, err := Load(w.Path())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if snap.Skill != "" {
		t.Fatalf("snapshot skill = %q, want empty", snap.Skill)
	}
}

func TestSkillActivatedEventCarriesNameAndSource(t *testing.T) {
	w := newTestWriter(t)
	w.Write(Event{Type: "skill_activated", Skill: "kspec-qa", Source: "project"})
	data, err := os.ReadFile(w.Path())
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	var ev Event
	if err := json.Unmarshal([]byte(strings.SplitN(strings.TrimSpace(string(data)), "\n", 2)[0]), &ev); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	if ev.Type != "skill_activated" || ev.Skill != "kspec-qa" || ev.Source != "project" {
		t.Fatalf("event = %+v, want skill_activated with skill and source", ev)
	}
}

func TestEventAgentSerializesWithOmitEmpty(t *testing.T) {
	w := newTestWriter(t)
	w.Write(Event{Type: "assistant", Content: "design ready", Agent: "architect"})
	w.Write(Event{Type: "assistant", Content: "maestro synthesis"})

	data, err := os.ReadFile(w.Path())
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")

	var ev1, ev2 Event
	if err := json.Unmarshal([]byte(lines[0]), &ev1); err != nil {
		t.Fatalf("unmarshal event 1: %v", err)
	}
	if ev1.Agent != "architect" {
		t.Fatalf("event 1 agent = %q, want architect", ev1.Agent)
	}
	if !strings.Contains(lines[0], `"agent":"architect"`) {
		t.Fatalf("event 1 JSON missing agent field: %s", lines[0])
	}

	if err := json.Unmarshal([]byte(lines[1]), &ev2); err != nil {
		t.Fatalf("unmarshal event 2: %v", err)
	}
	if ev2.Agent != "" {
		t.Fatalf("event 2 agent = %q, want empty", ev2.Agent)
	}
	if strings.Contains(lines[1], `"agent"`) {
		t.Fatalf("event 2 JSON should omit agent field: %s", lines[1])
	}
}

func TestSnapshotCarriesMode(t *testing.T) {
	w := newTestWriter(t)
	messages := []llm.Message{
		{Role: "user", Content: "design the API"},
		{Role: "assistant", Content: "squad plan ready"},
	}
	if err := w.WriteSnapshot(messages, "", "squad", nil); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	snap, err := Load(w.Path())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if snap.Mode != "squad" {
		t.Fatalf("snapshot mode = %q, want squad", snap.Mode)
	}
	assertMessagesEqual(t, snap.Messages, messages)
}

func TestSnapshotOmitsEmptyMode(t *testing.T) {
	w := newTestWriter(t)
	if err := w.WriteSnapshot([]llm.Message{{Role: "user", Content: "hi"}}, "", "", nil); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	data, err := os.ReadFile(w.Path())
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	if strings.Contains(string(data), `"mode"`) {
		t.Fatalf("snapshot with empty mode leaked the field: %s", data)
	}
}

func TestLoadOldJSONLWithoutAgentOrMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old.jsonl")
	oldLine := `{"type":"snapshot","messages":[{"role":"user","content":"legacy session"}]}` + "\n"
	if err := os.WriteFile(path, []byte(oldLine), 0o600); err != nil {
		t.Fatalf("write old transcript: %v", err)
	}
	snap, err := Load(path)
	if err != nil {
		t.Fatalf("load old JSONL: %v", err)
	}
	if snap.Mode != "" {
		t.Fatalf("old snapshot mode = %q, want empty", snap.Mode)
	}
	if len(snap.Messages) != 1 || snap.Messages[0].Content != "legacy session" {
		t.Fatalf("old snapshot messages = %+v, want legacy session", snap.Messages)
	}
}

func populatedMesa() *squad.Mesa {
	m := &squad.Mesa{}
	m.Reset(
		squad.Kickoff{Roles: []string{"architect", "backend", "qa"}, MaxConvocations: 6, TokenBudget: 150000},
		map[string]string{"architect": "architecture", "backend": "backend", "qa": "quality"},
	)
	m.AddConvocation()
	m.AddConvocation()
	m.AddTokens(4200)
	m.StartDeliberation("architect")
	m.ObservePersona("architect", "model-a", 900, 0.5)
	m.FinishDeliberation("architect")
	m.StartDeliberation("backend")
	m.ObservePersona("backend", "model-b", 300, 0.75)
	return m
}

func assertMesaEqual(t *testing.T, got, want *squad.Mesa) {
	t.Helper()
	if !reflect.DeepEqual(got.Roles, want.Roles) {
		t.Errorf("mesa roles = %v, want %v", got.Roles, want.Roles)
	}
	if got.MaxConvocations != want.MaxConvocations {
		t.Errorf("mesa max_convocations = %d, want %d", got.MaxConvocations, want.MaxConvocations)
	}
	if got.TokenBudget != want.TokenBudget {
		t.Errorf("mesa token_budget = %d, want %d", got.TokenBudget, want.TokenBudget)
	}
	if got.Convocations != want.Convocations {
		t.Errorf("mesa convocations = %d, want %d", got.Convocations, want.Convocations)
	}
	if got.Tokens != want.Tokens {
		t.Errorf("mesa tokens = %d, want %d", got.Tokens, want.Tokens)
	}
	if !reflect.DeepEqual(got.Entries, want.Entries) {
		t.Errorf("mesa entries = %+v, want %+v", got.Entries, want.Entries)
	}
}

func TestSnapshotRoundtripWithMesa(t *testing.T) {
	w := newTestWriter(t)
	messages := []llm.Message{
		{Role: "user", Content: "design the API"},
		{Role: "assistant", Content: "squad plan ready"},
	}
	mesa := populatedMesa()
	if err := w.WriteSnapshot(messages, "", "squad", mesa); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	snap, err := Load(w.Path())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if snap.Mesa == nil {
		t.Fatalf("snapshot mesa = nil, want restored mesa")
	}
	assertMesaEqual(t, snap.Mesa, mesa)
	if snap.Mode != "squad" {
		t.Fatalf("snapshot mode = %q, want squad", snap.Mode)
	}
	assertMessagesEqual(t, snap.Messages, messages)
}

func TestSnapshotOmitsNilMesa(t *testing.T) {
	w := newTestWriter(t)
	if err := w.WriteSnapshot([]llm.Message{{Role: "user", Content: "hi"}}, "", "", nil); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	data, err := os.ReadFile(w.Path())
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	if strings.Contains(string(data), `"mesa"`) {
		t.Fatalf("snapshot with nil mesa leaked the field: %s", data)
	}
	snap, err := Load(w.Path())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if snap.Mesa != nil {
		t.Fatalf("snapshot mesa = %+v, want nil", snap.Mesa)
	}
}

func TestLoadOldJSONLWithoutMesa(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old.jsonl")
	oldLine := `{"type":"snapshot","messages":[{"role":"user","content":"legacy session"}],"skill":"kspec-prd","mode":"squad"}` + "\n"
	if err := os.WriteFile(path, []byte(oldLine), 0o600); err != nil {
		t.Fatalf("write old transcript: %v", err)
	}
	snap, err := Load(path)
	if err != nil {
		t.Fatalf("load old JSONL: %v", err)
	}
	if snap.Mesa != nil {
		t.Fatalf("old snapshot mesa = %+v, want nil", snap.Mesa)
	}
	if snap.Skill != "kspec-prd" || snap.Mode != "squad" {
		t.Fatalf("old snapshot skill/mode = %q/%q, want kspec-prd/squad", snap.Skill, snap.Mode)
	}
	if len(snap.Messages) != 1 || snap.Messages[0].Content != "legacy session" {
		t.Fatalf("old snapshot messages = %+v, want legacy session", snap.Messages)
	}
}

func TestResumeLoadLatestTranscriptWithMesa(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}
	mesa := populatedMesa()
	mesaRaw, err := json.Marshal(mesa)
	if err != nil {
		t.Fatalf("marshal mesa: %v", err)
	}
	messagesRaw := string(mustJSON(t, []llm.Message{
		{Role: "user", Content: "design the API"},
		{Role: "assistant", Content: "architect contribution"},
	}))
	lines := []string{
		`{"type":"user","content":"design the API","mode":"squad"}`,
		`{"type":"assistant","content":"plan ready","agent":"architect","mode":"squad"}`,
		`{"type":"snapshot","messages":` + messagesRaw + `,"skill":"","mode":"squad","mesa":` + string(mesaRaw) + `}`,
		`{"type":"user","content":"follow up after snapshot"}`,
	}
	path := filepath.Join(Dir(), "20260101-120000.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	resumedPath, snap, err := LoadLatest()
	if err != nil {
		t.Fatalf("load latest: %v", err)
	}
	if resumedPath != path {
		t.Fatalf("resumed path = %s, want %s", resumedPath, path)
	}
	if snap.Mode != "squad" {
		t.Fatalf("resumed mode = %q, want squad", snap.Mode)
	}
	if snap.Mesa == nil {
		t.Fatalf("resumed mesa = nil, want restored mesa")
	}
	assertMesaEqual(t, snap.Mesa, mesa)
	if len(snap.Messages) != 2 || snap.Messages[1].Content != "architect contribution" {
		t.Fatalf("resumed messages = %+v, want snapshot messages", snap.Messages)
	}
}

func TestResumeLoadLatestPreFeatureSnapshotWithoutMesa(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}
	oldLine := `{"type":"snapshot","messages":[{"role":"user","content":"legacy squad session"}],"skill":"","mode":"squad"}` + "\n"
	path := filepath.Join(Dir(), "20240101-000000.jsonl")
	if err := os.WriteFile(path, []byte(oldLine), 0o600); err != nil {
		t.Fatalf("write pre-feature transcript: %v", err)
	}

	gotPath, snap, err := LoadLatest()
	if err != nil {
		t.Fatalf("load latest pre-feature snapshot: %v", err)
	}
	if gotPath != path {
		t.Fatalf("resumed path = %s, want %s", gotPath, path)
	}
	if snap.Mode != "squad" {
		t.Fatalf("pre-feature snapshot mode = %q, want squad", snap.Mode)
	}
	if snap.Mesa != nil {
		t.Fatalf("pre-feature snapshot mesa = %+v, want nil", snap.Mesa)
	}
	if len(snap.Messages) != 1 || snap.Messages[0].Content != "legacy squad session" {
		t.Fatalf("pre-feature snapshot messages = %+v, want the legacy session", snap.Messages)
	}
}
