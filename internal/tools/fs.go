package tools

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"kterminal/internal/llm"
)

const maxReadBytes = 64 * 1024
const maxGlobResults = 200
const maxGrepResults = 100

func readTool() Tool {
	return Tool{
		Name:     "read",
		Mutating: false,
		Schema: llm.Tool{
			Type: "function",
			Function: llm.Function{
				Name:        "read",
				Description: "Read a file's contents from the filesystem",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{"type": "string", "description": "File path, relative to the working directory"},
					},
					"required": []string{"path"},
				},
			},
		},
		Execute: func(ctx context.Context, args map[string]any) (Result, error) {
			path, err := str(args, "path")
			if err != nil {
				return Result{}, err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return Result{}, err
			}
			if len(data) > maxReadBytes {
				return Result{Output: string(data[:maxReadBytes]) + "\n... (truncated)"}, nil
			}
			return Result{Output: string(data)}, nil
		},
	}
}

func globTool() Tool {
	return Tool{
		Name:     "glob",
		Mutating: false,
		Schema: llm.Tool{
			Type: "function",
			Function: llm.Function{
				Name:        "glob",
				Description: "Find files matching a glob pattern, e.g. \"**/*.go\"",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"pattern": map[string]any{"type": "string", "description": "Glob pattern"},
					},
					"required": []string{"pattern"},
				},
			},
		},
		Execute: func(ctx context.Context, args map[string]any) (Result, error) {
			pattern, err := str(args, "pattern")
			if err != nil {
				return Result{}, err
			}
			matches, err := filepath.Glob(pattern)
			if err != nil {
				return Result{}, err
			}
			sort.Strings(matches)
			if len(matches) > maxGlobResults {
				matches = matches[:maxGlobResults]
			}
			if len(matches) == 0 {
				return Result{Output: "no matches"}, nil
			}
			return Result{Output: strings.Join(matches, "\n")}, nil
		},
	}
}

func grepTool() Tool {
	return Tool{
		Name:     "grep",
		Mutating: false,
		Schema: llm.Tool{
			Type: "function",
			Function: llm.Function{
				Name:        "grep",
				Description: "Search file contents with a regular expression",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"pattern": map[string]any{"type": "string", "description": "Regular expression"},
						"path":    map[string]any{"type": "string", "description": "File or directory to search (default: .)"},
					},
					"required": []string{"pattern"},
				},
			},
		},
		Execute: func(ctx context.Context, args map[string]any) (Result, error) {
			pattern, err := str(args, "pattern")
			if err != nil {
				return Result{}, err
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				return Result{}, err
			}
			root := optStr(args, "path")
			if root == "" {
				root = "."
			}
			var out []string
			info, err := os.Stat(root)
			if err != nil {
				return Result{}, err
			}
			if !info.IsDir() {
				lines, err := grepFile(re, root)
				if err != nil {
					return Result{}, err
				}
				out = lines
			} else {
				err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
					if err != nil {
						return nil
					}
					if d.IsDir() {
						name := d.Name()
						if name == ".git" || name == "node_modules" || name == "vendor" {
							return filepath.SkipDir
						}
						return nil
					}
					lines, err := grepFile(re, path)
					if err != nil {
						return nil
					}
					out = append(out, lines...)
					return nil
				})
				if err != nil {
					return Result{}, err
				}
			}
			if len(out) > maxGrepResults {
				out = out[:maxGrepResults]
				out = append(out, "... (truncated)")
			}
			if len(out) == 0 {
				return Result{Output: "no matches"}, nil
			}
			return Result{Output: strings.Join(out, "\n")}, nil
		},
	}
}

func grepFile(re *regexp.Regexp, path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) > 2*1024*1024 {
		return nil, nil
	}
	var out []string
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if re.MatchString(line) {
			out = append(out, fmt.Sprintf("%s:%d: %s", path, i+1, strings.TrimRight(line, "\r")))
		}
	}
	return out, nil
}

func writeTool() Tool {
	return Tool{
		Name:     "write",
		Mutating: true,
		Schema: llm.Tool{
			Type: "function",
			Function: llm.Function{
				Name:        "write",
				Description: "Write contents to a file, creating it and parent directories if needed (overwrites)",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":    map[string]any{"type": "string", "description": "File path"},
						"content": map[string]any{"type": "string", "description": "Full file contents"},
					},
					"required": []string{"path", "content"},
				},
			},
		},
		Execute: func(ctx context.Context, args map[string]any) (Result, error) {
			path, err := str(args, "path")
			if err != nil {
				return Result{}, err
			}
			content, err := str(args, "content")
			if err != nil {
				return Result{}, err
			}
			existing, _ := os.ReadFile(path)
			if dir := filepath.Dir(path); dir != "." {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return Result{}, err
				}
			}
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				return Result{}, err
			}
			return Result{
				Output: fmt.Sprintf("wrote %d bytes to %s", len(content), path),
				Diff:   LineDiff(string(existing), content),
			}, nil
		},
		PendingDiff: pendingWriteDiff,
	}
}

func pendingWriteDiff(ctx context.Context, args map[string]any) ([]DiffLine, error) {
	path, err := str(args, "path")
	if err != nil {
		return nil, err
	}
	content, err := str(args, "content")
	if err != nil {
		return nil, err
	}
	existing, _ := os.ReadFile(path)
	return LineDiff(string(existing), content), nil
}

func editTool() Tool {
	return Tool{
		Name:     "edit",
		Mutating: true,
		Schema: llm.Tool{
			Type: "function",
			Function: llm.Function{
				Name:        "edit",
				Description: "Replace an exact, unique string in a file",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":       map[string]any{"type": "string", "description": "File path"},
						"old_string": map[string]any{"type": "string", "description": "Exact text to replace (must appear exactly once)"},
						"new_string": map[string]any{"type": "string", "description": "Replacement text"},
					},
					"required": []string{"path", "old_string", "new_string"},
				},
			},
		},
		Execute: func(ctx context.Context, args map[string]any) (Result, error) {
			path, oldStr, newStr, err := editParams(args)
			if err != nil {
				return Result{}, err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return Result{}, err
			}
			out, err := applyEdit(data, path, oldStr, newStr)
			if err != nil {
				return Result{}, err
			}
			if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
				return Result{}, err
			}
			return Result{
				Output: fmt.Sprintf("edited %s", path),
				Diff:   LineDiff(string(data), out),
			}, nil
		},
		PendingDiff: pendingEditDiff,
	}
}

func pendingEditDiff(ctx context.Context, args map[string]any) ([]DiffLine, error) {
	path, oldStr, newStr, err := editParams(args)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	simulated, err := applyEdit(data, path, oldStr, newStr)
	if err != nil {
		return nil, err
	}
	return LineDiff(string(data), simulated), nil
}

func editParams(args map[string]any) (path, oldStr, newStr string, err error) {
	path, err = str(args, "path")
	if err != nil {
		return "", "", "", err
	}
	oldStr, err = str(args, "old_string")
	if err != nil {
		return "", "", "", err
	}
	newStr, err = str(args, "new_string")
	if err != nil {
		return "", "", "", err
	}
	return path, oldStr, newStr, nil
}

func applyEdit(data []byte, path, oldStr, newStr string) (string, error) {
	count := strings.Count(string(data), oldStr)
	if count == 0 {
		return "", fmt.Errorf("old_string not found in %s", path)
	}
	if count > 1 {
		return "", fmt.Errorf("old_string appears %d times in %s; provide more context", count, path)
	}
	return strings.Replace(string(data), oldStr, newStr, 1), nil
}
