package mcp

import "testing"

func TestNamespace(t *testing.T) {
	tests := []struct {
		server string
		tool   string
		want   string
	}{
		{"alpha", "search", "mcp__alpha__search"},
		{"my-server", "my-tool", "mcp__my-server__my-tool"},
		{"", "", "mcp____"},
	}
	for _, tt := range tests {
		got := namespace(tt.server, tt.tool)
		if got != tt.want {
			t.Fatalf("namespace(%q, %q) = %q, want %q", tt.server, tt.tool, got, tt.want)
		}
	}
}

func TestMCPConfigEnabledVal(t *testing.T) {
	tests := []struct {
		name    string
		enabled *bool
		want    bool
	}{
		{"nil", nil, true},
		{"true", ptrBool(true), true},
		{"false", ptrBool(false), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := MCPConfig{Enabled: tt.enabled}
			got := cfg.EnabledVal()
			if got != tt.want {
				t.Fatalf("EnabledVal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func ptrBool(b bool) *bool {
	return &b
}
