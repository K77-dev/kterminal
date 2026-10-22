package mcp

type ServerConfig struct {
	Name     string            `toml:"name"`
	Command  string            `toml:"command"`
	Args     []string          `toml:"args"`
	Env      map[string]string `toml:"env"`
	Timeout  string            `toml:"timeout"`
	ReadOnly bool              `toml:"read_only"`
}

type DiscoveredTool struct {
	ServerName string
	ToolName   string
	Namespaced string
	Definition toolDef
	ReadOnly   bool
}

type MCPServer struct {
	Name     string            `toml:"name"`
	Command  string            `toml:"command"`
	Args     []string          `toml:"args"`
	Env      map[string]string `toml:"env"`
	Timeout  string            `toml:"timeout"`
	ReadOnly bool              `toml:"read_only"`
}

type MCPConfig struct {
	Enabled *bool       `toml:"enabled"`
	Servers []MCPServer `toml:"servers"`
}

func (m MCPConfig) EnabledVal() bool {
	if m.Enabled == nil {
		return true
	}
	return *m.Enabled
}

func namespace(server, tool string) string {
	return "mcp__" + server + "__" + tool
}
