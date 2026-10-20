package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"

	"kterminal/internal/llm"
	"kterminal/internal/procmon"
	"kterminal/internal/rtk"
)

const bashTimeout = 120 * time.Second
const maxBashOutput = 32 * 1024

var bashWaitDelay = 5 * time.Second

func bashTool(pm *procmon.Manager) Tool {
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
						"command":           map[string]any{"type": "string", "description": "Shell command to execute"},
						"run_in_background": map[string]any{"type": "boolean", "description": "Set true for long-running processes (dev servers, watchers, long builds): the command returns immediately and keeps running in the background. Output is discarded."},
					},
					"required": []string{"command"},
				},
			},
		},
		Execute: func(ctx context.Context, args map[string]any) (Result, error) {
			return runBash(ctx, args, nil, pm)
		},
		ExecuteStream: func(ctx context.Context, args map[string]any, onLine func(string)) (Result, error) {
			return runBash(ctx, args, onLine, pm)
		},
	}
}

func rtkBashTool(pm *procmon.Manager, det *rtk.Detector) Tool {
	t := bashTool(pm)
	origExec := t.Execute
	origStream := t.ExecuteStream
	t.Execute = func(ctx context.Context, args map[string]any) (Result, error) {
		args = applyRtkRewrite(ctx, args, det)
		return origExec(ctx, args)
	}
	t.ExecuteStream = func(ctx context.Context, args map[string]any, onLine func(string)) (Result, error) {
		args = applyRtkRewrite(ctx, args, det)
		return origStream(ctx, args, onLine)
	}
	return t
}

func applyRtkRewrite(ctx context.Context, args map[string]any, det *rtk.Detector) map[string]any {
	if det == nil || !det.Available() {
		return args
	}
	command, _ := args["command"].(string)
	if command == "" {
		return args
	}
	rewritten := rtk.Rewrite(ctx, command)
	if rewritten == command {
		return args
	}
	copied := make(map[string]any, len(args))
	for k, v := range args {
		copied[k] = v
	}
	copied["command"] = rewritten
	return copied
}

func runBash(ctx context.Context, args map[string]any, onLine func(string), pm *procmon.Manager) (Result, error) {
	command, err := str(args, "command")
	if err != nil {
		return Result{}, err
	}
	if optBool(args, "run_in_background") {
		if pm == nil {
			return Result{}, fmt.Errorf("background execution unavailable")
		}
		entry, err := pm.Start(ctx, command)
		if err != nil {
			return Result{}, err
		}
		return Result{Output: fmt.Sprintf("background process %d started: %s", entry.ID, command)}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, bashTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.WaitDelay = bashWaitDelay
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
		if errors.Is(runErr, exec.ErrWaitDelay) {
			if out == "" {
				out = "(no output)"
			}
			return Result{Output: out + "\n(note: background processes from this command are still running and their later output is discarded — prefer run_in_background for long-running processes)"}, nil
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
