package squad

import "fmt"

type Kickoff struct {
	Roles           []string
	MaxConvocations int
	TokenBudget     int64
	ExitCriterion   string
}

type Limits struct {
	MaxConvocations int
	TokenBudget     int64
}

const (
	DefaultMaxConvocations = 8
	DefaultTokenBudget     = 200000
)

func DefaultLimits() Limits {
	return Limits{MaxConvocations: DefaultMaxConvocations, TokenBudget: DefaultTokenBudget}
}

func ValidateKickoff(k Kickoff, store *Store, limits Limits) (Kickoff, error) {
	if len(k.Roles) == 0 {
		return k, fmt.Errorf("kickoff: roles must not be empty")
	}
	available := map[string]bool{}
	for _, p := range store.List() {
		available[p.Name] = true
	}
	for _, role := range k.Roles {
		if !available[role] {
			return k, fmt.Errorf("kickoff: unknown role %q", role)
		}
	}
	if k.MaxConvocations <= 0 {
		k.MaxConvocations = limits.MaxConvocations
	}
	if k.MaxConvocations > limits.MaxConvocations {
		k.MaxConvocations = limits.MaxConvocations
	}
	if k.TokenBudget <= 0 {
		k.TokenBudget = limits.TokenBudget
	}
	if k.TokenBudget > limits.TokenBudget {
		k.TokenBudget = limits.TokenBudget
	}
	if k.ExitCriterion == "" {
		k.ExitCriterion = "all personas agree on the plan"
	}
	return k, nil
}
