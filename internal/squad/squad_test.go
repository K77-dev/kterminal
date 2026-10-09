package squad

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadReturnsSevenPersonas(t *testing.T) {
	s := Load()
	personas := s.List()
	if len(personas) != 7 {
		t.Fatalf("expected 7 personas, got %d", len(personas))
	}
	expected := map[string]bool{
		"architect": true,
		"backend":   true,
		"frontend":  true,
		"database":  true,
		"ux":        true,
		"qa":        true,
		"test":      true,
	}
	for _, p := range personas {
		if !expected[p.Name] {
			t.Errorf("unexpected persona %q", p.Name)
		}
	}
}

func TestParseFrontmatterAllPersonas(t *testing.T) {
	s := Load()
	personas := s.List()
	if len(personas) != 7 {
		t.Fatalf("expected 7 personas, got %d", len(personas))
	}
	for _, p := range personas {
		if p.Name == "" {
			t.Errorf("persona has empty name")
		}
		if p.Discipline == "" {
			t.Errorf("persona %q has empty discipline", p.Name)
		}
		if p.Prompt == "" {
			t.Errorf("persona %q has empty prompt", p.Name)
		}
	}
}

func TestResolveArchitect(t *testing.T) {
	s := Load()
	p, err := s.Resolve("architect")
	if err != nil {
		t.Fatalf("Resolve architect: %v", err)
	}
	if p.Name != "architect" {
		t.Errorf("expected name architect, got %q", p.Name)
	}
	if p.Discipline != "architecture" {
		t.Errorf("expected discipline architecture, got %q", p.Discipline)
	}
	if len(p.ModelTags) != 1 || p.ModelTags[0] != "reasoning" {
		t.Errorf("expected ModelTags [reasoning], got %v", p.ModelTags)
	}
	if p.Prompt == "" {
		t.Errorf("expected non-empty prompt")
	}
}

func TestResolveBackend(t *testing.T) {
	s := Load()
	p, err := s.Resolve("backend")
	if err != nil {
		t.Fatalf("Resolve backend: %v", err)
	}
	if p.Name != "backend" {
		t.Errorf("expected name backend, got %q", p.Name)
	}
	if p.Discipline != "backend" {
		t.Errorf("expected discipline backend, got %q", p.Discipline)
	}
}

func TestResolveAllPersonas(t *testing.T) {
	s := Load()
	names := []string{"architect", "backend", "frontend", "database", "ux", "qa", "test"}
	for _, name := range names {
		p, err := s.Resolve(name)
		if err != nil {
			t.Errorf("Resolve %q: %v", name, err)
			continue
		}
		if p.Name != name {
			t.Errorf("expected name %q, got %q", name, p.Name)
		}
	}
}

func TestResolveUnknownReturnsError(t *testing.T) {
	s := Load()
	_, err := s.Resolve("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent persona")
	}
}

func TestMaestroPromptNotEmpty(t *testing.T) {
	s := Load()
	prompt := s.MaestroPrompt()
	if prompt == "" {
		t.Fatal("expected non-empty maestro prompt")
	}
}

func TestProjectOverrideEmbedded(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, agentsDir, personaDir, "architect")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: architect\ndiscipline: overridden\nmodel-tags: [fast]\nrules: [custom]\n---\nOverridden prompt body.\n"
	if err := os.WriteFile(filepath.Join(agentDir, skillFile), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(orig) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	s := Load()
	p, err := s.Resolve("architect")
	if err != nil {
		t.Fatalf("Resolve architect: %v", err)
	}
	if p.Discipline != "overridden" {
		t.Errorf("expected overridden discipline, got %q", p.Discipline)
	}
	if len(p.ModelTags) != 1 || p.ModelTags[0] != "fast" {
		t.Errorf("expected ModelTags [fast], got %v", p.ModelTags)
	}
	if p.Prompt != "Overridden prompt body." {
		t.Errorf("expected overridden prompt, got %q", p.Prompt)
	}
}

func TestProjectNewPersonaInList(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, agentsDir, personaDir, "custom")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: custom\ndiscipline: custom-discipline\nmodel-tags: [fast]\nrules: []\n---\nCustom persona prompt.\n"
	if err := os.WriteFile(filepath.Join(agentDir, skillFile), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(orig) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	s := Load()
	personas := s.List()
	found := false
	for _, p := range personas {
		if p.Name == "custom" {
			found = true
			if p.Discipline != "custom-discipline" {
				t.Errorf("expected custom-discipline, got %q", p.Discipline)
			}
		}
	}
	if !found {
		t.Fatal("custom persona not found in List()")
	}
	if len(personas) != 8 {
		t.Errorf("expected 8 personas (7 embedded + 1 custom), got %d", len(personas))
	}
}

func TestValidNameRejectsPathTraversal(t *testing.T) {
	invalid := []string{"", ".", "..", "../etc", "/etc/passwd", `C:\windows`, "a/b", "a\\b", "foo..bar"}
	for _, name := range invalid {
		if validName(name) {
			t.Errorf("expected %q to be invalid", name)
		}
	}
}

func TestValidNameAcceptsValid(t *testing.T) {
	valid := []string{"architect", "backend", "qa", "test", "my-custom-persona", "persona123"}
	for _, name := range valid {
		if !validName(name) {
			t.Errorf("expected %q to be valid", name)
		}
	}
}

func TestValidateKickoffRejectsUnknownRoles(t *testing.T) {
	s := Load()
	limits := DefaultLimits()
	k := Kickoff{Roles: []string{"architect", "nonexistent"}, MaxConvocations: 4, TokenBudget: 100000}
	_, err := ValidateKickoff(k, s, limits)
	if err == nil {
		t.Fatal("expected error for unknown role")
	}
}

