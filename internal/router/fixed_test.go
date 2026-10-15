package router

import (
	"context"
	"testing"

	"kterminal/internal/catalog"
)

func fixedTestCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	cat, err := catalog.Parse([]byte(`
models:
  - name: fast-model
    vision: false
  - name: balanced-model
    vision: false
  - name: vision-lite
    vision: true
  - name: vision-pro
    vision: true
default_model: balanced-model
`))
	if err != nil {
		t.Fatalf("parse test catalog: %v", err)
	}
	return cat
}

func TestFixedRouterPicksConfiguredModel(t *testing.T) {
	cat := fixedTestCatalog(t)
	r := &FixedRouter{Model: "fast-model", Default: cat.DefaultModel}
	d, err := r.Route(context.Background(), "complex refactor with a tricky race", cat.Models)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if d.Model != "fast-model" {
		t.Fatalf("model = %q, want fast-model", d.Model)
	}
	if d.Confidence != 1 {
		t.Fatalf("confidence = %v, want 1", d.Confidence)
	}
	if d.Router != "fixed" {
		t.Fatalf("router = %q, want fixed", d.Router)
	}
}

func TestFixedRouterDegradesToDefault(t *testing.T) {
	cat := fixedTestCatalog(t)
	r := &FixedRouter{Model: "retired-model", Default: cat.DefaultModel}
	d, err := r.Route(context.Background(), "any step", cat.Models)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if d.Model != cat.DefaultModel {
		t.Fatalf("model = %q, want default %q", d.Model, cat.DefaultModel)
	}
	if d.Confidence != 1 {
		t.Fatalf("confidence = %v, want 1", d.Confidence)
	}
	if d.Router != "fixed" {
		t.Fatalf("router = %q, want fixed", d.Router)
	}
}

func TestFixedRouterDegradesToFirstCandidate(t *testing.T) {
	cat := fixedTestCatalog(t)
	r := &FixedRouter{Model: "retired-model", Default: "also-retired"}
	d, err := r.Route(context.Background(), "any step", cat.Models)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if d.Model != cat.Models[0].Name {
		t.Fatalf("model = %q, want first candidate %q", d.Model, cat.Models[0].Name)
	}
	if d.Router != "fixed" {
		t.Fatalf("router = %q, want fixed", d.Router)
	}
}

func TestFixedRouterVisionFilterKeepsChainDeterministic(t *testing.T) {
	cat := fixedTestCatalog(t)
	var visionOnly []catalog.Model
	for _, m := range cat.Models {
		if m.Vision {
			visionOnly = append(visionOnly, m)
		}
	}
	if len(visionOnly) != 2 {
		t.Fatalf("vision candidates = %d, want 2", len(visionOnly))
	}
	r := &FixedRouter{Model: "fast-model", Default: cat.DefaultModel}
	d, err := r.Route(context.Background(), "step with image attachments", visionOnly)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if d.Model != visionOnly[0].Name {
		t.Fatalf("model = %q, want first vision candidate %q — model and default are not vision-capable", d.Model, visionOnly[0].Name)
	}
	rd := &FixedRouter{Model: "fast-model", Default: "vision-pro"}
	d2, err := rd.Route(context.Background(), "step with image attachments", visionOnly)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if d2.Model != "vision-pro" {
		t.Fatalf("model = %q, want vision-pro via default branch", d2.Model)
	}
}

func TestFixedRouterDeterministicAcrossStates(t *testing.T) {
	cat := fixedTestCatalog(t)
	r := &FixedRouter{Model: "fast-model", Default: cat.DefaultModel}
	states := []string{"architect a complex system", "list the files", "why does this fail", ""}
	var want string
	for i, state := range states {
		d, err := r.Route(context.Background(), state, cat.Models)
		if err != nil {
			t.Fatalf("route %q: %v", state, err)
		}
		if i == 0 {
			want = d.Model
			continue
		}
		if d.Model != want {
			t.Fatalf("state %q routed to %q, want %q — router must be deterministic", state, d.Model, want)
		}
	}
	if want != "fast-model" {
		t.Fatalf("routed model = %q, want fast-model", want)
	}
}

func TestFixedRouterNoCandidatesError(t *testing.T) {
	r := &FixedRouter{Model: "fast-model", Default: "balanced-model"}
	if _, err := r.Route(context.Background(), "any step", nil); err == nil {
		t.Fatal("route with no candidates: want error")
	}
}
