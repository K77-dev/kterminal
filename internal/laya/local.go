package laya

import (
	"context"
	"strings"
)

// LocalEngine is a heuristic-based laya.Engine that runs in-process with no
// external dependencies (no ONNX, no checkpoints, no network). It uses the
// same keyword matching strategy as router.HeuristicRouter (router.go).
//
// When the ONNX engine (module 002) is implemented, it can replace LocalEngine
// as a drop-in laya.Engine — the wiring in main.go changes from laya.NewLocal()
// to laya.NewOnnx() with zero changes to the rest of the system.
type LocalEngine struct{}

func NewLocal() *LocalEngine { return &LocalEngine{} }

func (e *LocalEngine) Predict(_ context.Context, state string, questions map[string]Question) (map[string]Answer, error) {
	answers := make(map[string]Answer, len(questions))
	for key, q := range questions {
		if q.Type != QuestionTypeChoice || len(q.Criteria) == 0 {
			continue
		}
		choice, confidence := scoreChoice(state, q.Criteria)
		answers[key] = Answer{Type: q.Type, Choice: choice, Confidence: confidence}
	}
	return answers, nil
}

func (e *LocalEngine) Status() Status { return Status{Ready: true, Dir: Dir()} }

func (e *LocalEngine) EnsureReady(_ context.Context, _ func(Progress)) error { return nil }

// hardKeywords and routineKeywords mirror the lists in HeuristicRouter (router.go).
// Keep them in sync until the ONNX engine replaces LocalEngine.
var hardKeywords = []string{
	"architect", "design", "refactor", "complex", "tricky", "race",
	"deadlock", "algorithm", "debug", "why", "root cause",
}

var routineKeywords = []string{
	"list", "read", "run", "test", "format", "rename",
	"simple", "quick", "what is", "explain",
}

func scoreChoice(state string, criteria map[string]string) (string, float64) {
	s := strings.ToLower(state)

	isHard := false
	for _, kw := range hardKeywords {
		if strings.Contains(s, kw) {
			isHard = true
			break
		}
	}
	isRoutine := false
	for _, kw := range routineKeywords {
		if strings.Contains(s, kw) {
			isRoutine = true
			break
		}
	}

	best := ""
	bestScore := -1.0
	for name := range criteria {
		cl := strings.ToLower(name)
		score := 0.1 // baseline so every candidate has a non-zero score
		if isHard {
			if strings.Contains(cl, "pro") || strings.Contains(cl, "5.3") || strings.Contains(cl, "large") {
				score += 2.0
			}
		}
		if isRoutine {
			if strings.Contains(cl, "flash") || strings.Contains(cl, "fast") || strings.Contains(cl, "mini") {
				score += 2.0
			}
		}
		if score > bestScore {
			bestScore = score
			best = name
		}
	}

	if best == "" {
		for name := range criteria {
			best = name
			break
		}
	}
	return best, 0.5
}
