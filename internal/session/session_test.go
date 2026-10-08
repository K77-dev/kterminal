package session

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kterminal/internal/llm"
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
		if err := w.WriteSnapshot(turn, ""); err != nil {
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
	if err := w.WriteSnapshot(stateA, ""); err != nil {
		t.Fatalf("write snapshot A: %v", err)
	}
	w.Write(Event{Type: "assistant", Content: "answer B"})
	if err := w.WriteSnapshot(stateB, ""); err != nil {
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
	if err := w.WriteSnapshot(first, ""); err != nil {
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
	if err := aw.WriteSnapshot(second, ""); err != nil {
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
	if err := w.WriteSnapshot(messages, ""); err != nil {
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
	if err := w.WriteSnapshot(messages, ""); err != nil {
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
	if err := w.WriteSnapshot(messages, "kspec-prd"); err != nil {
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
	if err := w.WriteSnapshot([]llm.Message{{Role: "user", Content: "hi"}}, ""); err != nil {
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
