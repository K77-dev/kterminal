package kspec

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const specTasksPath = "spec/tasks"

const partialBootstrapFormat = "bootstrap stopped partway, the tree may be incomplete: %w; re-run with force to repair"

func (s *Store) Bootstrap(dir string, force bool) error {
	if s.embedded == nil {
		return fmt.Errorf("embedded kspec content unavailable")
	}
	agentsPath := filepath.Join(dir, agentsDir)
	if !force {
		if _, err := os.Stat(agentsPath); err == nil {
			return fmt.Errorf("%s already exists; pass force to overwrite it", agentsPath)
		}
		if err := s.checkRootVersion(dir); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(agentsPath, 0o755); err != nil {
		return err
	}
	if err := s.copyEmbeddedTree(dir); err != nil {
		return fmt.Errorf(partialBootstrapFormat, err)
	}
	if err := os.MkdirAll(filepath.Join(dir, specTasksPath), 0o755); err != nil {
		return fmt.Errorf(partialBootstrapFormat, err)
	}
	return nil
}

func (s *Store) checkRootVersion(dir string) error {
	want, err := fs.ReadFile(s.embedded, "VERSION")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	versionPath := filepath.Join(dir, "VERSION")
	existing, err := os.ReadFile(versionPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if strings.TrimSpace(string(existing)) == strings.TrimSpace(string(want)) {
		return nil
	}
	return fmt.Errorf("%s already exists with different content; pass force to overwrite it", versionPath)
}

func (s *Store) copyEmbeddedTree(dir string) error {
	return fs.WalkDir(s.embedded, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		target, ok := bootstrapTarget(dir, p)
		if !ok {
			return nil
		}
		data, err := fs.ReadFile(s.embedded, p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func bootstrapTarget(dir, p string) (string, bool) {
	if p == "VERSION" {
		return filepath.Join(dir, "VERSION"), true
	}
	for _, top := range []string{skillsDir, templatesDir, rulesDir} {
		if !strings.HasPrefix(p, top+"/") {
			continue
		}
		if top == skillsDir && !isKspecSkillPath(p) {
			return "", false
		}
		return filepath.Join(dir, agentsDir, filepath.FromSlash(p)), true
	}
	return "", false
}

func isKspecSkillPath(p string) bool {
	rest := strings.TrimPrefix(p, skillsDir+"/")
	name, _, found := strings.Cut(rest, "/")
	return found && strings.HasPrefix(name, "kspec-")
}
