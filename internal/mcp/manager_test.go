package mcp

import (
	"context"
	"testing"
)

func TestNewManager(t *testing.T) {
	servers := []ServerConfig{
		{Name: "alpha", Command: "echo"},
		{Name: "beta", Command: "echo"},
	}
	mgr := NewManager(servers)
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
	if len(mgr.configs) != 2 {
		t.Fatalf("expected 2 configs, got %d", len(mgr.configs))
	}
	if mgr.servers == nil {
		t.Fatal("expected non-nil servers map")
	}
	if mgr.tools == nil {
		t.Fatal("expected non-nil tools map")
	}
}

func TestManagerStartNoServers(t *testing.T) {
	mgr := NewManager(nil)
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start with no servers: %v", err)
	}
	if len(mgr.Tools()) != 0 {
		t.Fatalf("expected 0 tools, got %d", len(mgr.Tools()))
	}
	if !mgr.Started() {
		t.Fatal("expected started=true after Start")
	}
}

func TestManagerStopIdempotent(t *testing.T) {
	mgr := NewManager(nil)
	mgr.Start(context.Background())
	mgr.Stop()
	mgr.Stop()
}
