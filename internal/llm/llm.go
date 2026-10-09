package llm

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Function struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type Tool struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

type ToolCall struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Function FuncCall `json:"function"`
}

type FuncCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ImageURL struct {
	URL string `json:"url,omitempty"`
}

type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

type Message struct {
	Role         string        `json:"role"`
	Content      string        `json:"content"`
	ContentParts []ContentPart `json:"-"`
	ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
	ToolCallID   string        `json:"tool_call_id,omitempty"`
}

func (m Message) MarshalJSON() ([]byte, error) {
	if len(m.ContentParts) == 0 {
		type plain Message
		return json.Marshal(plain(m))
	}
	return json.Marshal(struct {
		Role       string        `json:"role"`
		Content    []ContentPart `json:"content"`
		ToolCalls  []ToolCall    `json:"tool_calls,omitempty"`
		ToolCallID string        `json:"tool_call_id,omitempty"`
	}{
		Role:       m.Role,
		Content:    m.ContentParts,
		ToolCalls:  m.ToolCalls,
		ToolCallID: m.ToolCallID,
	})
}

func (m *Message) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	var raw struct {
		Role       string          `json:"role"`
		Content    json.RawMessage `json:"content"`
		ToolCalls  []ToolCall      `json:"tool_calls"`
		ToolCallID string          `json:"tool_call_id"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	m.Role = raw.Role
	m.ToolCalls = raw.ToolCalls
	m.ToolCallID = raw.ToolCallID
	m.Content = ""
	m.ContentParts = nil
	content := bytes.TrimSpace(raw.Content)
	if len(content) == 0 {
		return nil
	}
	if content[0] == '[' {
		return json.Unmarshal(content, &m.ContentParts)
	}
	return json.Unmarshal(content, &m.Content)
}

type Usage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
}

type StreamResult struct {
	Content       string
	ToolCalls     []ToolCall
	Usage         Usage
	FinishReason  string
	StreamSeconds float64
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type chatRequest struct {
	Model         string         `json:"model"`
	Messages      []Message      `json:"messages"`
	Tools         []Tool         `json:"tools,omitempty"`
	Stream        bool           `json:"stream"`
	StreamOptions *streamOptions `json:"stream_options,omitempty"`
}

type deltaToolCall struct {
	Index    int      `json:"index"`
	ID       string   `json:"id"`
	Function FuncCall `json:"function"`
}

type delta struct {
	Content      string          `json:"content"`
	ToolCalls    []deltaToolCall `json:"tool_calls"`
	Role         string          `json:"role"`
	FinishReason string          `json:"finish_reason"`
}

type chunkChoice struct {
	Delta delta `json:"delta"`
}

type streamChunk struct {
	Choices []chunkChoice `json:"choices"`
	Usage   *Usage        `json:"usage"`
}

var (
	ErrStreamIdle = errors.New("stream stalled")
	ErrFirstByte  = errors.New("no response headers")
)

type Limits struct {
	RequestTimeout   time.Duration
	IdleTimeout      time.Duration
	FirstByteTimeout time.Duration
	MaxRetries       int
}

var defaultLimits = Limits{
	RequestTimeout:   10 * time.Minute,
	IdleTimeout:      5 * time.Minute,
	FirstByteTimeout: 60 * time.Second,
	MaxRetries:       2,
}

type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	limits     Limits
	retryBase  time.Duration
}

func normalizeBaseURL(u string) string {
	u = strings.TrimRight(u, "/")
	if strings.HasSuffix(u, "/v1") {
		u = strings.TrimSuffix(u, "/v1")
	}
	return u
}

func New(baseURL, apiKey string, skipTLSVerify bool) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if skipTLSVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &Client{
		BaseURL: normalizeBaseURL(baseURL),
		APIKey:  apiKey,
		HTTPClient: &http.Client{
			Timeout:   defaultLimits.RequestTimeout,
			Transport: transport,
		},
		limits:    defaultLimits,
		retryBase: time.Second,
	}
}

func (c *Client) SetLimits(l Limits) {
	c.limits = l
	c.HTTPClient.Timeout = l.RequestTimeout
}

func (c *Client) do(ctx context.Context, path string, body any) (*http.Response, context.CancelFunc, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, nil, err
	}
	reqCtx, cancel := context.WithCancel(ctx)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, c.BaseURL+path, bytes.NewReader(data))
	if err != nil {
		cancel()
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	var timer *time.Timer
	if c.limits.FirstByteTimeout > 0 {
		timer = time.AfterFunc(c.limits.FirstByteTimeout, cancel)
	}
	resp, err := c.HTTPClient.Do(req)
	expired := timer != nil && !timer.Stop()
	if err != nil {
		cancel()
		if expired && ctx.Err() == nil {
			return nil, nil, fmt.Errorf("%w within %s — check the gateway", ErrFirstByte, c.limits.FirstByteTimeout)
		}
		return nil, nil, err
	}
	if expired && ctx.Err() == nil {
		cancel()
		resp.Body.Close()
		return nil, nil, fmt.Errorf("%w within %s — check the gateway", ErrFirstByte, c.limits.FirstByteTimeout)
	}
	return resp, cancel, nil
}

type idleBody struct {
	rc      io.ReadCloser
	idle    time.Duration
	cancel  context.CancelFunc
	timer   *time.Timer
	mu      sync.Mutex
	last    time.Time
	expired bool
	closed  bool
}

func newIdleBody(rc io.ReadCloser, idle time.Duration, cancel context.CancelFunc) *idleBody {
	b := &idleBody{
		rc:     rc,
		idle:   idle,
		cancel: cancel,
		last:   time.Now(),
	}
	b.timer = time.AfterFunc(idle, b.fire)
	return b
}

func (b *idleBody) fire() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.expired {
		return
	}
	if time.Since(b.last) < b.idle {
		return
	}
	b.expired = true
	b.cancel()
}

func (b *idleBody) idleErr() error {
	return fmt.Errorf("%w for %s without data", ErrStreamIdle, b.idle)
}

func (b *idleBody) Read(p []byte) (int, error) {
	b.mu.Lock()
	expired := b.expired
	b.mu.Unlock()
	if expired {
		return 0, b.idleErr()
	}
	n, err := b.rc.Read(p)
	b.mu.Lock()
	defer b.mu.Unlock()
	if err != nil {
		if b.expired {
			return n, b.idleErr()
		}
		return n, err
	}
	b.last = time.Now()
	if !b.closed && !b.expired {
		b.timer.Reset(b.idle)
	}
	return n, nil
}

func (b *idleBody) Close() error {
	b.mu.Lock()
	b.closed = true
	b.timer.Stop()
	b.mu.Unlock()
	b.cancel()
	return b.rc.Close()
}

func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("GET /v1/models: %s: %s", resp.Status, string(b))
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	names := make([]string, len(out.Data))
	for i, m := range out.Data {
		names[i] = m.ID
	}
	return names, nil
}

type statusError struct {
	code       int
	status     string
	body       string
	retryAfter time.Duration
}

func newStatusError(resp *http.Response) *statusError {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var retryAfter time.Duration
	if secs, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); err == nil && secs > 0 {
		retryAfter = time.Duration(secs) * time.Second
	}
	return &statusError{
		code:       resp.StatusCode,
		status:     resp.Status,
		body:       string(b),
		retryAfter: retryAfter,
	}
}

func (e *statusError) Error() string {
	return fmt.Sprintf("%s: %s", e.status, e.body)
}

func isTransient(err error) bool {
	if err == nil {
		return false
	}
	var se *statusError
	if errors.As(err, &se) {
		switch se.code {
		case http.StatusTooManyRequests,
			http.StatusInternalServerError,
			http.StatusBadGateway,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout:
			return true
		}
		return false
	}
	if errors.Is(err, ErrStreamIdle) || errors.Is(err, ErrFirstByte) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return true
	}
	return false
}

const backoffCap = 30 * time.Second

func jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	return time.Duration(float64(d) * (0.8 + rand.Float64()*0.4))
}

func (c *Client) backoffDuration(attempt int, err error) time.Duration {
	raw := c.retryBase
	for i := 1; i < attempt && raw < backoffCap; i++ {
		raw *= 2
	}
	if raw > backoffCap {
		raw = backoffCap
	}
	wait := jitter(raw)
	var se *statusError
	if errors.As(err, &se) && se.retryAfter > wait {
		wait = se.retryAfter
	}
	if wait > backoffCap {
		wait = backoffCap
	}
	return wait
}

func finalStreamError(err error, attempts int, model string) error {
	noun := "attempts"
	if attempts == 1 {
		noun = "attempt"
	}
	return fmt.Errorf("chat stream failed after %d %s (model %s): %w", attempts, noun, model, err)
}

func (c *Client) ChatStream(ctx context.Context, model string, messages []Message, tools []Tool, onDelta func(string)) (StreamResult, error) {
	req := chatRequest{
		Model:    model,
		Messages: messages,
		Tools:    tools,
		Stream:   true,
		StreamOptions: &streamOptions{
			IncludeUsage: true,
		},
	}
	emitted := 0
	countDelta := func(d string) {
		emitted++
		if onDelta != nil {
			onDelta(d)
		}
	}
	var result StreamResult
	for attempt := 1; ; attempt++ {
		var err error
		result, err = c.streamAttempt(ctx, req, countDelta)
		if err == nil {
			return result, nil
		}
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if errors.Is(err, context.Canceled) {
			return result, err
		}
		if emitted > 0 || !isTransient(err) || attempt > c.limits.MaxRetries {
			return result, finalStreamError(err, attempt, model)
		}
		select {
		case <-time.After(c.backoffDuration(attempt, err)):
		case <-ctx.Done():
			return result, ctx.Err()
		}
	}
}

func (c *Client) streamAttempt(ctx context.Context, req chatRequest, onDelta func(string)) (StreamResult, error) {
	resp, cancel, err := c.do(ctx, "/v1/chat/completions", req)
	if err != nil {
		return StreamResult{}, err
	}
	defer cancel()
	if resp.StatusCode != http.StatusOK {
		se := newStatusError(resp)
		resp.Body.Close()
		return StreamResult{}, se
	}
	var body io.ReadCloser = resp.Body
	if c.limits.IdleTimeout > 0 {
		body = newIdleBody(resp.Body, c.limits.IdleTimeout, cancel)
	}
	defer body.Close()

	start := time.Now()
	var result StreamResult
	type acc struct {
		id   string
		name string
		args strings.Builder
	}
	calls := map[int]*acc{}
	var order []int

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			if payload == "[DONE]" {
				break
			}
			continue
		}
		var chunk streamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return result, fmt.Errorf("decode stream chunk: %w", err)
		}
		if chunk.Usage != nil {
			result.Usage = *chunk.Usage
		}
		for _, choice := range chunk.Choices {
			d := choice.Delta
			if d.Content != "" {
				result.Content += d.Content
				onDelta(d.Content)
			}
			for _, tc := range d.ToolCalls {
				a, ok := calls[tc.Index]
				if !ok {
					a = &acc{}
					calls[tc.Index] = a
					order = append(order, tc.Index)
				}
				if tc.ID != "" {
					a.id = tc.ID
				}
				if tc.Function.Name != "" {
					a.name = tc.Function.Name
				}
				a.args.WriteString(tc.Function.Arguments)
			}
			if d.FinishReason != "" {
				result.FinishReason = d.FinishReason
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return result, err
	}
	for _, idx := range order {
		a := calls[idx]
		result.ToolCalls = append(result.ToolCalls, ToolCall{
			ID:       a.id,
			Type:     "function",
			Function: FuncCall{Name: a.name, Arguments: a.args.String()},
		})
	}
	result.StreamSeconds = time.Since(start).Seconds()
	return result, nil
}
