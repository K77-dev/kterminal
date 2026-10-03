package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
