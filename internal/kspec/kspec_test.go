package kspec

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
)

func TestParseFrontmatterComplete(t *testing.T) {
	src := "---\n" +
		"name: kspec-demo\n" +
		"version: 1.2.3\n" +
		"description: Demo skill.\n" +
		"argument-hint: \"<slug>\"\n" +
		"---\n" +
		"body\n"
	fm, err := parseFrontmatter([]byte(src))
	if err != nil {
		t.Fatalf("parseFrontmatter: %v", err)
	}
	if fm.Name != "kspec-demo" {
		t.Errorf("Name = %q, want %q", fm.Name, "kspec-demo")
	}
	if fm.Version != "1.2.3" {
		t.Errorf("Version = %q, want %q", fm.Version, "1.2.3")
	}
	if fm.Description != "Demo skill." {
		t.Errorf("Description = %q, want %q", fm.Description, "Demo skill.")
	}
	if fm.ArgHint != "<slug>" {
		t.Errorf("ArgHint = %q, want %q", fm.ArgHint, "<slug>")
	}
}

func TestParseFrontmatterMissingArgHint(t *testing.T) {
	src := "---\nname: kspec-demo\nversion: 1.2.3\ndescription: Demo skill.\n---\nbody\n"
	fm, err := parseFrontmatter([]byte(src))
	if err != nil {
		t.Fatalf("parseFrontmatter: %v", err)
	}
	if fm.Name != "kspec-demo" || fm.Version != "1.2.3" || fm.Description != "Demo skill." {
		t.Fatalf("got %+v", fm)
	}
	if fm.ArgHint != "" {
		t.Errorf("ArgHint = %q, want empty", fm.ArgHint)
	}
}

func TestParseFrontmatterErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"no frontmatter", "# Title\n\nbody\n"},
		{"missing closing delimiter", "---\nname: kspec-demo\nversion: 1.2.3\n"},
		{"missing name", "---\nversion: 1.2.3\ndescription: Demo skill.\n---\nbody\n"},
		{"invalid yaml", "---\nname: [unclosed\n---\nbody\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseFrontmatter([]byte(tc.src)); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestListReturnsKspecSkills(t *testing.T) {
	want := []string{
		"kspec-bootstrap",
		"kspec-bugfix",
		"kspec-ideia",
		"kspec-implement",
		"kspec-pr-review",
		"kspec-prd",
		"kspec-qa",
		"kspec-tasks",
		"kspec-techspec",
		"kspec-version",
	}
	got := Load().List()
	names := make([]string, len(got))
	for i, s := range got {
		names[i] = s.Name
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("List() names = %v, want %v", names, want)
	}
	argHints := 0
	for _, s := range got {
		if s.Version == "" {
			t.Errorf("skill %s: empty version", s.Name)
		}
		if s.Description == "" {
			t.Errorf("skill %s: empty description", s.Name)
		}
		if s.ArgHint != "" {
			argHints++
		}
	}
	if argHints == 0 {
		t.Error("no skill has argument-hint parsed")
	}
}

func TestVersionReadsEmbeddedVersion(t *testing.T) {
	v := Load().Version()
	if v == "" {
		t.Fatal("Version() returned empty string")
	}
	if strings.ContainsAny(v, " \t\r\n") {
		t.Fatalf("Version() = %q, want trimmed", v)
	}
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(v) {
		t.Fatalf("Version() = %q, want semver", v)
	}
}

func TestNewStoreRecordsInvalidSkills(t *testing.T) {
	f := fstest.MapFS{
		"VERSION":                        &fstest.MapFile{Data: []byte("1.0.0\n")},
		"skills/kspec-good/SKILL.md":     &fstest.MapFile{Data: []byte("---\nname: kspec-good\nversion: 1.0.0\ndescription: Good.\n---\nbody\n")},
		"skills/kspec-broken/SKILL.md":   &fstest.MapFile{Data: []byte("# no frontmatter\n")},
		"skills/kspec-unparsed/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: [unclosed\n---\nbody\n")},
		"skills/kspec-missing/SKILL.md":  &fstest.MapFile{Data: []byte("---\nversion: 1.0.0\ndescription: Missing name.\n---\nbody\n")},
		"skills/third-party/SKILL.md":    &fstest.MapFile{Data: []byte("---\nname: third-party\n---\nnope\n")},
		"skills/kspec-nofile/other.md":   &fstest.MapFile{Data: []byte("no SKILL.md here\n")},
	}
	s := newStore(f, "")
	list := s.List()
	if len(list) != 1 || list[0].Name != "kspec-good" {
		t.Fatalf("List() = %v, want only kspec-good", list)
	}
	invalid := s.Invalid()
	wantDirs := []string{
		"skills/kspec-broken",
		"skills/kspec-missing",
		"skills/kspec-nofile",
		"skills/kspec-unparsed",
	}
	gotDirs := make([]string, len(invalid))
	for i, inv := range invalid {
		gotDirs[i] = inv.Dir
		if inv.Err == nil {
			t.Errorf("Invalid() entry %s: nil error", inv.Dir)
		}
	}
	if !reflect.DeepEqual(gotDirs, wantDirs) {
		t.Fatalf("Invalid() dirs = %v, want %v", gotDirs, wantDirs)
	}
}

