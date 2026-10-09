package tools

import (
	"context"
	"fmt"

	"kterminal/internal/llm"
	"kterminal/internal/squad"
)

const SquadKickoffToolName = "squad_kickoff"

func SquadKickoffTool(store *squad.Store, limits squad.Limits, register func(squad.Kickoff)) Tool {
	return Tool{
		Name: SquadKickoffToolName,
		Schema: llm.Tool{
			Type: "function",
			Function: llm.Function{
				Name:        SquadKickoffToolName,
				Description: "Register the squad mesa (roles, limits and exit criterion) before any persona convocation. The Go layer validates roles and clamps the limits to the configured maxima.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"roles": map[string]any{
							"type":        "array",
							"description": "Personas relevant to the problem (subset of the persona catalog)",
							"items":       map[string]any{"type": "string"},
						},
						"max_convocations": map[string]any{"type": "integer", "description": "Maximum number of convocations for the mesa"},
						"token_budget":     map[string]any{"type": "integer", "description": "Token budget for the entire mesa"},
						"exit_criterion":   map[string]any{"type": "string", "description": "Binary condition that signals convergence"},
					},
					"required": []string{"roles"},
				},
			},
		},
		Execute: func(ctx context.Context, args map[string]any) (Result, error) {
			roles, err := parseRoles(args)
			if err != nil {
				return Result{}, err
			}
			k := squad.Kickoff{
				Roles:           roles,
				MaxConvocations: intArg(args, "max_convocations"),
				TokenBudget:     int64(intArg(args, "token_budget")),
				ExitCriterion:   optStr(args, "exit_criterion"),
			}
			validated, err := squad.ValidateKickoff(k, store, limits)
			if err != nil {
				return Result{}, err
			}
			register(validated)
			return Result{Output: formatKickoff(validated)}, nil
		},
	}
}

func parseRoles(args map[string]any) ([]string, error) {
	raw, ok := args["roles"]
	if !ok {
		return nil, fmt.Errorf("missing argument %q", "roles")
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("argument %q must be an array", "roles")
	}
	roles := make([]string, 0, len(list))
	for i, item := range list {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("roles[%d] must be a string", i)
		}
		roles = append(roles, s)
	}
	return roles, nil
}

func intArg(args map[string]any, key string) int {
	v, ok := args[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	default:
		return 0
	}
}

func formatKickoff(k squad.Kickoff) string {
	return fmt.Sprintf("mesa registered: roles=%v max_convocations=%d token_budget=%d exit_criterion=%q",
		k.Roles, k.MaxConvocations, k.TokenBudget, k.ExitCriterion)
}
