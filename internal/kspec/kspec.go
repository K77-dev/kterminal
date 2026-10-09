package kspec

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

//go:embed embed
var embedded embed.FS

const (
	SourceProject  = "project"
	SourceEmbedded = "embedded"
)

const (
	agentsDir    = ".agents"
	skillsDir    = "skills"
	templatesDir = "templates"
	rulesDir     = "rules"
	skillFile    = "SKILL.md"
	versionSkill = "kspec-version"
)

var templateRef = regexp.MustCompile(`@\.agents/templates/([A-Za-z0-9._-]+\.md)`)

type Skill struct {
	Name        string
	Version     string
	Description string
	ArgHint     string
}

type Rule struct {
	Name        string
	Disciplines []string
	Content     string
}

type InvalidSkill struct {
	Dir string
	Err error
}

type Store struct {
	embedded fs.FS
	dir      string
	skills   []Skill
	invalid  []InvalidSkill
	version  string
}

func Load() *Store {
	sub, err := fs.Sub(embedded, "embed")
	if err != nil {
		return &Store{}
	}
	dir, err := os.Getwd()
	if err != nil {
		return newStore(sub, "")
	}
	return newStore(sub, dir)
}

func newStore(f fs.FS, dir string) *Store {
	s := &Store{embedded: f, dir: dir, version: readVersion(f)}
	entries, err := fs.ReadDir(f, skillsDir)
	if err != nil {
		return s
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "kspec-") {
			continue
		}
		skillDir := path.Join(skillsDir, e.Name())
		sk, err := loadSkill(f, skillDir)
		if err != nil {
			s.invalid = append(s.invalid, InvalidSkill{Dir: skillDir, Err: err})
			continue
		}
		s.skills = append(s.skills, sk)
	}
	return s
}

func loadSkill(f fs.FS, dir string) (Skill, error) {
	data, err := fs.ReadFile(f, path.Join(dir, skillFile))
	if err != nil {
		return Skill{}, err
	}
	fm, err := parseFrontmatter(data)
	if err != nil {
		return Skill{}, err
	}
	return Skill{Name: fm.Name, Version: fm.Version, Description: fm.Description, ArgHint: fm.ArgHint}, nil
}

func readVersion(f fs.FS) string {
	data, err := fs.ReadFile(f, "VERSION")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

type projectState struct {
	skills map[string]Skill
	broken map[string]error
}

func (s *Store) project() projectState {
	st := projectState{skills: map[string]Skill{}, broken: map[string]error{}}
	if s.dir == "" {
		return st
	}
	entries, err := os.ReadDir(filepath.Join(s.dir, agentsDir, skillsDir))
	if err != nil {
		return st
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "kspec-") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, agentsDir, skillsDir, e.Name(), skillFile))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			st.broken[e.Name()] = err
			continue
		}
		fm, err := parseFrontmatter(data)
		if err != nil {
			st.broken[e.Name()] = err
			continue
		}
		st.skills[e.Name()] = Skill{Name: fm.Name, Version: fm.Version, Description: fm.Description, ArgHint: fm.ArgHint}
	}
	return st
}

func (s *Store) List() []Skill {
	st := s.project()
	out := make([]Skill, 0, len(s.skills)+len(st.skills))
	listed := map[string]bool{}
	for _, sk := range s.skills {
		if _, broken := st.broken[sk.Name]; broken {
			continue
		}
		if proj, ok := st.skills[sk.Name]; ok {
			out = append(out, proj)
		} else {
			out = append(out, sk)
		}
		listed[sk.Name] = true
	}
	for name, sk := range st.skills {
		if !listed[name] {
			out = append(out, sk)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *Store) Invalid() []InvalidSkill {
	out := make([]InvalidSkill, len(s.invalid))
	copy(out, s.invalid)
	st := s.project()
	names := make([]string, 0, len(st.broken))
	for name := range st.broken {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		out = append(out, InvalidSkill{Dir: path.Join(agentsDir, skillsDir, name), Err: st.broken[name]})
	}
	return out
}

func (s *Store) Source() string {
	if s.hasProjectSkills() {
		return SourceProject
	}
	return SourceEmbedded
}

func (s *Store) hasProjectSkills() bool {
	if s.dir == "" {
		return false
	}
	entries, err := os.ReadDir(filepath.Join(s.dir, agentsDir, skillsDir))
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "kspec-") {
			continue
		}
		if _, err := os.Stat(filepath.Join(s.dir, agentsDir, skillsDir, e.Name(), skillFile)); err == nil {
			return true
		}
	}
	return false
}

