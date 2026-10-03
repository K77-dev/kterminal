package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kterminal/internal/catalog"
	"kterminal/internal/jev"
	"kterminal/internal/llm"
	"kterminal/internal/router"
	"kterminal/internal/tools"
)

func testCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	return cat
}

func sseHandler(t *testing.T, responses [][]string) http.HandlerFunc {
	t.Helper()
	call := 0
	return func(w http.ResponseWriter, r *http.Request) {
		if call >= len(responses) {
			t.Errorf("unexpected chat call %d", call)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		chunks := responses[call]
		call++
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for _, c := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", c)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}
}

func contentChunks(text string, promptTokens, completionTokens int64) []string {
	return []string{
		fmt.Sprintf(`{"choices":[{"delta":{"role":"assistant","content":%s}}]}`, mustJSON(text)),
		`{"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":` + fmt.Sprint(promptTokens) + `,"completion_tokens":` + fmt.Sprint(completionTokens) + `}}`,
	}
}

func toolCallChunks(id, name, args string) []string {
	return []string{
		fmt.Sprintf(`{"choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":%s,"function":{"name":%s,"arguments":%s}}]}}]}`, mustJSON(id), mustJSON(name), mustJSON(args)),
		`{"choices":[{"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`,
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func mockGateway(t *testing.T, responses [][]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		data := []map[string]string{}
		for _, m := range testCatalog(t).Models {
			data = append(data, map[string]string{"id": m.Name})
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": data,
		})
	})
	mux.HandleFunc("POST /v1/chat/completions", sseHandler(t, responses))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func mockJev(t *testing.T, choice string, confidence float64) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/systemone", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"model": "jev-test",
			"answers": map[string]any{
				"model_choice": map[string]any{
					"type":          "choice",
					"choice":        choice,
					"confidence":    confidence,
					"probabilities": map[string]float64{choice: confidence},
				},
			},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func collectEvents(t *testing.T, ag *Agent) []Event {
	t.Helper()
	var events []Event
	for {
		e, ok := <-ag.Events
		if !ok {
			break
		}
		events = append(events, e)
		if e.Kind == EventTurnDone || e.Kind == EventError {
			return events
		}
	}
	return events
}

func TestAgentRoutesViaJevAndStreams(t *testing.T) {
	cat := testCatalog(t)
	gw := mockGateway(t, [][]string{contentChunks("hello from glm", 10, 3)})
	jevSrv := mockJev(t, "glm-5.3", 0.91)

	jevClient := jev.New("test-key")
	jevClient.BaseURL = jevSrv.URL
	ag := New(
		llm.New(gw.URL, "gw-key", false),
		router.NewJev(jevClient),
		&router.HeuristicRouter{Default: cat.DefaultModel},
		cat,
		tools.NewRegistry(),
		nil,
		false,
	)

	ag.Run("write me a haiku")
	events := collectEvents(t, ag)

	var route, done *Event
	var text string
	for i, e := range events {
		switch e.Kind {
		case EventRoute:
			route = &events[i]
		case EventDelta:
			text += e.Text
		case EventTurnDone:
			done = &events[i]
		case EventError:
			t.Fatalf("unexpected error event: %s", e.Text)
		}
	}
	if route == nil {
		t.Fatal("no route event")
	}
	if route.Model != "glm-5.3" || route.Router != "jev" {
		t.Fatalf("route = %+v, want glm-5.3 via jev", route)
	}
	if route.Confidence != 0.91 {
		t.Fatalf("confidence = %v, want 0.91", route.Confidence)
	}
	if text != "hello from glm" {
		t.Fatalf("streamed text = %q", text)
	}
	if done == nil {
		t.Fatal("no turn_done event")
	}
	if done.SessionCost <= 0 {
		t.Fatalf("session cost = %v, want > 0", done.SessionCost)
	}
}

func TestAgentToolLoopReroutes(t *testing.T) {
	cat := testCatalog(t)
	gw := mockGateway(t, [][]string{
		toolCallChunks("call_1", "bash", `{"command":"echo hi"}`),
		contentChunks("done", 20, 1),
	})
	jevSrv := mockJev(t, "deepseek-v4-flash", 0.8)

	jevClient := jev.New("test-key")
	jevClient.BaseURL = jevSrv.URL
	ag := New(
		llm.New(gw.URL, "gw-key", false),
		router.NewJev(jevClient),
		&router.HeuristicRouter{Default: cat.DefaultModel},
		cat,
		tools.NewRegistry(),
		nil,
		false,
	)

	ag.Run("run echo hi")
	events := collectEvents(t, ag)

	var routes, toolStarts, toolResults int
	var result string
	sawDone := false
	for _, e := range events {
		switch e.Kind {
		case EventRoute:
			routes++
		case EventToolStart:
			toolStarts++
		case EventToolResult:
			toolResults++
			result = e.Result
		case EventTurnDone:
			sawDone = true
		case EventError:
			t.Fatalf("unexpected error event: %s", e.Text)
		}
	}
	if routes != 2 {
		t.Fatalf("routes = %d, want 2 (one per LLM call)", routes)
	}
	if toolStarts != 1 || toolResults != 1 {
		t.Fatalf("toolStarts=%d toolResults=%d, want 1/1", toolStarts, toolResults)
	}
	if !strings.Contains(result, "hi") {
		t.Fatalf("tool result = %q, want it to contain echo output", result)
	}
	if !sawDone {
		t.Fatal("no turn_done event")
	}
}

func TestAgentFallsBackToHeuristicWhenJevFails(t *testing.T) {
	cat := testCatalog(t)
	gw := mockGateway(t, [][]string{contentChunks("ok", 5, 1)})
	jevSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(jevSrv.Close)

	jevClient := jev.New("test-key")
	jevClient.BaseURL = jevSrv.URL
	ag := New(
		llm.New(gw.URL, "gw-key", false),
		router.NewJev(jevClient),
		&router.HeuristicRouter{Default: cat.DefaultModel},
		cat,
		tools.NewRegistry(),
		nil,
		false,
	)

	ag.Run("please refactor this complex architecture")
	events := collectEvents(t, ag)

	for _, e := range events {
		if e.Kind == EventError {
			t.Fatalf("unexpected error event: %s", e.Text)
		}
		if e.Kind == EventRoute {
			if e.Router != "heuristic" {
				t.Fatalf("router = %q, want heuristic fallback", e.Router)
			}
			if e.Model != "glm-5.3" {
				t.Fatalf("model = %q, want glm-5.3 for hard task", e.Model)
			}
		}
	}
}

func TestAgentLowConfidenceUsesDefault(t *testing.T) {
	cat := testCatalog(t)
	gw := mockGateway(t, [][]string{contentChunks("ok", 5, 1)})
	jevSrv := mockJev(t, "deepseek-v4-flash", 0.2)

	jevClient := jev.New("test-key")
	jevClient.BaseURL = jevSrv.URL
	ag := New(
		llm.New(gw.URL, "gw-key", false),
		router.NewJev(jevClient),
		nil,
		cat,
		tools.NewRegistry(),
		nil,
		false,
	)

	ag.Run("hello")
	events := collectEvents(t, ag)

	for _, e := range events {
		if e.Kind == EventRoute {
			if e.Model != cat.DefaultModel {
				t.Fatalf("model = %q, want default %q on low confidence", e.Model, cat.DefaultModel)
			}
		}
		if e.Kind == EventError {
			t.Fatalf("unexpected error: %s", e.Text)
		}
	}
}

func TestAgentPinnedSkipsRouter(t *testing.T) {
	cat := testCatalog(t)
	gw := mockGateway(t, [][]string{contentChunks("ok", 5, 1)})
	ag := New(
		llm.New(gw.URL, "gw-key", false),
		&router.HeuristicRouter{Default: cat.DefaultModel},
		nil,
		cat,
		tools.NewRegistry(),
		nil,
		false,
	)
	ag.SetPinned("glm-5.1")
	ag.Run("anything")
	events := collectEvents(t, ag)
	for _, e := range events {
		if e.Kind == EventRoute && (e.Model != "glm-5.1" || e.Router != "pin") {
			t.Fatalf("route = %+v, want pinned glm-5.1", e)
		}
	}
}

func TestHeuristicRouterRouting(t *testing.T) {
	cat := testCatalog(t)
	r := &router.HeuristicRouter{Default: cat.DefaultModel}
	ctx := context.Background()

	d, err := r.Route(ctx, "debug this tricky race condition", cat.Models)
	if err != nil || d.Model != "glm-5.3" {
		t.Fatalf("hard task -> %+v err=%v, want glm-5.3", d, err)
	}
	d, err = r.Route(ctx, "quick, just list the files", cat.Models)
	if err != nil || d.Model != "deepseek-v4.1-flash" {
		t.Fatalf("routine task -> %+v err=%v, want deepseek-v4.1-flash", d, err)
	}
	d, err = r.Route(ctx, "make me a sandwich", cat.Models)
	if err != nil || d.Model != cat.DefaultModel {
		t.Fatalf("default task -> %+v err=%v, want %s", d, err, cat.DefaultModel)
	}
}
