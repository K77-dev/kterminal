package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNormalizeBaseURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://host", "https://host"},
		{"https://host/", "https://host"},
		{"https://host/v1", "https://host"},
		{"https://host/v1/", "https://host"},
		{"https://host/v1//", "https://host"},
		{"https://host/prefix/v1", "https://host/prefix"},
		{"https://host/v11", "https://host/v11"},
	}
	for _, c := range cases {
		if got := normalizeBaseURL(c.in); got != c.want {
			t.Errorf("normalizeBaseURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestListModelsWithV1SuffixBaseURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"id": "glm-5.2"}},
		})
	}))
	defer srv.Close()

	client := New(srv.URL+"/v1", "key", false)
	names, err := client.ListModels(t.Context())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if gotPath != "/v1/models" {
		t.Fatalf("request path = %q, want /v1/models", gotPath)
	}
	if len(names) != 1 || names[0] != "glm-5.2" {
		t.Fatalf("names = %v", names)
	}
}

func TestMessageMarshalStringContentGolden(t *testing.T) {
	cases := []struct {
		name string
		msg  Message
		want string
	}{
		{
			name: "user message",
			msg:  Message{Role: "user", Content: "list the files"},
			want: `{"role":"user","content":"list the files"}`,
		},
		{
			name: "empty content",
			msg:  Message{Role: "user", Content: ""},
			want: `{"role":"user","content":""}`,
		},
		{
			name: "content with escapes",
			msg:  Message{Role: "user", Content: "line1\nline2 \"quoted\""},
			want: `{"role":"user","content":"line1\nline2 \"quoted\""}`,
		},
		{
			name: "assistant with tool calls",
			msg: Message{
				Role:    "assistant",
				Content: "running",
				ToolCalls: []ToolCall{{
					ID:       "call_1",
					Type:     "function",
					Function: FuncCall{Name: "bash", Arguments: `{"command":"ls"}`},
				}},
			},
			want: `{"role":"assistant","content":"running","tool_calls":[{"id":"call_1","type":"function","function":{"name":"bash","arguments":"{\"command\":\"ls\"}"}}]}`,
		},
		{
			name: "tool result",
			msg:  Message{Role: "tool", Content: "file1\nfile2", ToolCallID: "call_1"},
			want: `{"role":"tool","content":"file1\nfile2","tool_call_id":"call_1"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.msg)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if string(got) != tc.want {
				t.Fatalf("Marshal() = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestMessageMarshalContentParts(t *testing.T) {
	t.Run("text and image in order", func(t *testing.T) {
		msg := Message{
			Role: "user",
			ContentParts: []ContentPart{
				{Type: "text", Text: "what is in this image?"},
				{Type: "image_url", ImageURL: &ImageURL{URL: "data:image/png;base64,aGVsbG8="}},
			},
		}
		want := `{"role":"user","content":[{"type":"text","text":"what is in this image?"},{"type":"image_url","image_url":{"url":"data:image/png;base64,aGVsbG8="}}]}`
		got, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if string(got) != want {
			t.Fatalf("Marshal() = %s, want %s", got, want)
		}
	})
	t.Run("multiple images keep order", func(t *testing.T) {
		msg := Message{
			Role: "user",
			ContentParts: []ContentPart{
				{Type: "text", Text: "compare these"},
				{Type: "image_url", ImageURL: &ImageURL{URL: "data:image/png;base64,AAA="}},
				{Type: "image_url", ImageURL: &ImageURL{URL: "data:image/jpeg;base64,BBB="}},
			},
		}
		want := `{"role":"user","content":[{"type":"text","text":"compare these"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AAA="}},{"type":"image_url","image_url":{"url":"data:image/jpeg;base64,BBB="}}]}`
		got, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if string(got) != want {
			t.Fatalf("Marshal() = %s, want %s", got, want)
		}
	})
	t.Run("parts override string content", func(t *testing.T) {
		msg := Message{
			Role:         "user",
			Content:      "ignored",
			ContentParts: []ContentPart{{Type: "text", Text: "visible"}},
		}
		want := `{"role":"user","content":[{"type":"text","text":"visible"}]}`
		got, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if string(got) != want {
			t.Fatalf("Marshal() = %s, want %s", got, want)
		}
	})
	t.Run("omitempty tags", func(t *testing.T) {
		msg := Message{
			Role: "user",
			ContentParts: []ContentPart{
				{Type: "text"},
				{Type: "image_url", ImageURL: &ImageURL{}},
			},
		}
		want := `{"role":"user","content":[{"type":"text"},{"type":"image_url","image_url":{}}]}`
		got, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if string(got) != want {
			t.Fatalf("Marshal() = %s, want %s", got, want)
		}
	})
}

func TestMessageUnmarshalRoundtrip(t *testing.T) {
	t.Run("fields preserved", func(t *testing.T) {
		originals := []Message{
			{Role: "user", Content: "list the files"},
			{
				Role: "user",
				ContentParts: []ContentPart{
					{Type: "text", Text: "what is in this image?"},
					{Type: "image_url", ImageURL: &ImageURL{URL: "data:image/png;base64,aGVsbG8="}},
				},
			},
			{
				Role:    "assistant",
				Content: "running",
				ToolCalls: []ToolCall{{
					ID:       "call_1",
					Type:     "function",
					Function: FuncCall{Name: "bash", Arguments: `{"command":"ls"}`},
				}},
			},
			{Role: "tool", Content: "file1\nfile2", ToolCallID: "call_1"},
		}
		for _, orig := range originals {
			data, err := json.Marshal(orig)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var back Message
			if err := json.Unmarshal(data, &back); err != nil {
				t.Fatalf("Unmarshal(%s): %v", data, err)
			}
			if !reflect.DeepEqual(back, orig) {
				t.Errorf("roundtrip = %+v, want %+v (json %s)", back, orig, data)
			}
		}
	})
	t.Run("missing content", func(t *testing.T) {
		var m Message
		if err := json.Unmarshal([]byte(`{"role":"user"}`), &m); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if m.Role != "user" || m.Content != "" || m.ContentParts != nil {
			t.Fatalf("got %+v", m)
		}
	})
	t.Run("null content", func(t *testing.T) {
		var m Message
		if err := json.Unmarshal([]byte(`{"role":"user","content":null}`), &m); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if m.Role != "user" || m.Content != "" || m.ContentParts != nil {
			t.Fatalf("got %+v", m)
		}
	})
	t.Run("null message is no-op", func(t *testing.T) {
		m := Message{Role: "user", Content: "kept"}
		if err := json.Unmarshal([]byte(`null`), &m); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if m.Role != "user" || m.Content != "kept" {
			t.Fatalf("got %+v", m)
		}
	})
	t.Run("invalid content type", func(t *testing.T) {
		var m Message
		if err := json.Unmarshal([]byte(`{"role":"user","content":42}`), &m); err == nil {
			t.Fatal("want error for numeric content")
		}
	})
	t.Run("invalid part", func(t *testing.T) {
		var m Message
		if err := json.Unmarshal([]byte(`{"role":"user","content":[{"type":42}]}`), &m); err == nil {
			t.Fatal("want error for invalid part")
		}
	})
}

func writeSSE(w http.ResponseWriter, payload string) {
	fmt.Fprintf(w, "data: %s\n\n", payload)
	w.(http.Flusher).Flush()
}

func hold(r *http.Request) {
	select {
	case <-r.Context().Done():
	case <-time.After(5 * time.Second):
	}
}

func TestLimits(t *testing.T) {
	t.Run("new applies defaults", func(t *testing.T) {
		client := New("https://host/v1", "key", false)
		if client.limits != defaultLimits {
			t.Fatalf("limits = %+v, want %+v", client.limits, defaultLimits)
		}
		if client.HTTPClient.Timeout != defaultLimits.RequestTimeout {
			t.Fatalf("HTTPClient.Timeout = %s, want %s", client.HTTPClient.Timeout, defaultLimits.RequestTimeout)
		}
	})
	t.Run("set limits replaces all fields", func(t *testing.T) {
		client := New("https://host", "key", false)
		custom := Limits{RequestTimeout: 0, IdleTimeout: 30 * time.Second, FirstByteTimeout: 5 * time.Second, MaxRetries: 7}
		client.SetLimits(custom)
		if client.limits != custom {
			t.Fatalf("limits = %+v, want %+v", client.limits, custom)
		}
		if client.HTTPClient.Timeout != 0 {
			t.Fatalf("HTTPClient.Timeout = %s, want 0", client.HTTPClient.Timeout)
		}
	})
}

func TestChatStreamSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSE(w, `{"choices":[{"delta":{"role":"assistant"}}]}`)
		writeSSE(w, `{"choices":[{"delta":{"content":"hel"}}]}`)
		writeSSE(w, `{"choices":[{"delta":{"content":"lo"}}]}`)
		writeSSE(w, `{"choices":[{"delta":{"finish_reason":"stop"}}]}`)
		writeSSE(w, `{"usage":{"prompt_tokens":3,"completion_tokens":2}}`)
		writeSSE(w, `[DONE]`)
	}))
	defer srv.Close()

	client := New(srv.URL, "key", false)
	client.SetLimits(Limits{RequestTimeout: 5 * time.Second, IdleTimeout: 500 * time.Millisecond, FirstByteTimeout: 500 * time.Millisecond, MaxRetries: 0})
	var deltas []string
	res, err := client.ChatStream(t.Context(), "glm-5.2", []Message{{Role: "user", Content: "hi"}}, nil, func(d string) { deltas = append(deltas, d) })
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if res.Content != "hello" {
		t.Fatalf("content = %q, want %q", res.Content, "hello")
	}
	if res.FinishReason != "stop" {
		t.Fatalf("finish reason = %q, want %q", res.FinishReason, "stop")
	}
	if res.Usage.PromptTokens != 3 || res.Usage.CompletionTokens != 2 {
		t.Fatalf("usage = %+v, want {3 2}", res.Usage)
	}
	if !reflect.DeepEqual(deltas, []string{"hel", "lo"}) {
		t.Fatalf("deltas = %v, want [hel lo]", deltas)
	}
	if res.StreamSeconds <= 0 {
		t.Fatalf("stream seconds = %f, want > 0", res.StreamSeconds)
	}
}

func TestChatStreamFirstByteTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		hold(r)
	}))
	defer srv.Close()

	client := New(srv.URL, "key", false)
	client.SetLimits(Limits{RequestTimeout: 10 * time.Second, IdleTimeout: time.Second, FirstByteTimeout: 100 * time.Millisecond, MaxRetries: 0})
	start := time.Now()
	_, err := client.ChatStream(t.Context(), "glm-5.2", []Message{{Role: "user", Content: "hi"}}, nil, nil)
	if err == nil {
		t.Fatal("want first byte timeout error")
	}
	if !errors.Is(err, ErrFirstByte) {
		t.Fatalf("err = %v, want ErrFirstByte", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("ChatStream took %s, want under 1s", elapsed)
	}
}

func TestChatStreamIdleTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSE(w, `{"choices":[{"delta":{"content":"hel"}}]}`)
		writeSSE(w, `{"choices":[{"delta":{"content":"lo"}}]}`)
		hold(r)
	}))
	defer srv.Close()

	client := New(srv.URL, "key", false)
	client.SetLimits(Limits{RequestTimeout: 10 * time.Second, IdleTimeout: 150 * time.Millisecond, FirstByteTimeout: time.Second, MaxRetries: 0})
	start := time.Now()
	res, err := client.ChatStream(t.Context(), "glm-5.2", []Message{{Role: "user", Content: "hi"}}, nil, nil)
	if err == nil {
		t.Fatal("want stream idle error")
	}
	if !errors.Is(err, ErrStreamIdle) {
		t.Fatalf("err = %v, want ErrStreamIdle", err)
	}
	if res.Content != "hello" {
		t.Fatalf("content = %q, want %q", res.Content, "hello")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("ChatStream took %s, want under 2s", elapsed)
	}
}

