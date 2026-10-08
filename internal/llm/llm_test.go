package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
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
