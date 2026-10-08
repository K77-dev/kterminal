package llm

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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

type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
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
			Timeout:   10 * time.Minute,
			Transport: transport,
		},
	}
}

func (c *Client) do(ctx context.Context, path string, body any) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	return c.HTTPClient.Do(req)
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
	resp, err := c.do(ctx, "/v1/chat/completions", req)
	if err != nil {
		return StreamResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return StreamResult{}, fmt.Errorf("chat completions (%s): %s: %s", model, resp.Status, string(b))
	}

	start := time.Now()
	var result StreamResult
	type acc struct {
		id   string
		name string
		args strings.Builder
	}
	calls := map[int]*acc{}
	var order []int

	scanner := bufio.NewScanner(resp.Body)
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
				if onDelta != nil {
					onDelta(d.Content)
				}
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
