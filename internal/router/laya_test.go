package router

import (
	"context"
	"errors"
	"testing"

	"kterminal/internal/laya"
)

func TestLayaRouterNoCandidates(t *testing.T) {
	r := &LayaRouter{Engine: &fakeLayaEngine{}}
	if _, err := r.Route(context.Background(), "state", nil); err == nil {
		t.Fatal("want error with zero candidates")
	}
}

func TestLayaRouterSingleCandidateSkipsInference(t *testing.T) {
	cat := fixedTestCatalog(t)
	engine := &fakeLayaEngine{}
	r := &LayaRouter{Engine: engine}
	d, err := r.Route(context.Background(), "state", cat.Models[:1])
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if d.Model != cat.Models[0].Name || d.Confidence != 1 || d.Router != "laya" || d.Reason != "single candidate" {
		t.Fatalf("decision = %+v, want direct single candidate", d)
	}
}

func TestLayaRouterChoice(t *testing.T) {
	cat := fixedTestCatalog(t)
	engine := &fakeLayaEngine{answers: map[string]laya.Answer{
		"model_choice": {Type: "choice", Choice: "vision-pro", Confidence: 0.9, Probabilities: map[string]float64{"vision-pro": 0.9}},
	}}
	r := &LayaRouter{Engine: engine}
	d, err := r.Route(context.Background(), "hard refactor", cat.Models)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if d.Model != "vision-pro" || d.Router != "laya" || d.Confidence != 0.9 {
		t.Fatalf("decision = %+v, want vision-pro via laya", d)
	}
	if d.Probabilities["vision-pro"] != 0.9 {
		t.Fatalf("probabilities = %v, want passthrough", d.Probabilities)
	}
}

func TestLayaRouterPropagatesInferenceError(t *testing.T) {
	cat := fixedTestCatalog(t)
	engine := &fakeLayaEngine{err: errors.New("onnx session closed")}
	r := &LayaRouter{Engine: engine}
	if _, err := r.Route(context.Background(), "state", cat.Models); err == nil {
		t.Fatal("inference error must propagate (agent falls back to heuristic)")
	}
}

func TestLayaRouterRejectsUnknownChoice(t *testing.T) {
	cat := fixedTestCatalog(t)
	engine := &fakeLayaEngine{answers: map[string]laya.Answer{
		"model_choice": {Type: "choice", Choice: "not-a-model"},
	}}
	r := &LayaRouter{Engine: engine}
	_, err := r.Route(context.Background(), "state", cat.Models)
	if err == nil || err.Error() != `laya chose unknown model "not-a-model"` {
		t.Fatalf("err = %v, want unknown model rejection", err)
	}
}

func TestLayaRouterRejectsEmptyChoice(t *testing.T) {
	cat := fixedTestCatalog(t)
	engine := &fakeLayaEngine{answers: map[string]laya.Answer{
		"model_choice": {Type: "choice", Choice: ""},
	}}
	r := &LayaRouter{Engine: engine}
	if _, err := r.Route(context.Background(), "state", cat.Models); err == nil {
		t.Fatal("empty choice must error")
	}
}

func TestLayaRouterRejectsMissingAnswer(t *testing.T) {
	cat := fixedTestCatalog(t)
	r := &LayaRouter{Engine: &fakeLayaEngine{answers: map[string]laya.Answer{}}}
	if _, err := r.Route(context.Background(), "state", cat.Models); err == nil {
		t.Fatal("missing model_choice answer must error")
	}
}

func TestLayaRouterSendsJevQuestionShape(t *testing.T) {
	cat := fixedTestCatalog(t)
	var got map[string]laya.Question
	var gotState string
	engine := &capturingLaya{capture: func(state string, questions map[string]laya.Question) {
		gotState = state
		got = questions
	}, answers: map[string]laya.Answer{
		"model_choice": {Choice: cat.Models[0].Name},
	}}
	r := &LayaRouter{Engine: engine}
	if _, err := r.Route(context.Background(), "the state", cat.Models); err != nil {
		t.Fatalf("route: %v", err)
	}
	if gotState != "the state" {
		t.Fatalf("state = %q, want passthrough", gotState)
	}
	q, ok := got["model_choice"]
	if !ok {
		t.Fatal("question key must be model_choice")
	}
	if q.Type != laya.QuestionTypeChoice {
		t.Fatalf("question type = %q, want choice", q.Type)
	}
	if q.Instructions != instructions {
		t.Fatalf("instructions differ from the jev instructions:\n%q", q.Instructions)
	}
	for _, m := range cat.Models {
		if q.Criteria[m.Name] != m.Criteria() {
			t.Fatalf("criteria for %s must match catalog.Criteria()", m.Name)
		}
	}
}

type capturingLaya struct {
	capture func(string, map[string]laya.Question)
	answers map[string]laya.Answer
}

func (c *capturingLaya) Predict(_ context.Context, state string, questions map[string]laya.Question) (map[string]laya.Answer, error) {
	c.capture(state, questions)
	return c.answers, nil
}

func (c *capturingLaya) Status() laya.Status { return laya.Status{Ready: true} }

func (c *capturingLaya) EnsureReady(context.Context, func(laya.Progress)) error { return nil }
