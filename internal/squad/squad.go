package squad

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed embed
var embedded embed.FS

const (
	agentsDir   = ".agents"
	personaDir  = "agents"
	skillsDir   = "personas"
	skillFile   = "AGENT.md"
	maestroFile = "maestro.md"
)

const (
	SourceProject  = "project"
	SourceEmbedded = "embedded"
)

type Persona struct {
	Name       string
	Discipline string
	ModelTags  []string
	Rules      []string
	Prompt     string
}

type Store struct {
	embedded fs.FS
	dir      string
	personas []Persona
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
	s := &Store{embedded: f, dir: dir}
	entries, err := fs.ReadDir(f, skillsDir)
	if err != nil {
		return s
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		personaPath := path.Join(skillsDir, e.Name(), skillFile)
		data, err := fs.ReadFile(f, personaPath)
		if err != nil {
			continue
		}
		p, err := loadPersona(data)
		if err != nil {
			continue
		}
		s.personas = append(s.personas, p)
	}
	return s
}

func loadPersona(data []byte) (Persona, error) {
	fm, err := parseFrontmatter(data)
	if err != nil {
		return Persona{}, err
	}
	body, err := frontmatterBody(data)
	if err != nil {
		return Persona{}, err
	}
	return Persona{
		Name:       fm.Name,
		Discipline: fm.Discipline,
		ModelTags:  fm.ModelTags,
		Rules:      fm.Rules,
		Prompt:     strings.TrimSpace(string(body)),
	}, nil
}

func (s *Store) List() []Persona {
	projectPersonas := s.projectPersonas()
	out := make([]Persona, 0, len(s.personas)+len(projectPersonas))
	listed := map[string]bool{}
	for _, p := range s.personas {
		if proj, ok := projectPersonas[p.Name]; ok {
			out = append(out, proj)
		} else {
			out = append(out, p)
		}
		listed[p.Name] = true
	}
	for name, p := range projectPersonas {
		if !listed[name] {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *Store) Resolve(name string) (Persona, error) {
	if !validName(name) {
		return Persona{}, fmt.Errorf("invalid persona name %q", name)
	}
	data, err := s.readPersona(name)
	if err != nil {
		return Persona{}, err
	}
	return loadPersona(data)
}

func (s *Store) MaestroPrompt() string {
	data, err := fs.ReadFile(s.embedded, maestroFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (s *Store) readPersona(name string) ([]byte, error) {
	if s.dir != "" {
		data, err := os.ReadFile(filepath.Join(s.dir, agentsDir, personaDir, name, skillFile))
		if err == nil {
			return data, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	data, err := fs.ReadFile(s.embedded, path.Join(skillsDir, name, skillFile))
	if err != nil {
		return nil, fmt.Errorf("persona %q not found", name)
	}
	return data, nil
}

func (s *Store) projectPersonas() map[string]Persona {
	out := map[string]Persona{}
	if s.dir == "" {
		return out
	}
	entries, err := os.ReadDir(filepath.Join(s.dir, agentsDir, personaDir))
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, agentsDir, personaDir, e.Name(), skillFile))
		if err != nil {
			continue
		}
		p, err := loadPersona(data)
		if err != nil {
			continue
		}
		out[p.Name] = p
	}
	return out
}

func (s *Store) Source() string {
	if len(s.projectPersonas()) > 0 {
		return SourceProject
	}
	return SourceEmbedded
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

func PersonaDisciplines(p Persona) []string {
	out := make([]string, 0, len(p.Rules)+1)
	if p.Discipline != "" {
		out = append(out, p.Discipline)
	}
	out = append(out, p.Rules...)
	return out
}

func MatchesAny(personaDisciplines, ruleDisciplines []string) bool {
	for _, d := range ruleDisciplines {
		for _, p := range personaDisciplines {
			if d == p {
				return true
			}
		}
	}
	return false
}
