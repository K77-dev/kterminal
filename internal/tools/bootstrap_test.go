package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKspecBootstrapToolContract(t *testing.T) {
	reg := NewRegistry()
	var params map[string]any
	found := false
	for _, def := range reg.Definitions() {
		if def.Function.Name != KspecBootstrapToolName {
			continue
		}
		found = true
		params = def.Function.Parameters
	}
	if !found {
		t.Fatalf("definitions missing %s", KspecBootstrapToolName)
	}
	if !reg.IsMutating(KspecBootstrapToolName) {
		t.Fatal("kspec_bootstrap must be mutating — subject to the --confirm gate")
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatalf("parameters = %+v, want properties object", params)
	}
	force, ok := props["force"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %+v, want a force argument", props)
	}
	if force["type"] != "boolean" {
		t.Errorf("force type = %v, want boolean", force["type"])
	}
	if _, ok := params["required"]; ok {
		t.Error("force must be optional")
	}
}

func TestKspecBootstrapToolMaterializesIntoCwd(t *testing.T) {
	t.Chdir(t.TempDir())
	reg := NewRegistry()
	res, err := reg.Execute(context.Background(), KspecBootstrapToolName, `{}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, want := range []string{"bootstrapped kspec into", ".agents/", "VERSION", "spec/tasks/"} {
		if !strings.Contains(res.Output, want) {
			t.Errorf("Output = %q, want it to mention %q", res.Output, want)
		}
	}
	for _, p := range []string{
		filepath.Join(".agents", "skills", "kspec-prd", "SKILL.md"),
		filepath.Join(".agents", "templates", "prd-template.md"),
		filepath.Join(".agents", "rules", "code-standards.md"),
		"VERSION",
		filepath.Join("spec", "tasks"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing %s: %v", p, err)
		}
	}
}

func TestKspecBootstrapToolGuardAndForce(t *testing.T) {
	t.Chdir(t.TempDir())
	custom := filepath.Join(".agents", "skills", "mine", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(custom), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(custom, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry()

	_, err := reg.Execute(context.Background(), KspecBootstrapToolName, `{}`)
	if err == nil {
		t.Fatal("expected guard error over existing .agents")
	}
	if !strings.Contains(err.Error(), "force") {
		t.Errorf("err = %q, want it to mention force", err.Error())
	}
	if _, err := os.Stat(filepath.Join(".agents", "skills", "kspec-prd", "SKILL.md")); err == nil {
		t.Error("guard must not write kspec files")
	}

	res, err := reg.Execute(context.Background(), KspecBootstrapToolName, `{"force": true}`)
	if err != nil {
		t.Fatalf("Execute force: %v", err)
	}
	if !strings.Contains(res.Output, "bootstrapped kspec into") {
		t.Errorf("Output = %q, want bootstrap confirmation", res.Output)
	}
	if _, err := os.Stat(filepath.Join(".agents", "skills", "kspec-prd", "SKILL.md")); err != nil {
		t.Errorf("missing bootstrapped kspec-prd: %v", err)
	}
	if _, err := os.Stat(custom); err != nil {
		t.Errorf("force removed a non-kspec custom skill: %v", err)
	}
}

func TestKspecBootstrapToolRejectsNonBooleanForce(t *testing.T) {
	t.Chdir(t.TempDir())
	reg := NewRegistry()
	for _, args := range []string{`{"force": "yes"}`, `{"force": 1}`, `{"force": null}`, `{"force": []}`} {
		_, err := reg.Execute(context.Background(), KspecBootstrapToolName, args)
		if err == nil {
			t.Fatalf("Execute(%s): expected validation error, got nil", args)
		}
		if !strings.Contains(err.Error(), "must be a boolean") {
			t.Errorf("Execute(%s) err = %q, want boolean validation error", args, err.Error())
		}
	}
	if _, err := os.Stat(".agents"); err == nil {
		t.Error("invalid force must not write anything")
	}
}

func TestKspecBootstrapToolGuardRefusesDivergentRootVersion(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("VERSION", []byte("0.0.1-custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry()

	_, err := reg.Execute(context.Background(), KspecBootstrapToolName, `{}`)
	if err == nil {
		t.Fatal("expected guard error over divergent root VERSION")
	}
	if !strings.Contains(err.Error(), "force") {
		t.Errorf("err = %q, want it to mention force", err.Error())
	}
	data, err := os.ReadFile("VERSION")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "0.0.1-custom\n" {
		t.Errorf("guard overwrote divergent VERSION: %q", data)
	}
	if _, err := os.Stat(".agents"); err == nil {
		t.Error("guard must fail before writing anything")
	}

	res, err := reg.Execute(context.Background(), KspecBootstrapToolName, `{"force": true}`)
	if err != nil {
		t.Fatalf("Execute force: %v", err)
	}
	if !strings.Contains(res.Output, "bootstrapped kspec into") {
		t.Errorf("Output = %q, want bootstrap confirmation", res.Output)
	}
}
