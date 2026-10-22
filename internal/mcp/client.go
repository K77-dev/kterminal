package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type Client struct {
	name    string
	cmd     string
	args    []string
	env     []string
	timeout time.Duration

	mu      sync.Mutex
	writeMu sync.Mutex
	proc    *os.Process
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	nextID  int64
	pending map[int64]chan *rpcResponse
	started bool
	done    chan struct{}
	err     error
}

func newClient(name, cmd string, args, env []string, timeout time.Duration) *Client {
	return &Client{
		name:    name,
		cmd:     cmd,
		args:    args,
		env:     env,
		timeout: timeout,
		pending: map[int64]chan *rpcResponse{},
		done:    make(chan struct{}),
	}
}

func (c *Client) start(ctx context.Context) error {
	c.mu.Lock()
	if c.started {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	command := exec.Command(c.cmd, c.args...)
	command.Env = append(os.Environ(), c.env...)

	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdin, err := command.StdinPipe()
	if err != nil {
		return fmt.Errorf("mcp %s: stdin pipe: %w", c.name, err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		stdin.Close()
		return fmt.Errorf("mcp %s: stdout pipe: %w", c.name, err)
	}

	if err := command.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		return fmt.Errorf("mcp %s: start process: %w", c.name, err)
	}

	c.mu.Lock()
	c.proc = command.Process
	c.stdin = stdin
	c.stdout = stdout
	c.started = true
	c.mu.Unlock()

	go c.readLoop()

	initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := c.initialize(initCtx); err != nil {
		c.close()
		return fmt.Errorf("mcp %s: initialize: %w", c.name, err)
	}

	return nil
}

func (c *Client) initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": "2024-11-05",
		"clientInfo": map[string]any{
			"name":    "kterminal",
			"version": "1.0",
		},
		"capabilities": map[string]any{},
	}
	_, err := c.call(ctx, "initialize", params)
	if err != nil {
		return err
	}

	notif := &rpcNotification{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}

	c.writeMu.Lock()
	err = encodeNotification(c.stdin, notif)
	c.writeMu.Unlock()
	if err != nil {
		return fmt.Errorf("send initialized notification: %w", err)
	}

	return nil
}

func (c *Client) call(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	id := atomic.AddInt64(&c.nextID, 1)

	ch := make(chan *rpcResponse, 1)

	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	req := &rpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	c.writeMu.Lock()
	err := encodeRequest(c.stdin, req)
	c.writeMu.Unlock()
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("encode request: %w", err)
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	}
}

func (c *Client) listTools(ctx context.Context) ([]toolDef, error) {
	result, err := c.call(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}
	var tlr toolsListResult
	if err := json.Unmarshal(result, &tlr); err != nil {
		return nil, fmt.Errorf("unmarshal tools/list: %w", err)
	}
	return tlr.Tools, nil
}

func (c *Client) callTool(ctx context.Context, name string, args map[string]any) (toolCallResult, error) {
	callCtx := ctx
	cancel := func() {}
	if c.timeout > 0 {
		callCtx, cancel = context.WithTimeout(ctx, c.timeout)
	}
	defer cancel()

	params := map[string]any{
		"name":      name,
		"arguments": args,
	}
	result, err := c.call(callCtx, "tools/call", params)
	if err != nil {
		return toolCallResult{}, err
	}
	var tcr toolCallResult
	if err := json.Unmarshal(result, &tcr); err != nil {
		return toolCallResult{}, fmt.Errorf("unmarshal tools/call: %w", err)
	}
	if tcr.IsError {
		return tcr, fmt.Errorf("mcp tool %s: %s", name, extractText(tcr))
	}
	return tcr, nil
}

func (c *Client) shutdown(ctx context.Context) error {
	notif := &rpcNotification{
		JSONRPC: "2.0",
		Method:  "shutdown",
	}
	c.writeMu.Lock()
	err := encodeNotification(c.stdin, notif)
	c.writeMu.Unlock()
	return err
}

func (c *Client) close() error {
	c.mu.Lock()
	if c.proc == nil {
		c.mu.Unlock()
		return nil
	}
	proc := c.proc
	stdin := c.stdin
	c.proc = nil
	c.stdin = nil
	c.mu.Unlock()

	if stdin != nil {
		stdin.Close()
	}

	done := make(chan error, 1)
	go func() {
		_, err := proc.Wait()
		done <- err
	}()

	select {
	case <-done:
		return nil
	case <-time.After(3 * time.Second):
		_ = syscall.Kill(-proc.Pid, syscall.SIGKILL)
		<-done
		return nil
	}
}

func (c *Client) readLoop() {
	scanner := bufio.NewScanner(c.stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), scannerBufferSize)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		resp, err := decodeResponse(line)
		if err != nil {
			continue
		}

		c.mu.Lock()
		ch, ok := c.pending[resp.ID]
		if ok {
			delete(c.pending, resp.ID)
		}
		c.mu.Unlock()

		if ok {
			select {
			case ch <- &resp:
			default:
			}
		}
	}

	c.mu.Lock()
	c.err = scanner.Err()
	if c.err == nil {
		c.err = io.EOF
	}
	for id, ch := range c.pending {
		delete(c.pending, id)
		errResp := &rpcResponse{
			JSONRPC: "2.0",
			ID:      id,
			Error: &rpcError{
				Code:    errInternalError,
				Message: "connection closed",
			},
		}
		select {
		case ch <- errResp:
		default:
		}
	}
	close(c.done)
	c.mu.Unlock()
}

func (c *Client) alive() bool {
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}
