package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Event struct {
	TS            time.Time          `json:"ts"`
	Type          string             `json:"type"`
	Model         string             `json:"model,omitempty"`
	Router        string             `json:"router,omitempty"`
	Confidence    float64            `json:"confidence,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Content       string             `json:"content,omitempty"`
	Tool          string             `json:"tool,omitempty"`
	Args          string             `json:"args,omitempty"`
	Result        string             `json:"result,omitempty"`
	Cost          float64            `json:"cost,omitempty"`
	TPS           float64            `json:"tps,omitempty"`
	Error         string             `json:"error,omitempty"`
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

func (w *Writer) Close() error {
	if w == nil || w.file == nil {
		return nil
	}
	return w.file.Close()
}
