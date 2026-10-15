package router

import (
	"context"
	"testing"
)

func TestWireJevActiveReturnsJevAndHeuristic(t *testing.T) {
	cat := fixedTestCatalog(t)
	primary, fallback := Wire("ts-key", true, "fast-model", cat)
	if _, ok := primary.(*JevRouter); !ok {
		t.Fatalf("primary = %T, want *JevRouter", primary)
	}
	h, ok := fallback.(*HeuristicRouter)
	if !ok {
		t.Fatalf("fallback = %T, want *HeuristicRouter", fallback)
	}
	if h.Default != cat.DefaultModel {
		t.Fatalf("fallback default = %q, want %q", h.Default, cat.DefaultModel)
	}
}

func TestWireJevDisabledNeverReturnsJevRouter(t *testing.T) {
	cat := fixedTestCatalog(t)
	primary, fallback := Wire("ts-key", false, "fast-model", cat)
	if _, ok := primary.(*JevRouter); ok {
		t.Fatal("primary is *JevRouter with jev disabled — must never happen")
	}
	fr, ok := primary.(*FixedRouter)
	if !ok {
		t.Fatalf("primary = %T, want *FixedRouter", primary)
	}
	if fr.Model != "fast-model" {
		t.Fatalf("model = %q, want fast-model", fr.Model)
	}
	if fr.Default != cat.DefaultModel {
		t.Fatalf("default = %q, want %q", fr.Default, cat.DefaultModel)
	}
	if fallback != nil {
		t.Fatalf("fallback = %T, want nil", fallback)
	}
}

func TestWireWithoutKeyReturnsFixed(t *testing.T) {
	cat := fixedTestCatalog(t)
	for _, enabled := range []bool{true, false} {
		primary, fallback := Wire("", enabled, "fast-model", cat)
		if _, ok := primary.(*JevRouter); ok {
			t.Fatalf("enabled = %v: primary is *JevRouter without api key", enabled)
		}
		if _, ok := primary.(*FixedRouter); !ok {
			t.Fatalf("enabled = %v: primary = %T, want *FixedRouter", enabled, primary)
		}
		if fallback != nil {
			t.Fatalf("enabled = %v: fallback = %T, want nil", enabled, fallback)
		}
	}
}

func TestWireEmptyModelRoutesToCatalogDefault(t *testing.T) {
	cat := fixedTestCatalog(t)
	primary, _ := Wire("", false, "", cat)
	fr, ok := primary.(*FixedRouter)
	if !ok {
		t.Fatalf("primary = %T, want *FixedRouter", primary)
	}
	d, err := fr.Route(context.Background(), "any step", cat.Models)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if d.Model != cat.DefaultModel {
		t.Fatalf("model = %q, want catalog default %q", d.Model, cat.DefaultModel)
	}
}
