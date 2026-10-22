package mcp

import (
	"context"
	"strings"

	"kterminal/internal/llm"
	"kterminal/internal/tools"
)

func RegisterTools(reg *tools.Registry, mgr *Manager) []tools.Tool {
	discovered := mgr.Tools()
	registered := make([]tools.Tool, 0, len(discovered))

	for _, dt := range discovered {
		dt := dt
		t := tools.Tool{
			Name:     dt.Namespaced,
			Mutating: !dt.ReadOnly,
			Schema: llm.Tool{
				Type: "function",
				Function: llm.Function{
					Name:        dt.Namespaced,
					Description: dt.Definition.Description,
					Parameters:  dt.Definition.InputSchema,
				},
			},
			Execute: func(ctx context.Context, args map[string]any) (tools.Result, error) {
				client, ok := mgr.Client(dt.ServerName)
				if !ok {
					return tools.Result{}, nil
				}
				result, err := client.callTool(ctx, dt.ToolName, args)
				if err != nil {
					return tools.Result{}, err
				}
				return tools.Result{Output: extractText(result)}, nil
			},
		}
		reg.Register(t)
		registered = append(registered, t)
	}

	return registered
}

func extractText(result toolCallResult) string {
	var parts []string
	for _, block := range result.Content {
		if block.Type == "text" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}
