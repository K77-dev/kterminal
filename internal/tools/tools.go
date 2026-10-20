package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"kterminal/internal/llm"
	"kterminal/internal/procmon"
	"kterminal/internal/rtk"
)

type Result struct {
	Output string
	Diff   []DiffLine
}

type Executor func(ctx context.Context, args map[string]any) (Result, error)

type StreamExecutor func(ctx context.Context, args map[string]any, onLine func(string)) (Result, error)

type PendingDiffFunc func(ctx context.Context, args map[string]any) ([]DiffLine, error)

type Tool struct {
	Name          string
	Mutating      bool
	Schema        llm.Tool
	Execute       Executor
	ExecuteStream StreamExecutor
	PendingDiff   PendingDiffFunc
}

type Registry struct {
	tools    map[string]Tool
	order    []string
	procMon  *procmon.Manager
	rtkDet   *rtk.Detector
	OnGoEdit func(path string) string
}

func NewRegistry() *Registry {
	r := &Registry{tools: map[string]Tool{}}
	r.Register(readTool())
	r.Register(globTool())
	r.Register(grepTool())
	r.Register(writeTool())
	r.Register(editTool())
	r.Register(bashTool(nil))
	r.Register(kspecBootstrapTool())
	return r
}

func NewReadOnlyRegistry() *Registry {
	r := &Registry{tools: map[string]Tool{}}
	r.Register(readTool())
	r.Register(globTool())
	r.Register(grepTool())
	r.Register(graphifyTool())
	return r
}

func (r *Registry) Register(t Tool) {
	if _, ok := r.tools[t.Name]; !ok {
		r.order = append(r.order, t.Name)
	}
	r.tools[t.Name] = t
}

func (r *Registry) SetProcMon(pm *procmon.Manager) {
	r.procMon = pm
	if r.rtkDet != nil {
		r.Register(rtkBashTool(pm, r.rtkDet))
	} else {
		r.Register(bashTool(pm))
	}
}

func (r *Registry) SetRtkDetector(det *rtk.Detector) {
	r.rtkDet = det
	if r.procMon != nil {
		r.Register(rtkBashTool(r.procMon, det))
	} else {
		r.Register(rtkBashTool(nil, det))
	}
}

func (r *Registry) ProcMon() *procmon.Manager {
	return r.procMon
}

func (r *Registry) SetOnGoEdit(hook func(path string) string) {
	r.OnGoEdit = hook
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

func (r *Registry) ExecuteStream(ctx context.Context, name, argsJSON string, onLine func(string)) (Result, error) {
	t, ok := r.tools[name]
	if !ok {
		return Result{}, fmt.Errorf("unknown tool %q", name)
	}
	args, err := parseArgs(name, argsJSON)
	if err != nil {
		return Result{}, err
	}
	var res Result
	if onLine != nil && t.ExecuteStream != nil {
		res, err = t.ExecuteStream(ctx, args, onLine)
	} else {
		res, err = t.Execute(ctx, args)
	}
	if err != nil {
		return Result{}, fmt.Errorf("tool %s: %w", name, err)
	}
	if res.Output == "" {
		res.Output = "(no output)"
	}
	res.Output += r.goEditDiagnostics(name, args)
	return res, nil
}

func (r *Registry) goEditDiagnostics(name string, args map[string]any) string {
	if name != "write" && name != "edit" {
		return ""
	}
	if r.OnGoEdit == nil {
		return ""
	}
	path := optStr(args, "path")
	if !strings.HasSuffix(path, ".go") {
		return ""
	}
	return r.OnGoEdit(path)
}

func (r *Registry) Execute(ctx context.Context, name, argsJSON string) (Result, error) {
	return r.ExecuteStream(ctx, name, argsJSON, nil)
}

func (r *Registry) PendingDiff(ctx context.Context, name, argsJSON string) ([]DiffLine, error) {
	t, ok := r.tools[name]
	if !ok || t.PendingDiff == nil {
		return nil, nil
	}
	args, err := parseArgs(name, argsJSON)
	if err != nil {
		return nil, err
	}
	diff, err := t.PendingDiff(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("tool %s: %w", name, err)
	}
	return diff, nil
}

func parseArgs(name, argsJSON string) (map[string]any, error) {
	var args map[string]any
	if argsJSON == "" {
		return nil, nil
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return nil, fmt.Errorf("tool %s: bad arguments: %w", name, err)
	}
	return args, nil
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