func TestChatStreamHealthySpacedChunks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, piece := range []string{"a", "b", "c", "d"} {
			writeSSE(w, fmt.Sprintf(`{"choices":[{"delta":{"content":%q}}]}`, piece))
			time.Sleep(100 * time.Millisecond)
		}
		writeSSE(w, `[DONE]`)
	}))
	defer srv.Close()

	client := New(srv.URL, "key", false)
	client.SetLimits(Limits{RequestTimeout: 10 * time.Second, IdleTimeout: 400 * time.Millisecond, FirstByteTimeout: 150 * time.Millisecond, MaxRetries: 0})
	res, err := client.ChatStream(t.Context(), "glm-5.2", []Message{{Role: "user", Content: "hi"}}, nil, nil)
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if res.Content != "abcd" {
		t.Fatalf("content = %q, want %q", res.Content, "abcd")
	}
}

func TestChatStreamRequestTimeoutCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSE(w, `{"choices":[{"delta":{"content":"x"}}]}`)
		hold(r)
	}))
	defer srv.Close()

	client := New(srv.URL, "key", false)
	client.SetLimits(Limits{RequestTimeout: 150 * time.Millisecond, IdleTimeout: 5 * time.Second, FirstByteTimeout: 5 * time.Second, MaxRetries: 0})
	start := time.Now()
	_, err := client.ChatStream(t.Context(), "glm-5.2", []Message{{Role: "user", Content: "hi"}}, nil, nil)
	if err == nil {
		t.Fatal("want request timeout error")
	}
	if errors.Is(err, ErrStreamIdle) || errors.Is(err, ErrFirstByte) {
		t.Fatalf("err = %v, want total cap error", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("ChatStream took %s, want under 2s", elapsed)
	}
}