func TestInvalidReturnsCopy(t *testing.T) {
	s := newStore(fstest.MapFS{
		"skills/kspec-broken/SKILL.md": &fstest.MapFile{Data: []byte("no frontmatter\n")},
	}, "")
	invalid := s.Invalid()
	if len(invalid) != 1 {
		t.Fatalf("Invalid() = %v, want 1 entry", invalid)
	}
	invalid[0] = InvalidSkill{}
	if s.Invalid()[0].Dir != "skills/kspec-broken" {
		t.Fatal("Invalid() returned slice shares backing array with store")
	}
}

func TestLoadEmbeddedStoreHasNoInvalidSkills(t *testing.T) {
	for _, inv := range Load().Invalid() {
		t.Errorf("invalid skill %s: %v", inv.Dir, inv.Err)
	}
}

func fixtureKspecRepo(t *testing.T) (dir, branch, sha string) {
	t.Helper()
	dir = t.TempDir()
	agents := filepath.Join(dir, ".agents")
	files := map[string]string{
		filepath.Join(agents, "skills", "kspec-alpha", "SKILL.md"): "---\nname: kspec-alpha\nversion: 1.0.0\ndescription: Alpha.\n---\nalpha\n",
		filepath.Join(agents, "skills", "kspec-beta", "SKILL.md"):  "---\nname: kspec-beta\nversion: 1.0.0\ndescription: Beta.\nargument-hint: \"<slug>\"\n---\nbeta\n",
		filepath.Join(agents, "skills", "third-party", "SKILL.md"): "---\nname: third-party\n---\nnope\n",
		filepath.Join(agents, "skills", "skills-lock.json"):        "{}\n",
		filepath.Join(agents, "templates", "prd-template.md"):      "# PRD template\n",
		filepath.Join(agents, "rules", "code-standards.md"):        "# code standards\n",
		filepath.Join(dir, "VERSION"):                              "9.9.9\n",
	}
	for p, content := range files {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git := func(args ...string) string {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=kspec", "GIT_AUTHOR_EMAIL=kspec@example.com",
			"GIT_COMMITTER_NAME=kspec", "GIT_COMMITTER_EMAIL=kspec@example.com")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q")
	git("add", "-A")
	git("-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "-q", "-m", "fixture")
	branch = git("rev-parse", "--abbrev-ref", "HEAD")
	sha = git("rev-parse", "HEAD")
	return dir, branch, sha
}

func writeSyncScript(t *testing.T) (script, root string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found in PATH")
	}
	root = t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	scriptData, err := os.ReadFile(filepath.Join("..", "..", "scripts", "sync-kspec.sh"))
	if err != nil {
		t.Fatalf("read sync-kspec.sh: %v", err)
	}
	script = filepath.Join(root, "scripts", "sync-kspec.sh")
	if err := os.WriteFile(script, scriptData, 0o755); err != nil {
		t.Fatal(err)
	}
	return script, root
}

func TestSyncScriptDefaultRefMatchesVendoredVersion(t *testing.T) {
	scriptData, err := os.ReadFile(filepath.Join("..", "..", "scripts", "sync-kspec.sh"))
	if err != nil {
		t.Fatalf("read sync-kspec.sh: %v", err)
	}
	vData, err := os.ReadFile(filepath.Join("embed", "VERSION"))
	if err != nil {
		t.Fatalf("read embed/VERSION: %v", err)
	}
	want := "DEFAULT_REF=\"v" + strings.TrimSpace(string(vData)) + "\""
	if !strings.Contains(string(scriptData), want) {
		t.Fatalf("sync-kspec.sh default ref is not pinned to the vendored version %q", want)
	}
}

func TestSyncScriptLayout(t *testing.T) {
	src, branch, sha := fixtureKspecRepo(t)
	script, root := writeSyncScript(t)

	runSync := func(ref string) {
		cmd := exec.Command("bash", script, ref, src)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("sync-kspec.sh %s: %v\n%s", ref, err, out)
		}
	}

	runSync(branch)
	embed := filepath.Join(root, "internal", "kspec", "embed")

	mustExist := []string{
		filepath.Join(embed, "skills", "kspec-alpha", "SKILL.md"),
		filepath.Join(embed, "skills", "kspec-beta", "SKILL.md"),
		filepath.Join(embed, "templates", "prd-template.md"),
		filepath.Join(embed, "rules", "code-standards.md"),
		filepath.Join(embed, "VERSION"),
	}
	for _, p := range mustExist {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing %s: %v", p, err)
		}
	}

	mustAbsent := []string{
		filepath.Join(embed, "skills", "third-party"),
		filepath.Join(embed, "skills", "skills-lock.json"),
		filepath.Join(embed, "skills-lock.json"),
	}
	for _, p := range mustAbsent {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("unexpected %s in embed tree", p)
		}
	}

	vData, err := os.ReadFile(filepath.Join(embed, "VERSION"))
	if err != nil {
		t.Fatalf("read VERSION: %v", err)
	}
	if strings.TrimSpace(string(vData)) != "9.9.9" {
		t.Fatalf("VERSION = %q, want %q", vData, "9.9.9")
	}

	first := treeHash(t, embed)
	runSync(sha)
	second := treeHash(t, embed)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("sync script is not idempotent")
	}
}

