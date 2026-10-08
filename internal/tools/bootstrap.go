package tools

import (
	"context"
	"fmt"
	"os"

	"kterminal/internal/kspec"
	"kterminal/internal/llm"
)

const KspecBootstrapToolName = "kspec_bootstrap"

func kspecBootstrapTool() Tool {
	return Tool{
		Name:     KspecBootstrapToolName,
		Mutating: true,
		Schema: llm.Tool{
			Type: "function",
			Function: llm.Function{
				Name:        KspecBootstrapToolName,
				Description: "Materialize the embedded kspec tree into the current project: .agents/ (kspec-* skills, templates, rules), VERSION and spec/tasks/. The project becomes a standard kspec project, usable by Claude Code, Codex CLI and Cursor. Refuses to overwrite an existing .agents/ directory or a root VERSION file with different content unless force is true.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"force": map[string]any{"type": "boolean", "description": "Overwrite an existing .agents/ directory or a divergent root VERSION file (default false)"},
					},
				},
			},
		},
		Execute: func(ctx context.Context, args map[string]any) (Result, error) {
			force, err := optBoolStrict(args, "force")
			if err != nil {
				return Result{}, err
			}
			dir, err := os.Getwd()
			if err != nil {
				return Result{}, err
			}
			if err := kspec.Load().Bootstrap(dir, force); err != nil {
				return Result{}, err
			}
			return Result{Output: fmt.Sprintf("bootstrapped kspec into %s: .agents/ (kspec-* skills, templates, rules), VERSION, spec/tasks/", dir)}, nil
		},
	}
}

func optBoolStrict(args map[string]any, key string) (bool, error) {
	v, ok := args[key]
	if !ok {
		return false, nil
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("argument %q must be a boolean", key)
	}
	return b, nil
}
