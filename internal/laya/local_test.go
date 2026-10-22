package laya

import (
	"context"
	"testing"
)

func TestLocalEnginePredictReturnsChoice(t *testing.T) {
	e := NewLocal()
	questions := map[string]Question{
		"model_choice": {
			Type:     QuestionTypeChoice,
			Criteria: map[string]string{"glm-5.3": "strong reasoning", "deepseek-v4-flash": "fast cheap"},
		},
	}
	answers, err := e.Predict(context.Background(), "refactor this complex algorithm", questions)
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	ans, ok := answers["model_choice"]
	if !ok {
		t.Fatal("missing model_choice answer")
	}
	if ans.Choice == "" {
		t.Fatal("Choice is empty")
	}
	if ans.Choice != "glm-5.3" && ans.Choice != "deepseek-v4-flash" {
		t.Fatalf("Choice = %q, not in criteria", ans.Choice)
	}
}

func TestLocalEnginePredictHardKeywordPrefersStrongModel(t *testing.T) {
	e := NewLocal()
	questions := map[string]Question{
		"model_choice": {
			Type:     QuestionTypeChoice,
			Criteria: map[string]string{"glm-5.3": "strong", "deepseek-v4-flash": "fast"},
		},
	}
	answers, err := e.Predict(context.Background(), "debug this tricky race condition", questions)
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	if answers["model_choice"].Choice != "glm-5.3" {
		t.Fatalf("expected glm-5.3 for hard keyword, got %q", answers["model_choice"].Choice)
	}
}

func TestLocalEnginePredictRoutineKeywordPrefersFlashModel(t *testing.T) {
	e := NewLocal()
	questions := map[string]Question{
		"model_choice": {
			Type:     QuestionTypeChoice,
			Criteria: map[string]string{"glm-5.3": "strong", "deepseek-v4-flash": "fast"},
		},
	}
	answers, err := e.Predict(context.Background(), "list the files and read the config", questions)
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	if answers["model_choice"].Choice != "deepseek-v4-flash" {
		t.Fatalf("expected deepseek-v4-flash for routine keyword, got %q", answers["model_choice"].Choice)
	}
}

func TestLocalEnginePredictEmptyState(t *testing.T) {
	e := NewLocal()
	questions := map[string]Question{
		"model_choice": {
			Type:     QuestionTypeChoice,
			Criteria: map[string]string{"model-a": "a", "model-b": "b"},
		},
	}
	answers, err := e.Predict(context.Background(), "", questions)
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	if answers["model_choice"].Choice == "" {
		t.Fatal("Choice is empty for empty state")
	}
}

func TestLocalEnginePredictSkipsNonChoiceQuestions(t *testing.T) {
	e := NewLocal()
	questions := map[string]Question{
		"score_q": {
			Type:     QuestionTypeScore,
			Criteria: map[string]string{"a": "a"},
		},
	}
	answers, err := e.Predict(context.Background(), "test", questions)
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	if _, ok := answers["score_q"]; ok {
		t.Fatal("non-choice question should be skipped")
	}
}

func TestLocalEngineStatusReady(t *testing.T) {
	e := NewLocal()
	st := e.Status()
	if !st.Ready {
		t.Fatal("Status().Ready = false, want true")
	}
}

func TestLocalEngineEnsureReadyNoop(t *testing.T) {
	e := NewLocal()
	if err := e.EnsureReady(context.Background(), nil); err != nil {
		t.Fatalf("EnsureReady: %v", err)
	}
}