func TestSyncScriptUnknownRefFailsClearly(t *testing.T) {
	src, _, _ := fixtureKspecRepo(t)
	script, root := writeSyncScript(t)

	cmd := exec.Command("bash", script, "no-such-ref", src)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("sync-kspec.sh no-such-ref: expected failure, got success\n%s", out)
	}
	combined := string(out)
	if !strings.Contains(combined, "sync-kspec: checkout no-such-ref") {
		t.Fatalf("output missing checkout failure header, got:\n%s", combined)
	}
	if !strings.Contains(combined, "no-such-ref") {
		t.Fatalf("output missing offending ref, got:\n%s", combined)
	}
}

func TestSyncScriptUnreachableSourceReportsPrimaryError(t *testing.T) {
	notARepo := t.TempDir()
	script, root := writeSyncScript(t)

	cmd := exec.Command("bash", script, "v1.5.0", notARepo)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("sync-kspec.sh with unreachable source: expected failure, got success\n%s", out)
	}
	combined := string(out)
	if !strings.Contains(combined, "sync-kspec: clone") {
		t.Fatalf("output missing clone failure header, got:\n%s", combined)
	}
	if !strings.Contains(combined, "does not appear to be a git repository") {
		t.Fatalf("output missing primary git error, got:\n%s", combined)
	}
	if strings.Contains(combined, "sync-kspec: checkout") {
		t.Fatalf("fallback attempted for non-ref failure, got:\n%s", combined)
	}
}