func TestValidateKickoffRejectsEmptyRoles(t *testing.T) {
	s := Load()
	limits := DefaultLimits()
	k := Kickoff{Roles: []string{}, MaxConvocations: 4, TokenBudget: 100000}
	_, err := ValidateKickoff(k, s, limits)
	if err == nil {
		t.Fatal("expected error for empty roles")
	}
}

func TestValidateKickoffClampsMaxConvocations(t *testing.T) {
	s := Load()
	limits := DefaultLimits()
	k := Kickoff{Roles: []string{"architect"}, MaxConvocations: 100, TokenBudget: 100000}
	result, err := ValidateKickoff(k, s, limits)
	if err != nil {
		t.Fatalf("ValidateKickoff: %v", err)
	}
	if result.MaxConvocations != limits.MaxConvocations {
		t.Errorf("expected MaxConvocations clamped to %d, got %d", limits.MaxConvocations, result.MaxConvocations)
	}
}

func TestValidateKickoffKeepsTokenBudgetAboveLimit(t *testing.T) {
	s := Load()
	limits := DefaultLimits()
	k := Kickoff{Roles: []string{"architect"}, MaxConvocations: 4, TokenBudget: 999999999}
	result, err := ValidateKickoff(k, s, limits)
	if err != nil {
		t.Fatalf("ValidateKickoff: %v", err)
	}
	if result.TokenBudget != 999999999 {
		t.Errorf("expected TokenBudget preserved as declared (advisory), got %d", result.TokenBudget)
	}
}

func TestValidateKickoffDefaultsWhenZero(t *testing.T) {
	s := Load()
	limits := DefaultLimits()
	k := Kickoff{Roles: []string{"architect"}, MaxConvocations: 0, TokenBudget: 0}
	result, err := ValidateKickoff(k, s, limits)
	if err != nil {
		t.Fatalf("ValidateKickoff: %v", err)
	}
	if result.MaxConvocations != limits.MaxConvocations {
		t.Errorf("expected default MaxConvocations %d, got %d", limits.MaxConvocations, result.MaxConvocations)
	}
	if result.TokenBudget != limits.TokenBudget {
		t.Errorf("expected default TokenBudget %d, got %d", limits.TokenBudget, result.TokenBudget)
	}
	if result.ExitCriterion == "" {
		t.Errorf("expected non-empty default exit criterion")
	}
}

func TestValidateKickoffAcceptsValidWithinLimits(t *testing.T) {
	s := Load()
	limits := DefaultLimits()
	k := Kickoff{
		Roles:           []string{"architect", "backend", "qa"},
		MaxConvocations: 5,
		TokenBudget:     150000,
		ExitCriterion:   "all personas agree",
	}
	result, err := ValidateKickoff(k, s, limits)
	if err != nil {
		t.Fatalf("ValidateKickoff: %v", err)
	}
	if result.MaxConvocations != 5 {
		t.Errorf("expected MaxConvocations 5, got %d", result.MaxConvocations)
	}
	if result.TokenBudget != 150000 {
		t.Errorf("expected TokenBudget 150000, got %d", result.TokenBudget)
	}
	if result.ExitCriterion != "all personas agree" {
		t.Errorf("expected exit criterion preserved, got %q", result.ExitCriterion)
	}
}

func TestSplitFrontmatter(t *testing.T) {
	data := []byte("---\nname: test\ndiscipline: testing\nmodel-tags: [fast]\nrules: []\n---\nBody text here.\n")
	block, body, err := splitFrontmatter(data)
	if err != nil {
		t.Fatalf("splitFrontmatter: %v", err)
	}
	if string(block) != "name: test\ndiscipline: testing\nmodel-tags: [fast]\nrules: []" {
		t.Errorf("unexpected block: %q", string(block))
	}
	if strings.TrimSpace(string(body)) != "Body text here." {
		t.Errorf("unexpected body: %q", string(body))
	}
}

func TestParseFrontmatterMissingDelimiter(t *testing.T) {
	data := []byte("no frontmatter here")
	_, err := parseFrontmatter(data)
	if err == nil {
		t.Fatal("expected error for missing delimiter")
	}
}

func TestParseFrontmatterMissingName(t *testing.T) {
	data := []byte("---\ndiscipline: testing\n---\nBody.\n")
	_, err := parseFrontmatter(data)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestPersonaDisciplinesIncludesDisciplineAndRules(t *testing.T) {
	p := Persona{Name: "architect", Discipline: "architecture", Rules: []string{"go", "code-standards"}}
	got := PersonaDisciplines(p)
	want := map[string]bool{"architecture": true, "go": true, "code-standards": true}
	if len(got) != 3 {
		t.Fatalf("PersonaDisciplines = %v, want 3 entries", got)
	}
	for _, d := range got {
		if !want[d] {
			t.Errorf("unexpected discipline %q", d)
		}
	}
}

func TestMatchesAnyIntersects(t *testing.T) {
	if !MatchesAny([]string{"architecture", "go"}, []string{"backend", "architecture"}) {
		t.Fatal("expected an intersection")
	}
	if MatchesAny([]string{"architecture"}, []string{"backend", "database"}) {
		t.Fatal("expected no intersection")
	}
	if MatchesAny([]string{"architecture"}, nil) {
		t.Fatal("empty rule disciplines must never match")
	}
}

func TestEmbeddedRulesWithoutDisciplinesExcluded(t *testing.T) {
	for _, p := range Load().List() {
		if len(p.Rules) == 0 && p.Discipline == "" {
			t.Fatalf("persona %q has no discipline or rules", p.Name)
		}
	}
}