func TestChatStreamZeroTimeoutsDisableLayers(t *testing.T) {
	t.Run("idle zero keeps stream under total cap only", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.Copy(io.Discard, r.Body)
			w.Header().Set("Content-Type", "text/event-stream")
			writeSSE(w, `{"choices":[{"delta":{"content":"x"}}]}`)
			hold(r)
		}))
		defer srv.Close()

		client := New(srv.URL, "key", false)
		client.SetLimits(Limits{RequestTimeout: 250 * time.Millisecond, IdleTimeout: 0, FirstByteTimeout: 5 * time.Second, MaxRetries: 0})
		start := time.Now()
		_, err := client.ChatStream(t.Context(), "glm-5.2", []Message{{Role: "user", Content: "hi"}}, nil, nil)
		if err == nil {
			t.Fatal("want error")
		}
		if errors.Is(err, ErrStreamIdle) {
			t.Fatalf("err = %v, want no idle watchdog", err)
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("ChatStream took %s, want under 2s", elapsed)
		}
	})
	t.Run("first byte zero keeps request under total cap only", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.Copy(io.Discard, r.Body)
			hold(r)
		}))
		defer srv.Close()

		client := New(srv.URL, "key", false)
		client.SetLimits(Limits{RequestTimeout: 250 * time.Millisecond, IdleTimeout: 5 * time.Second, FirstByteTimeout: 0, MaxRetries: 0})
		start := time.Now()
		_, err := client.ChatStream(t.Context(), "glm-5.2", []Message{{Role: "user", Content: "hi"}}, nil, nil)
		if err == nil {
			t.Fatal("want error")
		}
		if errors.Is(err, ErrFirstByte) {
			t.Fatalf("err = %v, want no first byte timer", err)
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("ChatStream took %s, want under 2s", elapsed)
		}
	})
}

type stubNetError struct{ timeout bool }

func (stubNetError) Error() string   { return "stub net error" }
func (n stubNetError) Timeout() bool { return n.timeout }
func (stubNetError) Temporary() bool { return true }