func treeHash(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		out[rel] = fmt.Sprintf("%x", sha256.Sum256(data))
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

func embedSub(t *testing.T) fs.FS {
	t.Helper()
	sub, err := fs.Sub(embedded, "embed")
	if err != nil {
		t.Fatal(err)
	}
	return sub
}

func embeddedStore(t *testing.T) *Store {
	t.Helper()
	return newStore(embedSub(t), "")
}

func storeAt(t *testing.T, dir string) *Store {
	t.Helper()
	return newStore(embedSub(t), dir)
}

func writeProject(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for p, content := range files {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestResolveEmbeddedSkillStripsFrontmatter(t *testing.T) {
	content, err := embeddedStore(t).Resolve("kspec-version")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if strings.Contains(content, "name: kspec-version") {
		t.Error("resolved content still contains frontmatter")
	}
	if !strings.Contains(content, "Exibe a versão atual do kspec") {
		t.Error("resolved content missing skill body")
	}
}

func TestResolveInlinesReferencedTemplates(t *testing.T) {
	content, err := embeddedStore(t).Resolve("kspec-prd")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if strings.Contains(content, "@.agents/templates/prd-template.md") {
		t.Error("template reference not inlined")
	}
	if !strings.Contains(content, "# PRD — [Nome da funcionalidade/produto]") {
		t.Error("template content missing from resolved skill")
	}
}

func TestResolveInlinesTechspecTemplate(t *testing.T) {
	content, err := embeddedStore(t).Resolve("kspec-techspec")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if strings.Contains(content, "@.agents/templates/techspec-template.md") {
		t.Error("techspec template reference not inlined")
	}
	if !strings.Contains(content, "# Template de Especificação Técnica") {
		t.Error("techspec template content missing from resolved skill")
	}
}

func TestResolveUnknownSkill(t *testing.T) {
	_, err := embeddedStore(t).Resolve("kspec-nope")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), `skill "kspec-nope" not found`) {
		t.Errorf("error = %q, want not-found message", err)
	}
}

func TestResolveRejectsInvalidNames(t *testing.T) {
	for _, name := range []string{"", ".", "..", "../kspec-prd", "a/b", `a\b`, "a..b"} {
		if _, err := embeddedStore(t).Resolve(name); err == nil {
			t.Errorf("Resolve(%q): expected error, got nil", name)
		}
	}
}

func TestResolveProjectSkillWinsOverEmbedded(t *testing.T) {
	dir := writeProject(t, map[string]string{
		filepath.Join(".agents", "skills", "kspec-prd", "SKILL.md"): "---\nname: kspec-prd\nversion: 9.9.9\ndescription: Custom PRD.\n---\nPROJECT PRD MARKER\n",
	})
	content, err := storeAt(t, dir).Resolve("kspec-prd")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !strings.Contains(content, "PROJECT PRD MARKER") {
		t.Error("project skill did not win over embedded")
	}
	if strings.Contains(content, "especialista em criar PRDs") {
		t.Error("resolved content leaked embedded skill body")
	}
}

func TestResolveFallsBackToEmbeddedForUnoverriddenSkill(t *testing.T) {
	dir := writeProject(t, map[string]string{
		filepath.Join(".agents", "skills", "kspec-qa", "SKILL.md"): "---\nname: kspec-qa\nversion: 1.0.0\ndescription: Custom QA.\n---\nPROJECT QA\n",
	})
	s := storeAt(t, dir)
	content, err := s.Resolve("kspec-prd")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !strings.Contains(content, "especialista em criar PRDs") {
		t.Error("embedded fallback not used for unoverridden skill")
	}
	qa, err := s.Resolve("kspec-qa")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !strings.Contains(qa, "PROJECT QA") {
		t.Error("project skill not resolved")
	}
}

func TestResolveProjectOnlySkill(t *testing.T) {
	dir := writeProject(t, map[string]string{
		filepath.Join(".agents", "skills", "kspec-custom", "SKILL.md"): "---\nname: kspec-custom\nversion: 1.0.0\ndescription: Custom.\n---\ncustom body\n",
	})
	s := storeAt(t, dir)
	content, err := s.Resolve("kspec-custom")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !strings.Contains(content, "custom body") {
		t.Error("project-only skill not resolvable")
	}
	found := false
	for _, sk := range s.List() {
		if sk.Name == "kspec-custom" {
			found = true
		}
	}
	if !found {
		t.Error("project-only skill missing from List()")
	}
}

func TestResolveTemplateProjectFirst(t *testing.T) {
	dir := writeProject(t, map[string]string{
		filepath.Join(".agents", "templates", "prd-template.md"): "PROJECT TEMPLATE MARKER\n",
	})
	content, err := storeAt(t, dir).Resolve("kspec-prd")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !strings.Contains(content, "PROJECT TEMPLATE MARKER") {
		t.Error("project template not inlined into embedded skill")
	}
	if strings.Contains(content, "# PRD — [Nome da funcionalidade/produto]") {
		t.Error("embedded template used despite project override")
	}
}

func TestResolveKeepsReferenceForMissingTemplate(t *testing.T) {
	f := fstest.MapFS{
		"skills/kspec-demo/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: kspec-demo\nversion: 1.0.0\ndescription: Demo.\n---\nsee @.agents/templates/nope.md now\n")},
	}
	content, err := newStore(f, "").Resolve("kspec-demo")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !strings.Contains(content, "@.agents/templates/nope.md") {
		t.Errorf("missing template reference not kept: %q", content)
	}
}

type failingTemplateFS struct {
	fs.FS
}

func (f failingTemplateFS) Open(name string) (fs.File, error) {
	if strings.HasPrefix(name, templatesDir+"/") {
		return nil, fmt.Errorf("injected template read failure")
	}
	return f.FS.Open(name)
}

func TestResolveReportsTemplateReadError(t *testing.T) {
	f := failingTemplateFS{FS: fstest.MapFS{
		"skills/kspec-demo/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: kspec-demo\nversion: 1.0.0\ndescription: Demo.\n---\nsee @.agents/templates/broken.md now\n")},
	}}
	_, err := newStore(f, "").Resolve("kspec-demo")
	if err == nil {
		t.Fatal("expected template read error, got nil")
	}
	if !strings.Contains(err.Error(), "broken.md") {
		t.Errorf("error = %q, want template name in error", err)
	}
}

