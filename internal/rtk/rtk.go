package rtk

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode"
)

const rewriteTimeout = 3 * time.Second

type Detector struct {
	available bool
	checked   bool
	mu        sync.Mutex
}

func (d *Detector) Available() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.checked {
		return d.available
	}
	d.checked = true
	d.available = detect()
	return d.available
}

func detect() bool {
	_, err := exec.LookPath("rtk")
	return err == nil
}

func Rewrite(ctx context.Context, command string) string {
	if !shouldRewrite(command) {
		return command
	}
	rewritten, ok := callRewrite(ctx, command)
	if !ok {
		return command
	}
	return rewritten
}

func shouldRewrite(command string) bool {
	if command == "" {
		return false
	}
	if hasLeadingRtk(command) {
		return false
	}
	if hasShellMetacharacters(command) {
		return false
	}
	return true
}

func hasLeadingRtk(command string) bool {
	trimmed := strings.TrimLeftFunc(command, unicode.IsSpace)
	return strings.HasPrefix(trimmed, "rtk ") || trimmed == "rtk"
}

func hasShellMetacharacters(command string) bool {
	return strings.ContainsAny(command, "&|;<>`") ||
		strings.Contains(command, "$(") ||
		strings.Contains(command, "${") ||
		strings.Contains(command, "<<") ||
		strings.Contains(command, ">>")
}

func callRewrite(ctx context.Context, command string) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, rewriteTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "rtk", "rewrite", command)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return "", false
	}
	out := strings.TrimSpace(buf.String())
	if out == "" || out == command {
		return "", false
	}
	return out, true
}
