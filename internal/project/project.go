package project

import (
	"io/fs"
	"os"
	"path/filepath"

	"kterminal/internal/globx"
)

const (
	maxContextBytes = 16 * 1024
	maxFiles        = 10000
)

var contextFiles = []string{"AGENTS.md", "CLAUDE.md", "README.md"}

var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"graphify-out": true,
	"dist":         true,
	"build":        true,
	"target":       true,
}

// Context returns the first project context file among AGENTS.md, CLAUDE.md
// and README.md, capped at maxContextBytes.
func Context(dir string) (name, content string, ok bool) {
	if dir == "" {
		return "", "", false
	}
	for _, candidate := range contextFiles {
		data, err := os.ReadFile(filepath.Join(dir, candidate))
		if err != nil {
			continue
		}
		return candidate, capContent(data), true
	}
	return "", "", false
}

func capContent(data []byte) string {
	if len(data) > maxContextBytes {
		return string(data[:maxContextBytes]) + "\n... (truncated)"
	}
	return string(data)
}

// Files walks the project collecting relative slash-separated paths,
// skipping noise directories and capping at maxFiles entries.
func Files(dir string) []string {
	if dir == "" {
		return nil
	}
	var out []string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != dir && skipDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return nil
		}
		out = append(out, filepath.ToSlash(rel))
		if len(out) >= maxFiles {
			return fs.SkipAll
		}
		return nil
	})
	return out
}

// MatchesAny reports whether any glob pattern matches any of the files.
func MatchesAny(patterns, files []string) bool {
	for _, f := range files {
		if globx.MatchAny(patterns, f) {
			return true
		}
	}
	return false
}