func TestRulesEmbeddedFallback(t *testing.T) {
	rules := embeddedStore(t).Rules()
	if len(rules) != 13 {
		t.Fatalf("Rules() = %d rules, want 13", len(rules))
	}
	names := make([]string, len(rules))
	for i, r := range rules {
		names[i] = r.Name
	}
	sort.Strings(names)
	if names[0] != "angular" || names[len(names)-1] != "typescript" {
		t.Errorf("Rules() names = %v", names)
	}
	found := false
	for _, r := range rules {
		if r.Name == "code-standards" {
			found = true
			if r.Content == "" {
				t.Error("rule code-standards has empty content")
			}
		}
	}
	if !found {
		t.Error("code-standards rule missing")
	}
}

func TestRulesProjectWins(t *testing.T) {
	dir := writeProject(t, map[string]string{
		filepath.Join(".agents", "rules", "custom-rule.md"): "custom rule content\n",
	})
	rules := storeAt(t, dir).Rules()
	if len(rules) != 1 {
		t.Fatalf("Rules() = %d rules, want 1 project rule", len(rules))
	}
	if rules[0].Name != "custom-rule" {
		t.Errorf("Name = %q, want custom-rule", rules[0].Name)
	}
	if rules[0].Content != "custom rule content\n" {
		t.Errorf("Content = %q", rules[0].Content)
	}
}

func TestHasProjectRules(t *testing.T) {
	if embeddedStore(t).HasProjectRules() {
		t.Fatal("HasProjectRules() = true, want false without a project dir")
	}
	if storeAt(t, t.TempDir()).HasProjectRules() {
		t.Fatal("HasProjectRules() = true, want false without .agents/rules")
	}
	dir := writeProject(t, map[string]string{
		filepath.Join(".agents", "rules", "custom-rule.md"): "custom rule content\n",
	})
	if !storeAt(t, dir).HasProjectRules() {
		t.Fatal("HasProjectRules() = false, want true with .agents/rules present")
	}
}

func TestRulesIgnoresNonMarkdownFiles(t *testing.T) {
	dir := writeProject(t, map[string]string{
		filepath.Join(".agents", "rules", "notes.txt"): "not a rule\n",
	})
	rules := storeAt(t, dir).Rules()
	if len(rules) != 13 {
		t.Fatalf("Rules() = %d, want 13 embedded fallback rules", len(rules))
	}
}

