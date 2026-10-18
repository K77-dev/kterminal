package laya

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

var ErrArtifactsMissing = errors.New("laya artifacts missing")

const (
	QuestionTypeChoice = "choice"
	QuestionTypeScore  = "score"
	QuestionTypeNoul   = "noul"
)

type Question struct {
	Type         string
	Instructions string
	Criteria     map[string]string
}

type Answer struct {
	Type          string
	Choice        string
	Confidence    float64
	Probabilities map[string]float64
}

type Status struct {
	Ready   bool
	Missing []string
	Corrupt []string
	Dir     string
}

type Progress struct {
	Artifact string
	Done     int64
	Total    int64
	Err      error
	Finished bool
}

type Engine interface {
	Predict(ctx context.Context, state string, questions map[string]Question) (map[string]Answer, error)
	Status() Status
	EnsureReady(ctx context.Context, progress func(Progress)) error
}

func Dir() string {
	if base := os.Getenv("XDG_CONFIG_HOME"); base != "" {
		return filepath.Join(base, "kterminal", "laya")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".kterminal", "laya")
	}
	return filepath.Join(home, ".config", "kterminal", "laya")
}
