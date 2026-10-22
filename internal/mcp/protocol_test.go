package mcp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestEncodeRequest(t *testing.T) {
	var buf bytes.Buffer
	req := &rpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
	}
	if err := encodeRequest(&buf, req); err != nil {
		t.Fatalf("encodeRequest: %v", err)
	}
	out := buf.Bytes()
	if !strings.HasSuffix(string(out), "\n") {
		t.Fatal("expected trailing newline")
	}
	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if decoded["jsonrpc"] != "2.0" {
		t.Fatalf("expected jsonrpc 2.0, got %v", decoded["jsonrpc"])
	}
	if decoded["method"] != "tools/list" {
		t.Fatalf("expected method tools/list, got %v", decoded["method"])
	}
	if decoded["id"].(float64) != 1 {
		t.Fatalf("expected id 1, got %v", decoded["id"])
	}
}

func TestEncodeNotification(t *testing.T) {
	var buf bytes.Buffer
	n := &rpcNotification{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	if err := encodeNotification(&buf, n); err != nil {
		t.Fatalf("encodeNotification: %v", err)
	}
	out := buf.Bytes()
	if !strings.HasSuffix(string(out), "\n") {
		t.Fatal("expected trailing newline")
	}
	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := decoded["id"]; ok {
		t.Fatal("notification should not have id field")
	}
	if decoded["jsonrpc"] != "2.0" {
		t.Fatalf("expected jsonrpc 2.0, got %v", decoded["jsonrpc"])
	}
}

func TestDecodeResponse(t *testing.T) {
	raw := `{"jsonrpc":"2.0","id":42,"result":{"tools":[]}}`
	resp, err := decodeResponse([]byte(raw))
	if err != nil {
		t.Fatalf("decodeResponse: %v", err)
	}
	if resp.ID != 42 {
		t.Fatalf("expected id 42, got %d", resp.ID)
	}
	if resp.Result == nil {
		t.Fatal("expected non-nil result")
	}
	var tlr toolsListResult
	if err := json.Unmarshal(resp.Result, &tlr); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(tlr.Tools) != 0 {
		t.Fatalf("expected 0 tools, got %d", len(tlr.Tools))
	}
}

func TestDecodeErrorResponse(t *testing.T) {
	raw := `{"jsonrpc":"2.0","id":5,"error":{"code":-32601,"message":"method not found"}}`
	resp, err := decodeResponse([]byte(raw))
	if err != nil {
		t.Fatalf("decodeResponse: %v", err)
	}
	if resp.ID != 5 {
		t.Fatalf("expected id 5, got %d", resp.ID)
	}
	if resp.Error == nil {
		t.Fatal("expected non-nil error")
	}
	if resp.Error.Code != -32601 {
		t.Fatalf("expected code -32601, got %d", resp.Error.Code)
	}
	if resp.Error.Message != "method not found" {
		t.Fatalf("expected message 'method not found', got %q", resp.Error.Message)
	}
}

func TestDecodeMalformedJSON(t *testing.T) {
	raw := `{not valid json`
	_, err := decodeResponse([]byte(raw))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestDecodeEmptyLine(t *testing.T) {
	for _, raw := range []string{"", "   ", "\n"} {
		_, err := decodeResponse([]byte(raw))
		if err == nil {
			t.Fatalf("expected error for empty/whitespace line %q", raw)
		}
	}
}