func TestSourceAndVersionEmbeddedMode(t *testing.T) {
	s := embeddedStore(t)
	if s.Source() != SourceEmbedded {
		t.Errorf("Source() = %q, want %q", s.Source(), SourceEmbedded)
	}
	vData, err := os.ReadFile(filepath.Join("embed", "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	want := strings.TrimSpace(string(vData))
	if s.Version() != want {
		t.Errorf("Version() = %q, want %q", s.Version(), want)
	}
}

func TestSourceAndVersionProjectMode(t *testing.T) {
	dir := writeProject(t, map[string]string{
		filepath.Join(".agents", "skills", "kspec-version", "SKILL.md"): "---\nname: kspec-version\nversion: 9.8.7\ndescription: Custom version skill.\n---\nbody\n",
	})
	s := storeAt(t, dir)
	if s.Source() != SourceProject {
		t.Errorf("Source() = %q, want %q", s.Source(), SourceProject)
	}
	if s.Version() != "9.8.7" {
		t.Errorf("Version() = %q, want 9.8.7", s.Version())
	}
}

func TestVersionEmptyInProjectModeWithoutVersionSkill(t *testing.T) {
	dir := writeProject(t, map[string]string{
		filepath.Join(".agents", "skills", "kspec-prd", "SKILL.md"): "---\nname: kspec-prd\nversion: 1.0.0\ndescription: Custom.\n---\nbody\n",
	})
	s := storeAt(t, dir)
	if s.Source() != SourceProject {
		t.Fatalf("Source() = %q, want %q", s.Source(), SourceProject)
	}
	if v := s.Version(); v != "" {
		t.Errorf("Version() = %q, want empty in project mode without kspec-version skill", v)
	}
}

func TestVersionEmptyInProjectModeWithBrokenVersionSkill(t *testing.T) {
	dir := writeProject(t, map[string]string{
		filepath.Join(".agents", "skills", "kspec-version", "SKILL.md"): "no frontmatter\n",
	})
	s := storeAt(t, dir)
	if s.Source() != SourceProject {
		t.Fatalf("Source() = %q, want %q", s.Source(), SourceProject)
	}
	if v := s.Version(); v != "" {
		t.Errorf("Version() = %q, want empty for broken project version skill", v)
	}
}

func TestSourceEmbeddedWhenNoSkillFile(t *testing.T) {
	dir := writeProject(t, map[string]string{
		filepath.Join(".agents", "skills", "kspec-empty", "other.md"): "no SKILL.md\n",
	})
	if s := storeAt(t, dir); s.Source() != SourceEmbedded {
		t.Errorf("Source() = %q, want %q", s.Source(), SourceEmbedded)
	}
}

func TestListMergesProjectAndEmbeddedSkills(t *testing.T) {
	dir := writeProject(t, map[string]string{
		filepath.Join(".agents", "skills", "kspec-prd", "SKILL.md"):   "---\nname: kspec-prd\nversion: 9.9.9\ndescription: Custom PRD.\n---\nbody\n",
		filepath.Join(".agents", "skills", "kspec-extra", "SKILL.md"): "---\nname: kspec-extra\nversion: 2.0.0\ndescription: Extra.\n---\nbody\n",
		filepath.Join(".agents", "skills", "third-party", "SKILL.md"): "---\nname: third-party\n---\nnope\n",
	})
	list := storeAt(t, dir).List()
	byName := map[string]Skill{}
	for _, sk := range list {
		byName[sk.Name] = sk
	}
	if len(list) != 11 {
		t.Fatalf("List() = %d skills, want 11", len(list))
	}
	if got := byName["kspec-prd"]; got.Version != "9.9.9" || got.Description != "Custom PRD." {
		t.Errorf("kspec-prd = %+v, want project copy", got)
	}
	if _, ok := byName["kspec-extra"]; !ok {
		t.Error("project-only skill kspec-extra missing from List()")
	}
	if _, ok := byName["third-party"]; ok {
		t.Error("third-party skill listed")
	}
	if got := byName["kspec-qa"]; got.Version == "" {
		t.Error("embedded fallback skill kspec-qa missing")
	}
}

func TestBrokenProjectSkillSuppressesEmbeddedAndReportsInvalid(t *testing.T) {
	dir := writeProject(t, map[string]string{
		filepath.Join(".agents", "skills", "kspec-prd", "SKILL.md"): "no frontmatter here\n",
	})
	s := storeAt(t, dir)
	for _, sk := range s.List() {
		if sk.Name == "kspec-prd" {
			t.Error("broken project skill listed")
		}
	}
	found := false
	for _, inv := range s.Invalid() {
		if strings.HasSuffix(inv.Dir, "kspec-prd") {
			found = true
			if inv.Err == nil {
				t.Error("invalid project skill without error")
			}
		}
	}
	if !found {
		t.Error("broken project skill not reported by Invalid()")
	}
	if _, err := s.Resolve("kspec-prd"); err == nil {
		t.Error("Resolve on broken project skill: expected error, got nil")
	}
}

func TestListResolveFlowOverProjectDir(t *testing.T) {
	dir := writeProject(t, map[string]string{
		filepath.Join(".agents", "skills", "kspec-flow", "SKILL.md"): "---\nname: kspec-flow\nversion: 3.1.4\ndescription: Flow skill.\n---\nUse @.agents/templates/flow-template.md\n",
		filepath.Join(".agents", "templates", "flow-template.md"):    "# Flow Template\n\nflow template body\n",
	})
	s := storeAt(t, dir)
	listed := false
	for _, sk := range s.List() {
		if sk.Name == "kspec-flow" && sk.Version == "3.1.4" {
			listed = true
		}
	}
	if !listed {
		t.Fatal("kspec-flow not in List()")
	}
	content, err := s.Resolve("kspec-flow")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if strings.Contains(content, "name: kspec-flow") {
		t.Error("frontmatter not stripped")
	}
	if !strings.Contains(content, "Use # Flow Template") {
		t.Errorf("template not inlined: %q", content)
	}
}

func TestBootstrapMaterializesFullTree(t *testing.T) {
	dir := t.TempDir()
	if err := embeddedStore(t).Bootstrap(dir, false); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	err := fs.WalkDir(embedSub(t), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		target := filepath.Join(dir, ".agents", filepath.FromSlash(p))
		if p == "VERSION" {
			target = filepath.Join(dir, "VERSION")
		}
		want, err := fs.ReadFile(embedSub(t), p)
		if err != nil {
			return err
		}
		got, err := os.ReadFile(target)
		if err != nil {
			return fmt.Errorf("bootstrapped file %s: %w", target, err)
		}
		if !bytes.Equal(got, want) {
			return fmt.Errorf("bootstrapped %s differs from the embedded copy", target)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := countDirs(t, filepath.Join(dir, agentsDir, skillsDir)); n != 10 {
		t.Errorf("bootstrapped skills = %d, want 10", n)
	}
	if n := countMarkdown(t, filepath.Join(dir, agentsDir, templatesDir)); n != 7 {
		t.Errorf("bootstrapped templates = %d, want 7", n)
	}
	if n := countMarkdown(t, filepath.Join(dir, agentsDir, rulesDir)); n != 13 {
		t.Errorf("bootstrapped rules = %d, want 13", n)
	}
	info, err := os.Stat(filepath.Join(dir, "spec", "tasks"))
	if err != nil || !info.IsDir() {
		t.Fatalf("spec/tasks missing or not a directory: %v", err)
	}
}

func countDirs(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			n++
		}
	}
	return n
}

func countMarkdown(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			n++
		}
	}
	return n
}

