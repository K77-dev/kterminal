package router

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"kterminal/internal/laya"
)

type fakeLayaEngine struct {
	answers map[string]laya.Answer
	err     error
}

func (f *fakeLayaEngine) Predict(_ context.Context, _ string, _ map[string]laya.Question) (map[string]laya.Answer, error) {
	return f.answers, f.err
}

func (f *fakeLayaEngine) Status() laya.Status { return laya.Status{Ready: true} }

func (f *fakeLayaEngine) EnsureReady(context.Context, func(laya.Progress)) error { return nil }

func TestWireWithTable(t *testing.T) {
	cat := fixedTestCatalog(t)
	engine := &fakeLayaEngine{}
	tests := []struct {
		name     string
		opts     Options
		wantPrim string
		wantFall string
		wantErr  error
	}{
		{
			name:     "disabled any engine yields fixed",
			opts:     Options{Engine: EngineJev, Enabled: false, JevAPIKey: "k", JevEnabled: true, FixedModel: "fast-model"},
			wantPrim: "*router.FixedRouter",
			wantFall: "nil",
		},
		{
			name:     "jev with key yields jev and heuristic",
			opts:     Options{Engine: EngineJev, Enabled: true, JevAPIKey: "k", JevEnabled: true},
			wantPrim: "*router.JevRouter",
			wantFall: "*router.HeuristicRouter",
		},
		{
			name:     "jev without key yields fixed",
			opts:     Options{Engine: EngineJev, Enabled: true, JevEnabled: true},
			wantPrim: "*router.FixedRouter",
			wantFall: "nil",
		},
		{
			name:     "empty engine behaves as jev",
			opts:     Options{Enabled: true, JevAPIKey: "k", JevEnabled: true},
			wantPrim: "*router.JevRouter",
			wantFall: "*router.HeuristicRouter",
		},
		{
			name:     "laya with engine yields laya and heuristic",
			opts:     Options{Engine: EngineLaya, Enabled: true, Laya: engine},
			wantPrim: "*router.LayaRouter",
			wantFall: "*router.HeuristicRouter",
		},
		{
			name:    "laya without engine errors",
			opts:    Options{Engine: EngineLaya, Enabled: true},
			wantErr: ErrEngineUnavailable,
		},
		{
			name:    "unknown engine errors",
			opts:    Options{Engine: "grok", Enabled: true},
			wantErr: errors.New("unknown router engine"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			primary, fallback, err := WireWith(tt.opts, cat)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("err = nil, want %v", tt.wantErr)
				}
				if tt.wantErr != ErrEngineUnavailable && !strings.Contains(err.Error(), tt.wantErr.Error()) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if got := typeName(primary); got != tt.wantPrim {
				t.Fatalf("primary = %s, want %s", got, tt.wantPrim)
			}
			gotFall := "nil"
			if fallback != nil {
				gotFall = typeName(fallback)
			}
			if gotFall != tt.wantFall {
				t.Fatalf("fallback = %s, want %s", gotFall, tt.wantFall)
			}
		})
	}
}

func typeName(v any) string {
	return fmt.Sprintf("%T", v)
}

func TestWireWithLayaLocalEngine(t *testing.T) {
	cat := fixedTestCatalog(t)
	primary, fallback, err := WireWith(Options{Engine: EngineLaya, Enabled: true, Laya: laya.NewLocal()}, cat)
	if err != nil {
		t.Fatalf("WireWith: %v", err)
	}
	if typeName(primary) != "*router.LayaRouter" {
		t.Fatalf("primary = %s, want *router.LayaRouter", typeName(primary))
	}
	if typeName(fallback) != "*router.HeuristicRouter" {
		t.Fatalf("fallback = %s, want *router.HeuristicRouter", typeName(fallback))
	}
}

func TestWireWithLayaFallbackDefault(t *testing.T) {
	cat := fixedTestCatalog(t)
	_, fallback, err := WireWith(Options{Engine: EngineLaya, Enabled: true, Laya: &fakeLayaEngine{}}, cat)
	if err != nil {
		t.Fatalf("WireWith: %v", err)
	}
	h, ok := fallback.(*HeuristicRouter)
	if !ok {
		t.Fatalf("fallback = %T, want *HeuristicRouter", fallback)
	}
	if h.Default != cat.DefaultModel {
		t.Fatalf("fallback default = %q, want %q", h.Default, cat.DefaultModel)
	}
}
