package mcp

import "testing"

func TestNewClient(t *testing.T) {
	c := newClient("test-server", "echo", []string{"hi"}, []string{"FOO=bar"}, 5000000000)
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	if c.name != "test-server" {
		t.Fatalf("expected name test-server, got %q", c.name)
	}
	if c.cmd != "echo" {
		t.Fatalf("expected cmd echo, got %q", c.cmd)
	}
	if len(c.args) != 1 || c.args[0] != "hi" {
		t.Fatalf("expected args [hi], got %v", c.args)
	}
	if len(c.env) != 1 || c.env[0] != "FOO=bar" {
		t.Fatalf("expected env [FOO=bar], got %v", c.env)
	}
	if c.timeout != 5000000000 {
		t.Fatalf("expected timeout 5s, got %v", c.timeout)
	}
	if c.pending == nil {
		t.Fatal("expected non-nil pending map")
	}
	if c.done == nil {
		t.Fatal("expected non-nil done channel")
	}
}