func TestBootstrapSkillsHaveValidFrontmatter(t *testing.T) {
	dir := t.TempDir()
	if err := embeddedStore(t).Bootstrap(dir, false); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, agentsDir, skillsDir))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 10 {
		t.Fatalf("bootstrapped skill dirs = %d, want 10", len(entries))
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, agentsDir, skillsDir, e.Name(), skillFile))
		if err != nil {
			t.Fatalf("read bootstrapped skill %s: %v", e.Name(), err)
		}
		fm, err := parseFrontmatter(data)
		if err != nil {
			t.Errorf("bootstrapped skill %s has invalid frontmatter: %v", e.Name(), err)
			continue
		}
		if fm.Name != e.Name() {
			t.Errorf("bootstrapped skill %s frontmatter name = %q", e.Name(), fm.Name)
		}
		if fm.Version == "" || fm.Description == "" {
			t.Errorf("bootstrapped skill %s missing version or description", e.Name())
		}
	}
}

func TestBootstrapGuardRefusesExistingAgents(t *testing.T) {
	dir := writeProject(t, map[string]string{
		filepath.Join(".agents", "skills", "kspec-prd", "SKILL.md"): "custom\n",
	})
	err := embeddedStore(t).Bootstrap(dir, false)
	if err == nil {
		t.Fatal("expected guard error over existing .agents")
	}
	if !strings.Contains(err.Error(), "force") {
		t.Errorf("error = %q, want it to mention force", err.Error())
	}
	if _, err := os.Stat(filepath.Join(dir, "spec", "tasks")); err == nil {
		t.Error("guard must fail before writing anything")
	}
	data, err := os.ReadFile(filepath.Join(dir, ".agents", "skills", "kspec-prd", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "custom\n" {
		t.Errorf("guard overwrote existing skill: %q", data)
	}
}

func TestBootstrapForceOverwritesKspecFilesKeepsCustoms(t *testing.T) {
	dir := writeProject(t, map[string]string{
		filepath.Join(".agents", "skills", "kspec-prd", "SKILL.md"): "custom\n",
		filepath.Join(".agents", "skills", "mine", "SKILL.md"):      "---\nname: mine\nversion: 1.0.0\ndescription: Mine.\n---\nkeep\n",
	})
	if err := embeddedStore(t).Bootstrap(dir, true); err != nil {
		t.Fatalf("Bootstrap force: %v", err)
	}
	want, err := fs.ReadFile(embedSub(t), path.Join(skillsDir, "kspec-prd", skillFile))
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, agentsDir, skillsDir, "kspec-prd", skillFile))
	if err != nil {
		t.Fatalf("kspec-prd not overwritten: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Error("force did not restore the embedded kspec-prd copy")
	}
	if _, err := os.Stat(filepath.Join(dir, agentsDir, skillsDir, "mine", skillFile)); err != nil {
		t.Errorf("force removed a non-kspec custom skill: %v", err)
	}
}

func TestBootstrapGuardRefusesDivergentRootVersion(t *testing.T) {
	dir := writeProject(t, map[string]string{"VERSION": "0.1.0-custom\n"})
	err := embeddedStore(t).Bootstrap(dir, false)
	if err == nil {
		t.Fatal("expected guard error over divergent root VERSION")
	}
	if !strings.Contains(err.Error(), "force") {
		t.Errorf("error = %q, want it to mention force", err.Error())
	}
	data, err := os.ReadFile(filepath.Join(dir, "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "0.1.0-custom\n" {
		t.Errorf("guard overwrote divergent VERSION: %q", data)
	}
	if _, err := os.Stat(filepath.Join(dir, agentsDir)); err == nil {
		t.Error("guard must fail before writing anything")
	}
	if err := embeddedStore(t).Bootstrap(dir, true); err != nil {
		t.Fatalf("Bootstrap force: %v", err)
	}
	want, err := fs.ReadFile(embedSub(t), "VERSION")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("force VERSION = %q, want embedded %q", got, want)
	}
}

func TestBootstrapIdenticalRootVersionProceeds(t *testing.T) {
	want, err := fs.ReadFile(embedSub(t), "VERSION")
	if err != nil {
		t.Fatal(err)
	}
	dir := writeProject(t, map[string]string{"VERSION": string(want)})
	if err := embeddedStore(t).Bootstrap(dir, false); err != nil {
		t.Fatalf("Bootstrap with identical root VERSION: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, agentsDir, skillsDir, "kspec-prd", skillFile)); err != nil {
		t.Errorf("missing bootstrapped kspec-prd: %v", err)
	}
}

func TestBootstrapTrimmedEqualRootVersionProceeds(t *testing.T) {
	want, err := fs.ReadFile(embedSub(t), "VERSION")
	if err != nil {
		t.Fatal(err)
	}
	dir := writeProject(t, map[string]string{"VERSION": strings.TrimSpace(string(want))})
	if err := embeddedStore(t).Bootstrap(dir, false); err != nil {
		t.Fatalf("Bootstrap with trimmed-equal root VERSION: %v", err)
	}
}

func TestBootstrapPartialFailureErrorGuidesRepair(t *testing.T) {
	dir := writeProject(t, map[string]string{
		filepath.Join(agentsDir, skillsDir, "kspec-bugfix"): "not a directory\n",
	})
	err := embeddedStore(t).Bootstrap(dir, true)
	if err == nil {
		t.Fatal("expected partial bootstrap failure")
	}
	if !strings.Contains(err.Error(), "re-run with force") {
		t.Errorf("error = %q, want guidance to re-run with force", err.Error())
	}
	if _, err := os.Stat(filepath.Join(dir, "VERSION")); err != nil {
		t.Errorf("expected partial tree with VERSION already written: %v", err)
	}
}

func TestBootstrapThenSourceProjectAndResolveUsesProjectCopy(t *testing.T) {
	dir := t.TempDir()
	if err := embeddedStore(t).Bootstrap(dir, false); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	s := storeAt(t, dir)
	if s.Source() != SourceProject {
		t.Fatalf("Source() = %q, want %q after bootstrap", s.Source(), SourceProject)
	}
	vData, err := os.ReadFile(filepath.Join(dir, agentsDir, skillsDir, versionSkill, skillFile))
	if err != nil {
		t.Fatal(err)
	}
	fm, err := parseFrontmatter(vData)
	if err != nil {
		t.Fatal(err)
	}
	if s.Version() != fm.Version {
		t.Errorf("Version() = %q, want project skill version %q", s.Version(), fm.Version)
	}
	marker := "---\nname: kspec-prd\nversion: 9.9.9\ndescription: Custom PRD.\n---\nPROJECT PRD MARKER\n"
	if err := os.WriteFile(filepath.Join(dir, agentsDir, skillsDir, "kspec-prd", skillFile), []byte(marker), 0o644); err != nil {
		t.Fatal(err)
	}
	content, err := s.Resolve("kspec-prd")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !strings.Contains(content, "PROJECT PRD MARKER") {
		t.Error("Resolve did not use the bootstrapped project copy")
	}
}

func TestBootstrapWithoutEmbeddedContent(t *testing.T) {
	err := (&Store{}).Bootstrap(t.TempDir(), false)
	if err == nil || !strings.Contains(err.Error(), "embedded kspec content unavailable") {
		t.Fatalf("err = %v, want embedded content unavailable", err)
	}
}

func TestRuleDisciplinesParsed(t *testing.T) {
	dir := writeProject(t, map[string]string{
		filepath.Join(".agents", "rules", "go.md"): "---\ndisciplines: [backend, architecture]\n---\n# Go\ngofmt, no comments.\n",
	})
	rules := storeAt(t, dir).Rules()
	if len(rules) != 1 {
		t.Fatalf("Rules() = %d, want 1", len(rules))
	}
	got := rules[0].Disciplines
	if len(got) != 2 || got[0] != "backend" || got[1] != "architecture" {
		t.Fatalf("Disciplines = %v, want [backend architecture]", got)
	}
}

func TestRuleWithoutFrontmatterHasNoDisciplines(t *testing.T) {
	dir := writeProject(t, map[string]string{
		filepath.Join(".agents", "rules", "plain.md"): "# Plain\n\nno frontmatter here\n",
	})
	rules := storeAt(t, dir).Rules()
	if len(rules) != 1 {
		t.Fatalf("Rules() = %d, want 1", len(rules))
	}
	if len(rules[0].Disciplines) != 0 {
		t.Fatalf("Disciplines = %v, want none for a rule without frontmatter", rules[0].Disciplines)
	}
}
