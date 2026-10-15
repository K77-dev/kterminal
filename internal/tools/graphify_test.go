package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func graphifyToolWithGraph(t *testing.T, run func(ctx context.Context, args []string) (string, error)) Tool {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "graphify-out"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "graphify-out", "graph.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	return newGraphifyTool(run)
}

func TestGraphifyToolNoGraphFallsBack(t *testing.T) {
	t.Chdir(t.TempDir())
	tool := newGraphifyTool(func(ctx context.Context, args []string) (string, error) {
		t.Fatal("runner must not be called without a graph")
		return "", nil
	})
	res, err := tool.Execute(context.Background(), map[string]any{"command": "query", "question": "where is auth?"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(res.Output, "no knowledge graph") {
		t.Fatalf("output = %q, want a no-graph fallback message", res.Output)
	}
}

func TestGraphifyToolQuery(t *testing.T) {
	var got []string
	tool := graphifyToolWithGraph(t, func(ctx context.Context, args []string) (string, error) {
		got = args
		return "NODE Agent\n", nil
	})
	res, err := tool.Execute(context.Background(), map[string]any{"command": "query", "question": "where is auth?"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.Join(got, " ") != "query where is auth?" {
		t.Fatalf("args = %v, want [query where is auth?]", got)
	}
	if res.Output != "NODE Agent" {
		t.Fatalf("output = %q, want the runner output trimmed", res.Output)
	}
}

func TestGraphifyToolPath(t *testing.T) {
	var got []string
	tool := graphifyToolWithGraph(t, func(ctx context.Context, args []string) (string, error) {
		got = args
		return "path output", nil
	})
	res, err := tool.Execute(context.Background(), map[string]any{"command": "path", "source": "Agent", "target": "Mesa"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.Join(got, " ") != "path Agent Mesa" {
		t.Fatalf("args = %v, want [path Agent Mesa]", got)
	}
	if res.Output != "path output" {
		t.Fatalf("output = %q, want the runner output", res.Output)
	}
}

func TestGraphifyToolExplain(t *testing.T) {
	var got []string
	tool := graphifyToolWithGraph(t, func(ctx context.Context, args []string) (string, error) {
		got = args
		return "explain output", nil
	})
	_, err := tool.Execute(context.Background(), map[string]any{"command": "explain", "node": "Agent"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.Join(got, " ") != "explain Agent" {
		t.Fatalf("args = %v, want [explain Agent]", got)
	}
}

func TestGraphifyToolValidation(t *testing.T) {
	tool := graphifyToolWithGraph(t, func(ctx context.Context, args []string) (string, error) {
		return "unused", nil
	})
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"query without question", map[string]any{"command": "query"}, "query requires a question"},
		{"path without target", map[string]any{"command": "path", "source": "Agent"}, "path requires source and target"},
		{"explain without node", map[string]any{"command": "explain"}, "explain requires a node"},
		{"unknown command", map[string]any{"command": "delete", "node": "x"}, "unknown command"},
	}
	for _, tc := range cases {
		_, err := tool.Execute(context.Background(), tc.args)
		if err == nil {
			t.Errorf("%s: expected error", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error = %q, want it to contain %q", tc.name, err.Error(), tc.want)
		}
	}
}

func TestGraphifyToolTruncatesOutput(t *testing.T) {
	tool := graphifyToolWithGraph(t, func(ctx context.Context, args []string) (string, error) {
		return strings.Repeat("a", maxGraphifyOutput+100), nil
	})
	res, err := tool.Execute(context.Background(), map[string]any{"command": "query", "question": "q"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.HasSuffix(res.Output, "... (truncated)") {
		t.Fatalf("output must end with the truncation marker, got %d bytes", len(res.Output))
	}
	if len(res.Output) > maxGraphifyOutput+len("\n... (truncated)") {
		t.Fatalf("output = %d bytes, want at most %d", len(res.Output), maxGraphifyOutput+len("\n... (truncated)"))
	}
}

func TestGraphifyToolCLIMissingFallsBack(t *testing.T) {
	tool := graphifyToolWithGraph(t, func(ctx context.Context, args []string) (string, error) {
		return "", fmt.Errorf("run: %w", exec.ErrNotFound)
	})
	res, err := tool.Execute(context.Background(), map[string]any{"command": "query", "question": "q"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(res.Output, "graphify CLI not available") {
		t.Fatalf("output = %q, want a CLI-missing fallback message", res.Output)
	}
}

func TestGraphifyToolTimeoutReportsPartialOutput(t *testing.T) {
	tool := graphifyToolWithGraph(t, func(ctx context.Context, args []string) (string, error) {
		return "partial", context.DeadlineExceeded
	})
	res, err := tool.Execute(context.Background(), map[string]any{"command": "query", "question": "q"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(res.Output, "timed out") || !strings.Contains(res.Output, "partial") {
		t.Fatalf("output = %q, want a timeout message with the partial output", res.Output)
	}
}

func TestGraphifyToolExitErrorIncludesOutput(t *testing.T) {
	exitErr := exec.Command("sh", "-c", "exit 3").Run()
	var exitTarget *exec.ExitError
	if !errors.As(exitErr, &exitTarget) {
		t.Skipf("could not produce an ExitError on this platform: %v", exitErr)
	}
	tool := graphifyToolWithGraph(t, func(ctx context.Context, args []string) (string, error) {
		return "cli error detail", exitErr
	})
	res, err := tool.Execute(context.Background(), map[string]any{"command": "query", "question": "q"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(res.Output, "exit status") || !strings.Contains(res.Output, "cli error detail") {
		t.Fatalf("output = %q, want exit status and the CLI output", res.Output)
	}
}

func TestGraphifyToolEmptyOutput(t *testing.T) {
	tool := graphifyToolWithGraph(t, func(ctx context.Context, args []string) (string, error) {
		return "   ", nil
	})
	res, err := tool.Execute(context.Background(), map[string]any{"command": "query", "question": "q"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Output != "(no output)" {
		t.Fatalf("output = %q, want (no output)", res.Output)
	}
}
