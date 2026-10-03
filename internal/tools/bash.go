package tools

import (
	"bytes"
	"context"
	"fmt"
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
		Execute: func(args map[string]any) (string, error) {
			command, err := str(args, "command")
			if err != nil {
				return "", err
			}
			ctx, cancel := context.WithTimeout(context.Background(), bashTimeout)
			defer cancel()
			cmd := exec.CommandContext(ctx, "bash", "-c", command)
			var buf bytes.Buffer
			cmd.Stdout = &buf
			cmd.Stderr = &buf
			runErr := cmd.Run()
			out := buf.String()
			if len(out) > maxBashOutput {
				out = out[:maxBashOutput] + "\n... (truncated)"
			}
			if runErr != nil {
				if ctx.Err() == context.DeadlineExceeded {
					return "", fmt.Errorf("command timed out after %s; output so far:\n%s", bashTimeout, out)
				}
				if exitErr, ok := runErr.(*exec.ExitError); ok {
					return fmt.Sprintf("exit status %d\n%s", exitErr.ExitCode(), out), nil
				}
				return "", runErr
			}
			if out == "" {
				out = "(no output)"
			}
			return out, nil
		},
	}
}
