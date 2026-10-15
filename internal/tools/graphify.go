package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"kterminal/internal/llm"
)

const (
	graphifyBinary    = "graphify"
	graphifyGraphPath = "graphify-out/graph.json"
	graphifyTimeout   = 30 * time.Second
	maxGraphifyOutput = 16 * 1024
)

func graphifyTool() Tool {
	return newGraphifyTool(runGraphify)
}

func newGraphifyTool(run func(ctx context.Context, args []string) (string, error)) Tool {
	return Tool{
		Name:     "graphify",
		Mutating: false,
		Schema: llm.Tool{
			Type: "function",
			Function: llm.Function{
				Name:        "graphify",
				Description: "Query the project knowledge graph (Graphify) for code structure, dependencies and impact. Read-only. Requires graphify-out/graph.json; when absent or unavailable, fall back to read/glob/grep.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"command":  map[string]any{"type": "string", "enum": []string{"query", "path", "explain"}, "description": "Graph operation: query (broad context), path (shortest path between two nodes), explain (node explanation)"},
						"question": map[string]any{"type": "string", "description": "query: question about the codebase"},
						"source":   map[string]any{"type": "string", "description": "path: start node"},
						"target":   map[string]any{"type": "string", "description": "path: end node"},
						"node":     map[string]any{"type": "string", "description": "explain: node to explain"},
					},
					"required": []string{"command"},
				},
			},
		},
		Execute: func(ctx context.Context, args map[string]any) (Result, error) {
			if _, err := os.Stat(graphifyGraphPath); err != nil {
				return Result{Output: "no knowledge graph at " + graphifyGraphPath + "; fall back to read/glob/grep"}, nil
			}
			command := optStr(args, "command")
			var cmdArgs []string
			switch command {
			case "query":
				question := optStr(args, "question")
				if question == "" {
					return Result{}, fmt.Errorf("query requires a question")
				}
				cmdArgs = []string{"query", question}
			case "path":
				source := optStr(args, "source")
				target := optStr(args, "target")
				if source == "" || target == "" {
					return Result{}, fmt.Errorf("path requires source and target")
				}
				cmdArgs = []string{"path", source, target}
			case "explain":
				node := optStr(args, "node")
				if node == "" {
					return Result{}, fmt.Errorf("explain requires a node")
				}
				cmdArgs = []string{"explain", node}
			default:
				return Result{}, fmt.Errorf("unknown command %q (use query, path or explain)", command)
			}
			ctx, cancel := context.WithTimeout(ctx, graphifyTimeout)
			defer cancel()
			out, err := run(ctx, cmdArgs)
			out = strings.TrimSpace(out)
			if len(out) > maxGraphifyOutput {
				out = out[:maxGraphifyOutput] + "\n... (truncated)"
			}
			if err != nil {
				if errors.Is(err, exec.ErrNotFound) {
					return Result{Output: "graphify CLI not available; fall back to read/glob/grep"}, nil
				}
				if errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded {
					return Result{Output: "graphify timed out after " + graphifyTimeout.String() + "\n" + out}, nil
				}
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) {
					return Result{Output: fmt.Sprintf("exit status %d\n%s", exitErr.ExitCode(), out)}, nil
				}
				return Result{}, err
			}
			if out == "" {
				out = "(no output)"
			}
			return Result{Output: out}, nil
		},
	}
}

func runGraphify(ctx context.Context, args []string) (string, error) {
	out, err := exec.CommandContext(ctx, graphifyBinary, args...).CombinedOutput()
	return string(out), err
}