func TestIsTransient(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"status 429", &statusError{code: http.StatusTooManyRequests}, true},
		{"status 500", &statusError{code: http.StatusInternalServerError}, true},
		{"status 502", &statusError{code: http.StatusBadGateway}, true},
		{"status 503", &statusError{code: http.StatusServiceUnavailable}, true},
		{"status 504", &statusError{code: http.StatusGatewayTimeout}, true},
		{"status 401", &statusError{code: http.StatusUnauthorized}, false},
		{"status 400", &statusError{code: http.StatusBadRequest}, false},
		{"status 404", &statusError{code: http.StatusNotFound}, false},
		{"stream idle", fmt.Errorf("wrapped: %w for %s without data", ErrStreamIdle, 5*time.Minute), true},
		{"first byte", fmt.Errorf("wrapped: %w within %s — check the gateway", ErrFirstByte, time.Minute), true},
		{"eof", io.EOF, true},
		{"unexpected eof", io.ErrUnexpectedEOF, true},
		{"deadline exceeded", context.DeadlineExceeded, true},
		{"net error timeout", stubNetError{timeout: true}, true},
		{"net error plain", stubNetError{}, true},
		{"canceled", context.Canceled, false},
		{"decode chunk", fmt.Errorf("decode stream chunk: %w", errors.New("invalid character")), false},
		{"generic", errors.New("boom"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTransient(tc.err); got != tc.want {
				t.Fatalf("isTransient(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestNewStatusError(t *testing.T) {
	cases := []struct {
		name   string
		header string
		want   time.Duration
	}{
		{"delta seconds", "1", time.Second},
		{"large delta seconds", "120", 120 * time.Second},
		{"missing header", "", 0},
		{"unparseable", "soon", 0},
		{"negative", "-5", 0},
		{"http date not supported", "Wed, 21 Oct 2026 07:28:00 GMT", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Status:     "429 Too Many Requests",
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader("rate limited")),
			}
			if tc.header != "" {
				resp.Header.Set("Retry-After", tc.header)
			}
			se := newStatusError(resp)
			if se.code != http.StatusTooManyRequests {
				t.Fatalf("code = %d, want 429", se.code)
			}
			if se.retryAfter != tc.want {
				t.Fatalf("retryAfter = %s, want %s", se.retryAfter, tc.want)
			}
			msg := se.Error()
			if !strings.Contains(msg, "429 Too Many Requests") || !strings.Contains(msg, "rate limited") {
				t.Fatalf("Error() = %q, want status and body", msg)
			}
		})
	}
}

func TestBackoffDuration(t *testing.T) {
	client := New("https://host", "key", false)
	client.retryBase = time.Second

	t.Run("exponential with jitter", func(t *testing.T) {
		cases := []struct {
			attempt  int
			min, max time.Duration
		}{
			{1, 800 * time.Millisecond, 1200 * time.Millisecond},
			{2, 1600 * time.Millisecond, 2400 * time.Millisecond},
			{3, 3200 * time.Millisecond, 4800 * time.Millisecond},
			{4, 6400 * time.Millisecond, 9600 * time.Millisecond},
		}
		for _, tc := range cases {
			for i := 0; i < 100; i++ {
				got := client.backoffDuration(tc.attempt, nil)
				if got < tc.min || got > tc.max {
					t.Fatalf("backoffDuration(%d) = %s, want within [%s, %s]", tc.attempt, got, tc.min, tc.max)
				}
			}
		}
	})
	t.Run("capped at 30s", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			got := client.backoffDuration(10, nil)
			if got < 24*time.Second || got > backoffCap {
				t.Fatalf("backoffDuration(10) = %s, want within [24s, %s]", got, backoffCap)
			}
		}
	})
	t.Run("retry-after prevails when greater", func(t *testing.T) {
		err := &statusError{code: http.StatusTooManyRequests, retryAfter: 5 * time.Second}
		for i := 0; i < 100; i++ {
			if got := client.backoffDuration(1, err); got != 5*time.Second {
				t.Fatalf("backoffDuration(1, retryAfter=5s) = %s, want 5s", got)
			}
		}
	})
	t.Run("retry-after capped at 30s", func(t *testing.T) {
		err := &statusError{code: http.StatusTooManyRequests, retryAfter: 10 * time.Minute}
		if got := client.backoffDuration(1, err); got != backoffCap {
			t.Fatalf("backoffDuration(1, retryAfter=10m) = %s, want %s", got, backoffCap)
		}
	})
	t.Run("retry-after smaller keeps backoff", func(t *testing.T) {
		err := &statusError{code: http.StatusTooManyRequests, retryAfter: 100 * time.Millisecond}
		for i := 0; i < 100; i++ {
			got := client.backoffDuration(2, err)
			if got < 1600*time.Millisecond || got > 2400*time.Millisecond {
				t.Fatalf("backoffDuration(2, retryAfter=100ms) = %s, want within [1.6s, 2.4s]", got)
			}
		}
	})
}

