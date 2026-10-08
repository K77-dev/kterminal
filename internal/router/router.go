package router

import (
	"context"
	"fmt"
	"math"
	"strings"

	"kterminal/internal/catalog"
	"kterminal/internal/jev"
)

type Decision struct {
	Model         string
	Confidence    float64
	Probabilities map[string]float64
	Router        string
	Reason        string
}

type Router interface {
	Route(ctx context.Context, state string, candidates []catalog.Model) (Decision, error)
}

func MinConfidence(candidates int) float64 {
	if candidates < 1 {
		candidates = 1
	}
	return math.Min(0.5, math.Max(0.3, 2.0/float64(candidates)))
}

const instructions = "Which LLM should handle this development step? Weigh quality, cost and tokens-per-second according to what the step needs: hard reasoning or complex refactors justify expensive strong models; routine tool steps, lookups and simple edits should use fast cheap models. Avoid overkill."

type JevRouter struct {
	Client *jev.Client
}

func NewJev(client *jev.Client) *JevRouter {
	return &JevRouter{Client: client}
}

func (r *JevRouter) Route(ctx context.Context, state string, candidates []catalog.Model) (Decision, error) {
	if len(candidates) == 0 {
		return Decision{}, fmt.Errorf("no candidate models")
	}
	if len(candidates) == 1 {
		return Decision{Model: candidates[0].Name, Confidence: 1, Router: "jev", Reason: "single candidate"}, nil
	}
	criteria := map[string]string{}
	for _, m := range candidates {
		criteria[m.Name] = m.Criteria()
	}
	choice, confidence, probs, err := r.Client.Decide(ctx, state, instructions, criteria)
	if err != nil {
		return Decision{}, err
	}
	valid := false
	for _, m := range candidates {
		if m.Name == choice {
			valid = true
			break
		}
	}
	if !valid {
		return Decision{}, fmt.Errorf("jev chose unknown model %q", choice)
	}
	return Decision{Model: choice, Confidence: confidence, Probabilities: probs, Router: "jev"}, nil
}

type HeuristicRouter struct {
	Default string
}

func (r *HeuristicRouter) Route(_ context.Context, state string, candidates []catalog.Model) (Decision, error) {
	if len(candidates) == 0 {
		return Decision{}, fmt.Errorf("no candidate models")
	}
	byName := map[string]catalog.Model{}
	for _, m := range candidates {
		byName[m.Name] = m
	}
	pick := func(name string) (Decision, bool) {
		if m, ok := byName[name]; ok {
			return Decision{Model: m.Name, Confidence: 0.4, Router: "heuristic", Reason: "keyword match"}, true
		}
		return Decision{}, false
	}
	s := strings.ToLower(state)
	hard := []string{"architect", "design", "refactor", "complex", "tricky", "race", "deadlock", "algorithm", "debug", "why", "root cause"}
	routine := []string{"list", "read", "run", "test", "format", "rename", "simple", "quick", "what is", "explain"}
	for _, kw := range hard {
		if strings.Contains(s, kw) {
			for _, name := range []string{"glm-5.3", "deepseek-v4-pro"} {
				if d, ok := pick(name); ok {
					d.Reason = "keyword: " + kw
					return d, nil
				}
			}
		}
	}
	for _, kw := range routine {
		if strings.Contains(s, kw) {
			for _, name := range []string{"deepseek-v4.1-flash", "deepseek-v4-flash", "glm-5.1"} {
				if d, ok := pick(name); ok {
					d.Reason = "heuristic: " + kw
					return d, nil
				}
			}
		}
	}
	if d, ok := pick(r.Default); ok {
		d.Reason = "default"
		return d, nil
	}
	return Decision{Model: candidates[0].Name, Confidence: 0.3, Router: "heuristic", Reason: "first candidate"}, nil
}
