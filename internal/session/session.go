package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"kterminal/internal/llm"
)

type AttachmentMeta struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type Event struct {
	TS            time.Time          `json:"ts"`
	Type          string             `json:"type"`
	Depth         int                `json:"depth,omitempty"`
	Model         string             `json:"model,omitempty"`
	Router        string             `json:"router,omitempty"`
	Confidence    float64            `json:"confidence,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Reason        string             `json:"reason,omitempty"`
	Content       string             `json:"content,omitempty"`
	Attachments   []AttachmentMeta   `json:"attachments,omitempty"`
	Tool          string             `json:"tool,omitempty"`
	Args          string             `json:"args,omitempty"`
	Result        string             `json:"result,omitempty"`
	Diff          []string           `json:"diff,omitempty"`
	Cost          float64            `json:"cost,omitempty"`
	TPS           float64            `json:"tps,omitempty"`
	Error         string             `json:"error,omitempty"`
	Skill         string             `json:"skill,omitempty"`
	Source        string             `json:"source,omitempty"`
	TokensBefore  int64              `json:"tokens_before,omitempty"`
	TokensAfter   int64              `json:"tokens_after,omitempty"`
	Messages      json.RawMessage    `json:"messages,omitempty"`
}

func Dir() string {
	if base := os.Getenv("XDG_DATA_HOME"); base != "" {
		return filepath.Join(base, "kterminal", "sessions")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".kterminal", "sessions")
	}
	return filepath.Join(home, ".local", "share", "kterminal", "sessions")
}

type Writer struct {
	path string
	file *os.File
}

func NewWriter() (*Writer, error) {
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(Dir(), time.Now().Format("20060102-150405")+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	return &Writer{path: path, file: f}, nil
}

func (w *Writer) Path() string { return w.path }

func (w *Writer) Write(e Event) {
	if w == nil || w.file == nil {
		return
	}
	e.TS = time.Now()
	data, err := json.Marshal(e)
	if err != nil {
		return
	}
	w.file.Write(append(data, '\n'))
}

const omittedImageURL = "[omitted]"

type Snapshot struct {
	Messages []llm.Message
	Skill    string
}

func (w *Writer) WriteSnapshot(messages []llm.Message, skill string) error {
	raw, err := json.Marshal(snapshotMessages(messages))
	if err != nil {
		return err
	}
	w.Write(Event{Type: "snapshot", Messages: raw, Skill: skill})
	return nil
}

func snapshotMessages(messages []llm.Message) []llm.Message {
	out := append([]llm.Message(nil), messages...)
	for i := range out {
		out[i].ContentParts = omitImageParts(out[i].ContentParts)
	}
	return out
}

func omitImageParts(parts []llm.ContentPart) []llm.ContentPart {
	hasImages := false
	for _, p := range parts {
		if p.Type == "image_url" {
			hasImages = true
			break
		}
	}
	if !hasImages {
		return parts
	}
	out := make([]llm.ContentPart, len(parts))
	for i, p := range parts {
		if p.Type == "image_url" {
			out[i] = llm.ContentPart{Type: "image_url", ImageURL: &llm.ImageURL{URL: omittedImageURL}}
			continue
		}
		out[i] = p
	}
	return out
}

func (w *Writer) Close() error {
	if w == nil || w.file == nil {
		return nil
	}
	return w.file.Close()
}

func AppendWriter(path string) (*Writer, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	return &Writer{path: path, file: f}, nil
}

func Load(path string) (Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, err
	}
	lines := strings.Split(string(data), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		if ev.Type != "snapshot" {
			continue
		}
		var messages []llm.Message
		if err := json.Unmarshal(ev.Messages, &messages); err != nil {
			return Snapshot{}, fmt.Errorf("snapshot inválido em %s: %w", path, err)
		}
		return Snapshot{Messages: messages, Skill: ev.Skill}, nil
	}
	return Snapshot{}, errors.New("sessão sem snapshot")
}

var ErrNoSessions = errors.New("sem sessões anteriores")

func LoadLatest() (string, Snapshot, error) {
	entries, err := os.ReadDir(Dir())
	if err != nil {
		if os.IsNotExist(err) {
			return "", Snapshot{}, ErrNoSessions
		}
		return "", Snapshot{}, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		names = append(names, entry.Name())
	}
	if len(names) == 0 {
		return "", Snapshot{}, ErrNoSessions
	}
	sort.Strings(names)
	path := filepath.Join(Dir(), names[len(names)-1])
	snap, err := Load(path)
	if err != nil {
		return "", Snapshot{}, err
	}
	return path, snap, nil
}
