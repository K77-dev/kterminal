package tools

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const vetTimeout = 30 * time.Second

var vetDiagPattern = regexp.MustCompile(`\.go:\d+`)

func GoVetHook(path string) string {
	root, rel, ok := goModuleRoot(path)
	if !ok {
		return ""
	}
	if _, err := exec.LookPath("go"); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), vetTimeout)
	defer cancel()
	pkgPattern := "."
	if dir := filepath.Dir(rel); dir != "." {
		pkgPattern = "./" + dir
	}
	cmd := exec.CommandContext(ctx, "go", "vet", pkgPattern)
	cmd.Dir = root
	cmd.Env = vetEnv()
	out, runErr := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return ""
	}
	matched := filterVetLines(string(out), rel)
	if len(matched) > 0 {
		return "\n\nDiagnostics:\n" + strings.Join(matched, "\n")
	}
	if runErr == nil || vetDiagPattern.Match(out) {
		return "\n\nDiagnostics: clean"
	}
	return ""
}

func goModuleRoot(path string) (root, rel string, ok bool) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", false
	}
	dir := filepath.Dir(abs)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			rel, err := filepath.Rel(dir, abs)
			if err != nil {
				return "", "", false
			}
			return dir, rel, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", false
		}
		dir = parent
	}
}

func filterVetLines(out, rel string) []string {
	var matched []string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, rel) {
			matched = append(matched, line)
		}
	}
	return matched
}

func vetEnv() []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "GOFLAGS=") {
			continue
		}
		env = append(env, e)
	}
	return append(env, "GOFLAGS=-mod=mod")
}
