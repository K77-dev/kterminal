package mcp

import (
	"testing"

	"kterminal/internal/tools"
)

func TestExtractText(t *testing.T) {
	result := toolCallResult{
		Content: []contentBlock{
			{Type: "text", Text: "hello"},
			{Type: "text", Text: "world"},
			{Type: "image"},
		},
	}
	got := extractText(result)
	want := "hello\nworld"
	if got != want {
		t.Fatalf("extractText = %q, want %q", got, want)
	}
}

func TestExtractTextNoTextBlocks(t *testing.T) {
	result := toolCallResult{
		Content: []contentBlock{
			{Type: "image"},
			{Type: "resource"},
		},
	}
	got := extractText(result)
	if got != "" {
		t.Fatalf("extractText = %q, want empty", got)
	}
}

func TestExtractTextEmpty(t *testing.T) {
	result := toolCallResult{}
	got := extractText(result)
	if got != "" {
		t.Fatalf("extractText = %q, want empty", got)
	}
}

func TestRegisterToolsNamespaced(t *testing.T) {
	mgr := &Manager{
		servers:   map[string]*Client{},
		tools:     map[string]DiscoveredTool{},
		toolOrder: []string{},
	}
	mgr.tools["mcp__alpha__search"] = DiscoveredTool{
		ServerName: "alpha",
		ToolName:   "search",
		Namespaced: "mcp__alpha__search",
		Definition: toolDef{
			Name:        "search",
			Description: "Search the web",
			InputSchema: map[string]any{"type": "object"},
		},
		ReadOnly: false,
	}
	mgr.toolOrder = append(mgr.toolOrder, "mcp__alpha__search")

	reg := tools.NewRegistry()
	RegisterTools(reg, mgr)

	defs := reg.Definitions()
	found := false
	for _, d := range defs {
		if d.Function.Name == "mcp__alpha__search" {
			found = true
			if d.Function.Description != "Search the web" {
				t.Fatalf("expected description 'Search the web', got %q", d.Function.Description)
			}
			if d.Type != "function" {
				t.Fatalf("expected type 'function', got %q", d.Type)
			}
		}
	}
	if !found {
		t.Fatal("expected mcp__alpha__search in registry definitions")
	}
}

func TestRegisterToolsMutatingDefault(t *testing.T) {
	mgr := &Manager{
		servers:   map[string]*Client{},
		tools:     map[string]DiscoveredTool{},
		toolOrder: []string{},
	}
	mgr.tools["mcp__alpha__write"] = DiscoveredTool{
		ServerName: "alpha",
		ToolName:   "write",
		Namespaced: "mcp__alpha__write",
		Definition: toolDef{
			Name:        "write",
			Description: "Write data",
			InputSchema: map[string]any{"type": "object"},
		},
		ReadOnly: false,
	}
	mgr.toolOrder = append(mgr.toolOrder, "mcp__alpha__write")

	reg := tools.NewRegistry()
	RegisterTools(reg, mgr)

	if !reg.IsMutating("mcp__alpha__write") {
		t.Fatal("expected mcp__alpha__write to be mutating")
	}
}

func TestRegisterToolsReadOnly(t *testing.T) {
	mgr := &Manager{
		servers:   map[string]*Client{},
		tools:     map[string]DiscoveredTool{},
		toolOrder: []string{},
	}
	mgr.tools["mcp__beta__read"] = DiscoveredTool{
		ServerName: "beta",
		ToolName:   "read",
		Namespaced: "mcp__beta__read",
		Definition: toolDef{
			Name:        "read",
			Description: "Read data",
			InputSchema: map[string]any{"type": "object"},
		},
		ReadOnly: true,
	}
	mgr.toolOrder = append(mgr.toolOrder, "mcp__beta__read")

	reg := tools.NewRegistry()
	RegisterTools(reg, mgr)

	if reg.IsMutating("mcp__beta__read") {
		t.Fatal("expected mcp__beta__read to NOT be mutating")
	}
}
