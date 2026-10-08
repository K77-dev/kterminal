package tools

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"

	"kterminal/internal/llm"
)

const bashTimeout = 120 * time.Second
const maxBashOutput = 32 * 1024

func bashTool() Tool {
	return Tool{
		Name:     "bash",
		Mutating: true,
		Schema: llm.Tool{
			Type: "function",
			Function: llm.Function{
				Name:        "bash",
				Description: "Run a shell command in the working directory and return its output",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"command": map[string]any{"type": "string", "description": "Shell command to execute"},
					},
					"required": []string{"command"},
				},
			},
		},
		Execute: func(ctx context.Context, args map[string]any) (Result, error) {
			return runBash(ctx, args, nil)
		},
		ExecuteStream: func(ctx context.Context, args map[string]any, onLine func(string)) (Result, error) {
			return runBash(ctx, args, onLine)
		},
	}
}

func runBash(ctx context.Context, args map[string]any, onLine func(string)) (Result, error) {
	command, err := str(args, "command")
	if err != nil {
		return Result{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, bashTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	var buf bytes.Buffer
	var w io.Writer = &buf
	var lw *lineWriter
	if onLine != nil {
		lw = newLineWriter(onLine)
		w = io.MultiWriter(&buf, lw)
	}
	cmd.Stdout = w
	cmd.Stderr = w
	runErr := cmd.Run()
	if lw != nil {
		lw.flush()
	}
	out := buf.String()
	if len(out) > maxBashOutput {
		out = out[:maxBashOutput] + "\n... (truncated)"
	}
	if runErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return Result{}, fmt.Errorf("command timed out after %s; output so far:\n%s", bashTimeout, out)
		}
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			return Result{Output: fmt.Sprintf("exit status %d\n%s", exitErr.ExitCode(), out)}, nil
		}
		return Result{}, runErr
	}
	if out == "" {
		out = "(no output)"
	}
	return Result{Output: out}, nil
}

type lineWriter struct {
	onLine  func(string)
	pending []byte
}

func newLineWriter(onLine func(string)) *lineWriter {
	return &lineWriter{onLine: onLine}
}

func (l *lineWriter) Write(p []byte) (int, error) {
	l.pending = append(l.pending, p...)
	for {
		i := bytes.IndexByte(l.pending, '\n')
		if i < 0 {
			break
		}
		l.onLine(string(l.pending[:i]))
		l.pending = l.pending[i+1:]
	}
	return len(p), nil
}

func (l *lineWriter) flush() {
	if len(l.pending) == 0 {
		return
	}
	l.onLine(string(l.pending))
	l.pending = l.pending[:0]
}