func TestChatStreamRetryTransientThenSuccess(t *testing.T) {
	var mu sync.Mutex
	var calls []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, time.Now())
		first := len(calls) == 1
		mu.Unlock()
		io.Copy(io.Discard, r.Body)
		if first {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSE(w, `{"choices":[{"delta":{"content":"hel"}}]}`)
		writeSSE(w, `{"choices":[{"delta":{"content":"lo"}}]}`)
		writeSSE(w, `{"choices":[{"delta":{"finish_reason":"stop"}}]}`)
		writeSSE(w, `[DONE]`)
	}))
	defer srv.Close()

	client := New(srv.URL, "key", false)
	client.SetLimits(Limits{RequestTimeout: 5 * time.Second, IdleTimeout: time.Second, FirstByteTimeout: time.Second, MaxRetries: 1})
	client.retryBase = 100 * time.Millisecond
	var deltas []string
	res, err := client.ChatStream(t.Context(), "glm-5.2", []Message{{Role: "user", Content: "hi"}}, nil, func(d string) { deltas = append(deltas, d) })
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 2 {
		t.Fatalf("handler calls = %d, want 2", len(calls))
	}
	if interval := calls[1].Sub(calls[0]); interval < 50*time.Millisecond {
		t.Fatalf("interval between attempts = %s, want >= 50ms of backoff", interval)
	}
	if res.Content != "hello" {
		t.Fatalf("content = %q, want %q", res.Content, "hello")
	}
	if !reflect.DeepEqual(deltas, []string{"hel", "lo"}) {
		t.Fatalf("deltas = %v, want [hel lo] with no duplication", deltas)
	}
}

func TestChatStreamRetryHonorsRetryAfter(t *testing.T) {
	var mu sync.Mutex
	var calls []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, time.Now())
		first := len(calls) == 1
		mu.Unlock()
		io.Copy(io.Discard, r.Body)
		if first {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSE(w, `{"choices":[{"delta":{"content":"ok"}}]}`)
		writeSSE(w, `[DONE]`)
	}))
	defer srv.Close()

	client := New(srv.URL, "key", false)
	client.SetLimits(Limits{RequestTimeout: 5 * time.Second, IdleTimeout: time.Second, FirstByteTimeout: time.Second, MaxRetries: 1})
	client.retryBase = 10 * time.Millisecond
	res, err := client.ChatStream(t.Context(), "glm-5.2", []Message{{Role: "user", Content: "hi"}}, nil, nil)
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 2 {
		t.Fatalf("handler calls = %d, want 2", len(calls))
	}
	if interval := calls[1].Sub(calls[0]); interval < 900*time.Millisecond {
		t.Fatalf("interval between attempts = %s, want >= 900ms honoring Retry-After: 1", interval)
	}
	if res.Content != "ok" {
		t.Fatalf("content = %q, want %q", res.Content, "ok")
	}
}

func TestChatStreamRetryFirstByteTimeout(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		first := calls == 1
		mu.Unlock()
		io.Copy(io.Discard, r.Body)
		if first {
			hold(r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSE(w, `{"choices":[{"delta":{"content":"ok"}}]}`)
		writeSSE(w, `[DONE]`)
	}))
	defer srv.Close()

	client := New(srv.URL, "key", false)
	client.SetLimits(Limits{RequestTimeout: 5 * time.Second, IdleTimeout: time.Second, FirstByteTimeout: 100 * time.Millisecond, MaxRetries: 1})
	client.retryBase = 10 * time.Millisecond
	res, err := client.ChatStream(t.Context(), "glm-5.2", []Message{{Role: "user", Content: "hi"}}, nil, nil)
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 2 {
		t.Fatalf("handler calls = %d, want 2", calls)
	}
	if res.Content != "ok" {
		t.Fatalf("content = %q, want %q", res.Content, "ok")
	}
}

