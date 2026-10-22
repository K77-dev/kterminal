package mcp

import (
	"context"
	"log"
	"sync"
	"time"
)

type Manager struct {
	configs   []ServerConfig
	mu        sync.Mutex
	servers   map[string]*Client
	tools     map[string]DiscoveredTool
	toolOrder []string
	started   bool
}

func NewManager(servers []ServerConfig) *Manager {
	return &Manager{
		configs: servers,
		servers: map[string]*Client{},
		tools:   map[string]DiscoveredTool{},
	}
}

func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return nil
	}

	seen := map[string]bool{}
	for _, cfg := range m.configs {
		if seen[cfg.Name] {
			m.mu.Unlock()
			return nil
		}
		seen[cfg.Name] = true
	}
	m.mu.Unlock()

	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, cfg := range m.configs {
		wg.Add(1)
		go func(cfg ServerConfig) {
			defer wg.Done()

			timeout := parseTimeout(cfg.Timeout)
			env := flattenEnv(cfg.Env)

			client := newClient(cfg.Name, cfg.Command, cfg.Args, env, timeout)
			if err := client.start(ctx); err != nil {
				log.Printf("mcp: server %q failed to start: %v", cfg.Name, err)
				return
			}

			tools, err := client.listTools(ctx)
			if err != nil {
				log.Printf("mcp: server %q failed to list tools: %v", cfg.Name, err)
				client.close()
				return
			}

			mu.Lock()
			m.servers[cfg.Name] = client
			for _, t := range tools {
				ns := namespace(cfg.Name, t.Name)
				dt := DiscoveredTool{
					ServerName: cfg.Name,
					ToolName:   t.Name,
					Namespaced: ns,
					Definition: t,
					ReadOnly:   cfg.ReadOnly,
				}
				m.tools[ns] = dt
				m.toolOrder = append(m.toolOrder, ns)
			}
			mu.Unlock()
		}(cfg)
	}

	wg.Wait()

	m.mu.Lock()
	m.started = true
	m.mu.Unlock()

	return nil
}

func (m *Manager) Tools() []DiscoveredTool {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]DiscoveredTool, 0, len(m.toolOrder))
	for _, ns := range m.toolOrder {
		if dt, ok := m.tools[ns]; ok {
			result = append(result, dt)
		}
	}
	return result
}

func (m *Manager) Client(serverName string) (*Client, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.servers[serverName]
	return c, ok
}

func (m *Manager) Stop() {
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return
	}
	servers := make([]*Client, 0, len(m.servers))
	for _, c := range m.servers {
		servers = append(servers, c)
	}
	m.mu.Unlock()

	var wg sync.WaitGroup
	for _, c := range servers {
		wg.Add(1)
		go func(c *Client) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = c.shutdown(ctx)
			_ = c.close()
		}(c)
	}
	wg.Wait()
}

func (m *Manager) Started() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.started
}

func parseTimeout(s string) time.Duration {
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}

func flattenEnv(env map[string]string) []string {
	result := make([]string, 0, len(env))
	for k, v := range env {
		result = append(result, k+"="+v)
	}
	return result
}