func (s *Store) Version() string {
	if s.Source() == SourceProject {
		return s.projectVersion()
	}
	return s.version
}

func (s *Store) projectVersion() string {
	if s.dir == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(s.dir, agentsDir, skillsDir, versionSkill, skillFile))
	if err != nil {
		return ""
	}
	fm, err := parseFrontmatter(data)
	if err != nil {
		return ""
	}
	return fm.Version
}

func (s *Store) Rules() []Rule {
	if rules := s.projectRules(); len(rules) > 0 {
		return rules
	}
	return s.embeddedRules()
}

func (s *Store) HasProjectRules() bool {
	return len(s.projectRules()) > 0
}

func (s *Store) projectRules() []Rule {
	if s.dir == "" {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(s.dir, agentsDir, rulesDir))
	if err != nil {
		return nil
	}
	return readRules(entries, func(name string) ([]byte, error) {
		return os.ReadFile(filepath.Join(s.dir, agentsDir, rulesDir, name))
	})
}

func (s *Store) embeddedRules() []Rule {
	entries, err := fs.ReadDir(s.embedded, rulesDir)
	if err != nil {
		return nil
	}
	return readRules(entries, func(name string) ([]byte, error) {
		return fs.ReadFile(s.embedded, path.Join(rulesDir, name))
	})
}

func readRules(entries []fs.DirEntry, read func(string) ([]byte, error)) []Rule {
	var out []Rule
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := read(e.Name())
		if err != nil {
			continue
		}
		out = append(out, Rule{
			Name:        strings.TrimSuffix(e.Name(), ".md"),
			Disciplines: ruleDisciplines(data),
			Content:     string(data),
		})
	}
	return out
}

func ruleDisciplines(data []byte) []string {
	fm, err := parseRuleFrontmatter(data)
	if err != nil {
		return nil
	}
	return fm.Disciplines
}

func (s *Store) Resolve(name string) (string, error) {
	if !validName(name) {
		return "", fmt.Errorf("invalid skill name %q", name)
	}
	data, err := s.readSkill(name)
	if err != nil {
		return "", err
	}
	body, err := frontmatterBody(data)
	if err != nil {
		return "", err
	}
	content, err := s.inlineTemplates(string(body))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(content), nil
}

func (s *Store) inlineTemplates(body string) (string, error) {
	matches := templateRef.FindAllStringIndex(body, -1)
	if len(matches) == 0 {
		return body, nil
	}
	var b strings.Builder
	last := 0
	for _, m := range matches {
		b.WriteString(body[last:m[0]])
		last = m[1]
		ref := body[m[0]:m[1]]
		name := templateRef.FindStringSubmatch(ref)[1]
		data, err := s.readTemplate(name)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				b.WriteString(ref)
				continue
			}
			return "", fmt.Errorf("read template %q: %w", name, err)
		}
		b.Write(data)
	}
	b.WriteString(body[last:])
	return b.String(), nil
}

func (s *Store) readSkill(name string) ([]byte, error) {
	if s.dir != "" {
		data, err := os.ReadFile(filepath.Join(s.dir, agentsDir, skillsDir, name, skillFile))
		if err == nil {
			return data, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	data, err := fs.ReadFile(s.embedded, path.Join(skillsDir, name, skillFile))
	if err != nil {
		return nil, fmt.Errorf("skill %q not found", name)
	}
	return data, nil
}

func (s *Store) readTemplate(name string) ([]byte, error) {
	if s.dir != "" {
		data, err := os.ReadFile(filepath.Join(s.dir, agentsDir, templatesDir, name))
		if err == nil {
			return data, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	return fs.ReadFile(s.embedded, path.Join(templatesDir, name))
}

func validName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if strings.ContainsAny(name, `/\`) {
		return false
	}
	return !strings.Contains(name, "..")
}