func TestChatStreamNoRetryAfterDelta(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSE(w, `{"choices":[{"delta":{"content":"hel"}}]}`)
		hold(r)
	}))
	defer srv.Close()

	client := New(srv.URL, "key", false)
	client.SetLimits(Limits{RequestTimeout: 10 * time.Second, IdleTimeout: 150 * time.Millisecond, FirstByteTimeout: time.Second, MaxRetries: 2})
	client.retryBase = 10 * time.Millisecond
	var deltas []string
	res, err := client.ChatStream(t.Context(), "glm-5.2", []Message{{Role: "user", Content: "hi"}}, nil, func(d string) { deltas = append(deltas, d) })
	if err == nil {
		t.Fatal("want stream idle error")
	}
	if !errors.Is(err, ErrStreamIdle) {
		t.Fatalf("err = %v, want ErrStreamIdle", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "chat stream failed after 1 attempt") {
		t.Fatalf("err = %q, want attempt count in final message", msg)
	}
	if !strings.Contains(msg, "stream stalled for 150ms without data") {
		t.Fatalf("err = %q, want friendly idle cause", msg)
	}
	if !strings.Contains(msg, "glm-5.2") {
		t.Fatalf("err = %q, want model name", msg)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1 (no retry after emitted delta)", calls)
	}
	if !reflect.DeepEqual(deltas, []string{"hel"}) {
		t.Fatalf("deltas = %v, want [hel] exactly once", deltas)
	}
	if res.Content != "hel" {
		t.Fatalf("content = %q, want %q", res.Content, "hel")
	}
}

func TestChatStreamNoRetryOnClientError(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"bad key"}}`)
	}))
	defer srv.Close()

	client := New(srv.URL, "key", false)
	client.SetLimits(Limits{RequestTimeout: 5 * time.Second, IdleTimeout: time.Second, FirstByteTimeout: time.Second, MaxRetries: 2})
	client.retryBase = 10 * time.Millisecond
	_, err := client.ChatStream(t.Context(), "glm-5.2", []Message{{Role: "user", Content: "hi"}}, nil, nil)
	if err == nil {
		t.Fatal("want unauthorized error")
	}
	var se *statusError
	if !errors.As(err, &se) || se.code != http.StatusUnauthorized {
		t.Fatalf("err = %v, want statusError with code 401", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "401 Unauthorized") {
		t.Fatalf("err = %q, want status in message", msg)
	}
	if !strings.Contains(msg, "bad key") {
		t.Fatalf("err = %q, want body in message", msg)
	}
	if !strings.Contains(msg, "chat stream failed after 1 attempt") {
		t.Fatalf("err = %q, want single attempt in message", msg)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1 (no retry on 4xx)", calls)
	}
}

func TestChatStreamRetryExhaustion(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := New(srv.URL, "key", false)
	client.SetLimits(Limits{RequestTimeout: 5 * time.Second, IdleTimeout: time.Second, FirstByteTimeout: time.Second, MaxRetries: 2})
	client.retryBase = 10 * time.Millisecond
	start := time.Now()
	_, err := client.ChatStream(t.Context(), "glm-5.2", []Message{{Role: "user", Content: "hi"}}, nil, nil)
	if err == nil {
		t.Fatal("want exhausted retry error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "chat stream failed after 3 attempts") {
		t.Fatalf("err = %q, want 3 attempts in message", msg)
	}
	if !strings.Contains(msg, "glm-5.2") {
		t.Fatalf("err = %q, want model name in message", msg)
	}
	if !strings.Contains(msg, "500 Internal Server Error") {
		t.Fatalf("err = %q, want status in message", msg)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 3 {
		t.Fatalf("handler calls = %d, want 3", calls)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("ChatStream took %s, want under 2s with short retry base", elapsed)
	}
}

func TestChatStreamCancelNotRetried(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSE(w, `{"choices":[{"delta":{"content":"hel"}}]}`)
		hold(r)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	gotDelta := make(chan struct{}, 1)
	go func() {
		<-gotDelta
		cancel()
	}()

	client := New(srv.URL, "key", false)
	client.SetLimits(Limits{RequestTimeout: 10 * time.Second, IdleTimeout: 5 * time.Second, FirstByteTimeout: 5 * time.Second, MaxRetries: 2})
	client.retryBase = 10 * time.Millisecond
	var deltas []string
	_, err := client.ChatStream(ctx, "glm-5.2", []Message{{Role: "user", Content: "hi"}}, nil, func(d string) {
		deltas = append(deltas, d)
		select {
		case gotDelta <- struct{}{}:
		default:
		}
	})
	if err == nil {
		t.Fatal("want cancel error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1 (caller cancel is never retried)", calls)
	}
	if !reflect.DeepEqual(deltas, []string{"hel"}) {
		t.Fatalf("deltas = %v, want [hel]", deltas)
	}
}
