package tools

import (
	"encoding/json"
	"fmt"

	"kterminal/internal/llm"
)

type Executor func(args map[string]any) (string, error)

type Tool struct {
	Name     string
	Mutating bool
	Schema   llm.Tool
	Execute  Executor
}

type Registry struct {
	tools map[string]Tool
	order []string
}

func NewRegistry() *Registry {
	r := &Registry{tools: map[string]Tool{}}
	r.register(readTool())
	r.register(globTool())
	r.register(grepTool())
	r.register(writeTool())
	r.register(editTool())
	r.register(bashTool())
	return r
}

func (r *Registry) register(t Tool) {
	r.tools[t.Name] = t
	r.order = append(r.order, t.Name)
}

func (r *Registry) Definitions() []llm.Tool {
	defs := make([]llm.Tool, 0, len(r.order))
	for _, name := range r.order {
		defs = append(defs, r.tools[name].Schema)
	}
	return defs
}

func (r *Registry) IsMutating(name string) bool {
	t, ok := r.tools[name]
	return ok && t.Mutating
}

func (r *Registry) Execute(name, argsJSON string) (string, error) {
	t, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("unknown tool %q", name)
	}
	var args map[string]any
	if argsJSON != "" {
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", fmt.Errorf("tool %s: bad arguments: %w", name, err)
		}
	}
	out, err := t.Execute(args)
	if err != nil {
		return "", fmt.Errorf("tool %s: %w", name, err)
	}
	if out == "" {
		out = "(no output)"
	}
	return out, nil
}

func str(args map[string]any, key string) (string, error) {
	v, ok := args[key]
	if !ok {
		return "", fmt.Errorf("missing argument %q", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("argument %q must be a string", key)
	}
	return s, nil
}

func optStr(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}
