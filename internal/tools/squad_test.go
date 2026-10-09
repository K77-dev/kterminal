package tools

import (
	"context"
	"testing"

	"kterminal/internal/squad"
)

func defNames(reg *Registry) []string {
	defs := reg.Definitions()
	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Function.Name
	}
	return names
}

func hasName(reg *Registry, name string) bool {
	for _, n := range defNames(reg) {
		if n == name {
			return true
		}
	}
	return false
}

func TestNewReadOnlyRegistryExposesOnlyReadTools(t *testing.T) {
	reg := NewReadOnlyRegistry()

	for _, name := range []string{"read", "glob", "grep"} {
		if !hasName(reg, name) {
			t.Errorf("read-only registry missing %q, has %v", name, defNames(reg))
		}
	}
	for _, name := range []string{"write", "edit", "bash", "kspec_bootstrap"} {
		if hasName(reg, name) {
			t.Errorf("read-only registry must not expose %q, has %v", name, defNames(reg))
		}
	}
	if got := len(defNames(reg)); got != 3 {
		t.Fatalf("read-only registry has %d tools, want 3: %v", got, defNames(reg))
	}
}

func TestNewReadOnlyRegistryNotMutating(t *testing.T) {
	reg := NewReadOnlyRegistry()
	for _, name := range defNames(reg) {
		if reg.IsMutating(name) {
			t.Errorf("tool %q must not be mutating", name)
		}
	}
}

func TestSquadKickoffToolRegistersValidated(t *testing.T) {
	store := squad.Load()
	limits := squad.DefaultLimits()
	var got *squad.Kickoff
	tool := SquadKickoffTool(store, limits, func(k squad.Kickoff) { got = &k })

	res, err := tool.Execute(context.Background(), map[string]any{
		"roles":            []any{"architect", "backend", "qa"},
		"max_convocations": float64(4),
		"token_budget":     float64(100000),
		"exit_criterion":   "all agree",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got == nil {
		t.Fatal("kickoff was not registered")
	}
	if len(got.Roles) != 3 {
		t.Fatalf("roles = %v, want 3", got.Roles)
	}
	if got.MaxConvocations != 4 || got.TokenBudget != 100000 {
		t.Fatalf("limits = %d/%d, want 4/100000", got.MaxConvocations, got.TokenBudget)
	}
	if res.Output == "" {
		t.Fatal("expected output describing the registered mesa")
	}
}

func TestSquadKickoffToolRejectsUnknownRole(t *testing.T) {
	store := squad.Load()
	tool := SquadKickoffTool(store, squad.DefaultLimits(), func(k squad.Kickoff) {
		t.Fatal("kickoff must not be registered on invalid roles")
	})
	_, err := tool.Execute(context.Background(), map[string]any{
		"roles": []any{"architect", "nonexistent"},
	})
	if err == nil {
		t.Fatal("expected error for unknown role")
	}
}

func TestSquadKickoffToolClampsConvocationsKeepsBudget(t *testing.T) {
	store := squad.Load()
	limits := squad.DefaultLimits()
	var got *squad.Kickoff
	tool := SquadKickoffTool(store, limits, func(k squad.Kickoff) { got = &k })

	_, err := tool.Execute(context.Background(), map[string]any{
		"roles":            []any{"architect"},
		"max_convocations": float64(1000),
		"token_budget":     float64(999999999),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.MaxConvocations != limits.MaxConvocations {
		t.Errorf("MaxConvocations = %d, want clamped to %d", got.MaxConvocations, limits.MaxConvocations)
	}
	if got.TokenBudget != 999999999 {
		t.Errorf("TokenBudget = %d, want preserved as declared (advisory)", got.TokenBudget)
	}
}

func TestSquadKickoffToolRejectsMissingRoles(t *testing.T) {
	store := squad.Load()
	tool := SquadKickoffTool(store, squad.DefaultLimits(), func(k squad.Kickoff) {})
	_, err := tool.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing roles")
	}
}
