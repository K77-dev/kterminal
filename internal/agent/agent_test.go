package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"kterminal/internal/catalog"
	"kterminal/internal/jev"
	"kterminal/internal/kspec"
	"kterminal/internal/llm"
	"kterminal/internal/router"
	"kterminal/internal/session"
	"kterminal/internal/squad"
	"kterminal/internal/telemetry"
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

func slowGateway(t *testing.T, delay time.Duration, responses [][]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", modelsHandler(t))
	var call int
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
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
		time.Sleep(delay)
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	})
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

func capturingJev(t *testing.T, choice string, confidence float64, captured *map[string]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/systemone", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Questions map[string]struct {
				Criteria map[string]string `json:"criteria"`
			} `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode jev request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		*captured = body.Questions["model_choice"].Criteria
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

func selfCorrectGateway(t *testing.T, editArgs string, captured *[]llm.Message) *httptest.Server {
	t.Helper()
	responses := [][]string{
		toolCallChunks("call_1", "edit", editArgs),
		contentChunks("fixed the undefined reference", 20, 1),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", modelsHandler(t))
	var call int
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if call >= len(responses) {
			t.Errorf("unexpected chat call %d", call)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if call == 1 {
			*captured = decodeMessages(r)
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
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestAgentSelfCorrectsWithinTurn(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "broken.go")
	if err := os.WriteFile(target, []byte("package main\n\nfunc main() {\n\tprintln(\"ok\")\n}\n"), 0o644); err != nil {
		t.Fatalf("setup target: %v", err)
	}
	editArgs := fmt.Sprintf(`{"path":%s,"old_string":%s,"new_string":%s}`,
		mustJSON(target), mustJSON(`println("ok")`), mustJSON("println(Foo)"))
	var captured []llm.Message
	gw := selfCorrectGateway(t, editArgs, &captured)
	jevSrv := mockJev(t, "glm-5.3", 0.9)

	diagnostic := "\n\nDiagnostics:\n" + target + ":5:10: undefined: Foo"
	var hookPaths []string
	reg := tools.NewRegistry()
	reg.SetOnGoEdit(func(path string) string {
		hookPaths = append(hookPaths, path)
		return diagnostic
	})

	cat := testCatalog(t)
	jevClient := jev.New("test-key")
	jevClient.BaseURL = jevSrv.URL
	ag := New(
		llm.New(gw.URL, "gw-key", false),
		router.NewJev(jevClient),
		&router.HeuristicRouter{Default: cat.DefaultModel},
		cat,
		reg,
		nil,
		false,
	)

	ag.Run("use Foo in main")
	events := collectEvents(t, ag)

	sawDone := false
	var toolResult string
	for _, e := range events {
		switch e.Kind {
		case EventToolResult:
			toolResult = e.Result
		case EventTurnDone:
			sawDone = true
		case EventError:
			t.Fatalf("unexpected error event: %s", e.Text)
		}
	}
	if !sawDone {
		t.Fatal("no turn_done — the turn must close without user input")
	}
	if len(hookPaths) != 1 || hookPaths[0] != target {
		t.Fatalf("hook paths = %v, want exactly [%s]", hookPaths, target)
	}
	if !strings.Contains(toolResult, "Diagnostics:") || !strings.Contains(toolResult, "undefined: Foo") {
		t.Fatalf("tool result = %q, want the diagnostic appended to the edit output", toolResult)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if !strings.Contains(string(data), "println(Foo)") {
		t.Fatalf("edited file = %q, want the breaking edit applied", string(data))
	}

	if len(captured) != 3 {
		t.Fatalf("second gateway call sent %d messages, want 3 (user, assistant tool call, tool result): %+v", len(captured), captured)
	}
	if captured[0].Role != "user" || captured[0].Content != "use Foo in main" {
		t.Fatalf("first message = %+v, want the original user request", captured[0])
	}
	if captured[1].Role != "assistant" || len(captured[1].ToolCalls) != 1 || captured[1].ToolCalls[0].Function.Name != "edit" {
		t.Fatalf("assistant message = %+v, want the edit tool call", captured[1])
	}
	toolMsg := captured[2]
	if toolMsg.Role != "tool" || toolMsg.ToolCallID != "call_1" {
		t.Fatalf("third message = %+v, want tool result for call_1", toolMsg)
	}
	if !strings.Contains(toolMsg.Content, "edited "+target) {
		t.Fatalf("tool message content = %q, want the edit confirmation alongside the diagnostic", toolMsg.Content)
	}
	if !strings.Contains(toolMsg.Content, "Diagnostics:") || !strings.Contains(toolMsg.Content, "undefined: Foo") {
		t.Fatalf("tool message content = %q, want the diagnostic reaching the model on the second call", toolMsg.Content)
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
			if e.Reason != "low confidence, using default" {
				t.Fatalf("reason = %q, want low confidence, using default", e.Reason)
			}
		}
		if e.Kind == EventError {
			t.Fatalf("unexpected error: %s", e.Text)
		}
	}
}

func TestAgentMediumConfidenceHonorsJevChoice(t *testing.T) {
	cat := testCatalog(t)
	gw := mockGateway(t, [][]string{contentChunks("ok", 5, 1)})
	jevSrv := mockJev(t, "deepseek-v4-flash", 0.4)

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

	routed := false
	for _, e := range events {
		if e.Kind == EventRoute {
			routed = true
			if e.Model != "deepseek-v4-flash" {
				t.Fatalf("model = %q, want deepseek-v4-flash (0.40 is above the scaled threshold)", e.Model)
			}
			if e.Router != "jev" {
				t.Fatalf("router = %q, want jev", e.Router)
			}
			if e.Reason != "" {
				t.Fatalf("reason = %q, want empty for a direct jev choice", e.Reason)
			}
		}
		if e.Kind == EventError {
			t.Fatalf("unexpected error: %s", e.Text)
		}
	}
	if !routed {
		t.Fatal("no route event collected")
	}
}

func TestStepLimitByContext(t *testing.T) {
	ag := New(nil, nil, nil, testCatalog(t), tools.NewRegistry(), nil, false)
	if got := ag.stepLimit(); got != maxSteps {
		t.Fatalf("stepLimit = %d, want %d without a skill", got, maxSteps)
	}
	ag.activeSkill = "kspec-implement"
	if got := ag.stepLimit(); got != skillMaxSteps {
		t.Fatalf("stepLimit = %d, want %d with an active skill", got, skillMaxSteps)
	}
	ag.depth = 1
	if got := ag.stepLimit(); got != subagentMaxSteps {
		t.Fatalf("stepLimit = %d, want %d for a subagent", got, subagentMaxSteps)
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

func waitForEvent(t *testing.T, ch chan Event, kinds ...EventKind) Event {
	t.Helper()
	for {
		select {
		case e := <-ch:
			if e.Kind == EventError {
				t.Fatalf("unexpected error event: %s", e.Text)
			}
			for _, k := range kinds {
				if e.Kind == k {
					return e
				}
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for event %v", kinds)
		}
	}
}

func newTestAgent(t *testing.T, gwURL, jevURL string, sess *session.Writer, confirm bool) *Agent {
	t.Helper()
	cat := testCatalog(t)
	jevClient := jev.New("test-key")
	jevClient.BaseURL = jevURL
	return New(
		llm.New(gwURL, "gw-key", false),
		router.NewJev(jevClient),
		&router.HeuristicRouter{Default: cat.DefaultModel},
		cat,
		tools.NewRegistry(),
		sess,
		confirm,
	)
}

func modelsHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		data := []map[string]string{}
		for _, m := range testCatalog(t).Models {
			data = append(data, map[string]string{"id": m.Name})
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": data,
		})
	}
}

func decodeMessages(r *http.Request) []llm.Message {
	var body struct {
		Messages []llm.Message `json:"messages"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	return body.Messages
}

func blockingGateway(t *testing.T, firstChunk string, captured *[]llm.Message) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", modelsHandler(t))
	var call int
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		if call == 0 {
			call++
			chunk := fmt.Sprintf(`{"choices":[{"delta":{"role":"assistant","content":%s}}]}`, mustJSON(firstChunk))
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
			<-r.Context().Done()
			return
		}
		call++
		*captured = decodeMessages(r)
		for _, c := range contentChunks("recovered", 5, 1) {
			fmt.Fprintf(w, "data: %s\n\n", c)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func confirmGateway(t *testing.T, captured *[]llm.Message) *httptest.Server {
	t.Helper()
	responses := [][]string{
		toolCallChunks("call_1", "bash", `{"command":"echo hi"}`),
		contentChunks("done", 20, 1),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", modelsHandler(t))
	var call int
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if call >= len(responses) {
			t.Errorf("unexpected chat call %d", call)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if call == 1 {
			*captured = decodeMessages(r)
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
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestAgentCancelAbortsStream(t *testing.T) {
	var nextMessages []llm.Message
	gw := blockingGateway(t, "partial answer", &nextMessages)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)

	ag.Run("long running question")
	waitForEvent(t, ag.Events, EventDelta)
	ag.Cancel()
	aborted := waitForEvent(t, ag.Events, EventTurnAborted)
	if aborted.Text != "partial answer" {
		t.Fatalf("aborted text = %q, want %q", aborted.Text, "partial answer")
	}
	if aborted.Model != "glm-5.3" {
		t.Fatalf("aborted model = %q, want glm-5.3", aborted.Model)
	}

	ag.Run("follow up")
	waitForEvent(t, ag.Events, EventTurnDone)

	if len(nextMessages) != 2 {
		t.Fatalf("captured %d messages, want 2: %+v", len(nextMessages), nextMessages)
	}
	for _, m := range nextMessages {
		if strings.Contains(m.Content, "partial answer") {
			t.Fatalf("aborted partial leaked into LLM history: %+v", m)
		}
	}
}

func TestAgentCancelDuringConfirmAbortsTurn(t *testing.T) {
	var nextMessages []llm.Message
	gw := confirmGateway(t, &nextMessages)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, true)

	ag.Run("run echo hi")
	confirm := waitForEvent(t, ag.Events, EventConfirm)
	ag.Cancel()
	confirm.ApproveCh <- false
	waitForEvent(t, ag.Events, EventTurnAborted)

	ag.Run("continue")
	waitForEvent(t, ag.Events, EventTurnDone)

	if len(nextMessages) != 4 {
		t.Fatalf("captured %d messages, want 4: %+v", len(nextMessages), nextMessages)
	}
	if nextMessages[0].Role != "user" || nextMessages[0].Content != "run echo hi" {
		t.Fatalf("first message = %+v, want user request", nextMessages[0])
	}
	if nextMessages[1].Role != "assistant" || len(nextMessages[1].ToolCalls) != 1 || nextMessages[1].ToolCalls[0].ID != "call_1" {
		t.Fatalf("assistant message = %+v, want tool call call_1", nextMessages[1])
	}
	if nextMessages[2].Role != "tool" || nextMessages[2].Content != "user aborted this turn" || nextMessages[2].ToolCallID != "call_1" {
		t.Fatalf("tool message = %+v, want synthetic abort result for call_1", nextMessages[2])
	}
	if nextMessages[3].Role != "user" || nextMessages[3].Content != "continue" {
		t.Fatalf("last message = %+v, want user continue", nextMessages[3])
	}
}

func TestAgentCancelWhenIdleIsNoOp(t *testing.T) {
	gw := mockGateway(t, [][]string{contentChunks("ok", 5, 1)})
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)

	ag.Cancel()

	ag.Run("hello")
	waitForEvent(t, ag.Events, EventTurnDone)
}

func TestAgentDoubleCancelIsIdempotent(t *testing.T) {
	var nextMessages []llm.Message
	gw := blockingGateway(t, "partial answer", &nextMessages)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)

	ag.Run("long running question")
	waitForEvent(t, ag.Events, EventDelta)
	ag.Cancel()
	ag.Cancel()
	waitForEvent(t, ag.Events, EventTurnAborted)

	ag.Run("follow up")
	aborts := 1
	for {
		e := waitForEvent(t, ag.Events, EventTurnDone, EventTurnAborted)
		if e.Kind == EventTurnAborted {
			aborts++
			continue
		}
		break
	}
	if aborts != 1 {
		t.Fatalf("got %d turn_aborted events, want 1", aborts)
	}
}

func newSessionWriter(t *testing.T) *session.Writer {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	sess, err := session.NewWriter()
	if err != nil {
		t.Fatalf("session writer: %v", err)
	}
	t.Cleanup(func() { sess.Close() })
	return sess
}

func transcriptLines(t *testing.T, sess *session.Writer) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(sess.Path())
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("unmarshal transcript line %q: %v", line, err)
		}
		out = append(out, ev)
	}
	return out
}

func TestToolResultEventCarriesDiff(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.txt")
	args := fmt.Sprintf(`{"path":%s,"content":"line1\nline2\n"}`, mustJSON(target))
	gw := mockGateway(t, [][]string{
		toolCallChunks("call_1", "write", args),
		contentChunks("done", 20, 1),
	})
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	sess := newSessionWriter(t)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, sess, false)

	ag.Run("write a file")
	events := collectEvents(t, ag)

	var result *Event
	for i, e := range events {
		if e.Kind == EventToolResult {
			result = &events[i]
		}
	}
	if result == nil {
		t.Fatal("no tool_result event")
	}
	if len(result.Diff) != 2 {
		t.Fatalf("event diff = %+v, want 2 lines", result.Diff)
	}
	if result.Diff[0].Kind != '+' || result.Diff[0].Text != "line1" {
		t.Fatalf("event diff[0] = %+v, want +line1", result.Diff[0])
	}
	if result.Diff[1].Kind != '+' || result.Diff[1].Text != "line2" {
		t.Fatalf("event diff[1] = %+v, want +line2", result.Diff[1])
	}

	var transcriptDiff []string
	for _, ev := range transcriptLines(t, sess) {
		if ev["type"] != "tool_result" || ev["tool"] != "write" {
			continue
		}
		raw, ok := ev["diff"].([]any)
		if !ok {
			t.Fatalf("transcript tool_result for write has no diff: %+v", ev)
		}
		for _, d := range raw {
			s, _ := d.(string)
			transcriptDiff = append(transcriptDiff, s)
		}
	}
	if len(transcriptDiff) != 2 || transcriptDiff[0] != "+line1" || transcriptDiff[1] != "+line2" {
		t.Fatalf("transcript diff = %v, want [+line1 +line2]", transcriptDiff)
	}
}

func TestConfirmEventCarriesPendingDiff(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.txt")
	args := fmt.Sprintf(`{"path":%s,"content":"hello\n"}`, mustJSON(target))
	gw := mockGateway(t, [][]string{
		toolCallChunks("call_1", "write", args),
		contentChunks("done", 20, 1),
	})
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, true)

	ag.Run("write a file")
	confirm := waitForEvent(t, ag.Events, EventConfirm)
	if len(confirm.Diff) != 1 || confirm.Diff[0].Kind != '+' || confirm.Diff[0].Text != "hello" {
		t.Fatalf("confirm diff = %+v, want [+hello]", confirm.Diff)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("file exists before approval: %v", err)
	}

	confirm.ApproveCh <- true

	result := waitForEvent(t, ag.Events, EventToolResult)
	if len(result.Diff) != 1 || result.Diff[0].Kind != '+' || result.Diff[0].Text != "hello" {
		t.Fatalf("tool_result diff = %+v, want [+hello]", result.Diff)
	}
	waitForEvent(t, ag.Events, EventTurnDone)

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(data) != "hello\n" {
		t.Fatalf("file content = %q, want %q", string(data), "hello\n")
	}
}

func TestConfirmPendingDiffErrorStillEmitsConfirm(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "missing.txt")
	args := fmt.Sprintf(`{"path":%s,"old_string":"nope","new_string":"yes"}`, mustJSON(target))
	gw := mockGateway(t, [][]string{
		toolCallChunks("call_1", "edit", args),
		contentChunks("done", 20, 1),
	})
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, true)

	ag.Run("edit a file")
	confirm := waitForEvent(t, ag.Events, EventConfirm)
	if confirm.Diff != nil {
		t.Fatalf("confirm diff = %+v, want nil when pending diff fails", confirm.Diff)
	}

	confirm.ApproveCh <- false

	result := waitForEvent(t, ag.Events, EventToolResult)
	if result.Result != "user declined this tool call" {
		t.Fatalf("tool result = %q, want decline message", result.Result)
	}
	if result.Diff != nil {
		t.Fatalf("declined tool result diff = %+v, want nil", result.Diff)
	}
	waitForEvent(t, ag.Events, EventTurnDone)
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("file created after decline: %v", err)
	}
}

func TestNonEditorToolResultOmitsDiffInTranscript(t *testing.T) {
	gw := mockGateway(t, [][]string{
		toolCallChunks("call_1", "bash", `{"command":"echo hi"}`),
		contentChunks("done", 20, 1),
	})
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	sess := newSessionWriter(t)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, sess, false)

	ag.Run("run echo hi")
	events := collectEvents(t, ag)

	var result *Event
	for i, e := range events {
		if e.Kind == EventToolResult {
			result = &events[i]
		}
	}
	if result == nil {
		t.Fatal("no tool_result event")
	}
	if result.Diff != nil {
		t.Fatalf("bash tool_result diff = %+v, want nil", result.Diff)
	}
	for _, ev := range transcriptLines(t, sess) {
		if ev["type"] != "tool_result" {
			continue
		}
		if _, ok := ev["diff"]; ok {
			t.Fatalf("non-editor tool_result has diff in transcript: %+v", ev)
		}
	}
}

func TestEstimateUsesRealUsageAfterFirstCall(t *testing.T) {
	gw := mockGateway(t, [][]string{contentChunks("answer", 100, 5)})
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)

	if got := ag.estimateTokens(); got != 0 {
		t.Fatalf("estimate with no messages = %d, want 0", got)
	}

	ag.messages = []llm.Message{{Role: "user", Content: strings.Repeat("x", 396)}}
	wantHeuristic := int64((len("user") + 396) / 4)
	if got := ag.estimateTokens(); got != wantHeuristic {
		t.Fatalf("estimate before first call = %d, want heuristic %d", got, wantHeuristic)
	}

	ag.messages = nil
	ag.Run(strings.Repeat("x", 396))
	waitForEvent(t, ag.Events, EventTurnDone)

	if ag.lastPromptTokens != 100 {
		t.Fatalf("lastPromptTokens = %d, want 100 from mock usage", ag.lastPromptTokens)
	}
	wantChars := len("user") + 396
	if ag.lastEstimateChars != wantChars {
		t.Fatalf("lastEstimateChars = %d, want %d captured at call time", ag.lastEstimateChars, wantChars)
	}

	ag.messages = append(ag.messages, llm.Message{Role: "user", Content: strings.Repeat("y", 28)})
	delta := len("assistant") + len("answer") + len("user") + 28
	want := int64(100) + int64(delta)/4
	if got := ag.estimateTokens(); got != want {
		t.Fatalf("estimate after first call = %d, want %d (100 + %d/4)", got, want, delta)
	}
}

func TestEstimateFallsBackWhenUsageUnreported(t *testing.T) {
	gw := mockGateway(t, [][]string{contentChunks("answer", 0, 0)})
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)

	ag.Run(strings.Repeat("x", 396))
	waitForEvent(t, ag.Events, EventTurnDone)

	if ag.lastPromptTokens != 0 {
		t.Fatalf("lastPromptTokens = %d, want 0 when gateway reports no usage", ag.lastPromptTokens)
	}
	want := int64((len("user") + 396 + len("assistant") + len("answer")) / 4)
	if got := ag.estimateTokens(); got != want {
		t.Fatalf("estimate with unreported usage = %d, want heuristic %d", got, want)
	}
}

func failingChatGateway(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", modelsHandler(t))
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func waitForErrorEvent(t *testing.T, ch chan Event) Event {
	t.Helper()
	for {
		select {
		case e := <-ch:
			if e.Kind == EventError {
				return e
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for error event")
		}
	}
}

func TestEstimateUnchangedAfterStreamError(t *testing.T) {
	gw := failingChatGateway(t)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)

	ag.Run(strings.Repeat("x", 396))
	waitForErrorEvent(t, ag.Events)

	if ag.lastPromptTokens != 0 || ag.lastEstimateChars != 0 {
		t.Fatalf("lastPromptTokens=%d lastEstimateChars=%d, want 0/0 after failed stream", ag.lastPromptTokens, ag.lastEstimateChars)
	}
	want := int64((len("user") + 396) / 4)
	if got := ag.estimateTokens(); got != want {
		t.Fatalf("estimate after failed stream = %d, want heuristic %d", got, want)
	}
}

func smallWindowCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	data := `
models:
  - name: deepseek-v4.1-flash
    description: fast cheap model
    strengths: speed and cost
    input_price_per_m: 0.30
    output_price_per_m: 1.50
    tps_estimate: 100
    context_window: 2000
    tags: [fast, cheap]
  - name: glm-5.2
    description: balanced default model
    strengths: everyday work
    input_price_per_m: 1.00
    output_price_per_m: 4.00
    tps_estimate: 70
    context_window: 2000
    tags: [balanced]
  - name: glm-5.3
    description: strongest model
    strengths: hard problems
    input_price_per_m: 2.00
    output_price_per_m: 8.00
    tps_estimate: 50
    context_window: 2000
    tags: [quality]
default_model: glm-5.2
`
	cat, err := catalog.Parse([]byte(data))
	if err != nil {
		t.Fatalf("parse small window catalog: %v", err)
	}
	return cat
}

type chatCall struct {
	Model    string
	Messages []llm.Message
	Tools    []llm.Tool
}

func compactionGateway(t *testing.T, remoteModels []string, summaryChunks, mainChunks []string, calls *[]chatCall) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		data := []map[string]string{}
		for _, m := range remoteModels {
			data = append(data, map[string]string{"id": m})
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": data,
		})
	})
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model    string        `json:"model"`
			Messages []llm.Message `json:"messages"`
			Tools    []llm.Tool    `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode chat request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		*calls = append(*calls, chatCall{Model: body.Model, Messages: body.Messages, Tools: body.Tools})
		chunks := mainChunks
		if len(body.Tools) == 0 {
			if summaryChunks == nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			chunks = summaryChunks
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for _, c := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", c)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newCompactionAgent(t *testing.T, gwURL, jevURL string, sess *session.Writer) *Agent {
	t.Helper()
	cat := smallWindowCatalog(t)
	jevClient := jev.New("test-key")
	jevClient.BaseURL = jevURL
	return New(
		llm.New(gwURL, "gw-key", false),
		router.NewJev(jevClient),
		&router.HeuristicRouter{Default: cat.DefaultModel},
		cat,
		tools.NewRegistry(),
		sess,
		false,
	)
}

func bigHistory() []llm.Message {
	return []llm.Message{
		{Role: "user", Content: strings.Repeat("a", 3000)},
		{Role: "assistant", Content: strings.Repeat("b", 3000)},
		{Role: "user", Content: "q1"},
		{Role: "assistant", Content: "a1"},
		{Role: "user", Content: "q2"},
		{Role: "assistant", Content: "a2"},
	}
}

func TestCompactionBeforeOverflow(t *testing.T) {
	var calls []chatCall
	gw := compactionGateway(t,
		[]string{"deepseek-v4.1-flash", "glm-5.2", "glm-5.3"},
		contentChunks("dense summary of earlier work", 5, 2),
		contentChunks("final answer", 30, 3),
		&calls,
	)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newCompactionAgent(t, gw.URL, jevSrv.URL, nil)
	ag.messages = bigHistory()

	ag.Run("final question")
	events := collectEvents(t, ag)

	if len(calls) != 2 {
		t.Fatalf("gateway calls = %d, want 2 (summary before the overflowing main call)", len(calls))
	}
	summary, main := calls[0], calls[1]
	if summary.Model != "deepseek-v4.1-flash" {
		t.Fatalf("summary call model = %q, want deepseek-v4.1-flash", summary.Model)
	}
	if len(summary.Tools) != 0 {
		t.Fatalf("summary call sent %d tools, want none", len(summary.Tools))
	}
	if len(summary.Messages) != 1 || summary.Messages[0].Role != "user" {
		t.Fatalf("summary call messages = %+v, want single user message", summary.Messages)
	}
	if !strings.Contains(summary.Messages[0].Content, "q1") {
		t.Fatalf("summary prompt missing old turn content: %q", summary.Messages[0].Content)
	}
	if main.Model != "glm-5.3" {
		t.Fatalf("main call model = %q, want glm-5.3", main.Model)
	}

	var sawCompaction, sawDone bool
	for _, e := range events {
		switch e.Kind {
		case EventCompaction:
			sawCompaction = true
		case EventTurnDone:
			sawDone = true
		case EventError:
			t.Fatalf("unexpected error event: %s", e.Text)
		}
	}
	if !sawCompaction {
		t.Fatal("no compaction event")
	}
	if !sawDone {
		t.Fatal("no turn_done event — session did not continue after compaction")
	}
	if ag.lastPromptTokens != 30 {
		t.Fatalf("lastPromptTokens = %d, want 30 from main call (summary call must not update measurement)", ag.lastPromptTokens)
	}
}

func TestCompactionPreservesLastFour(t *testing.T) {
	var calls []chatCall
	gw := compactionGateway(t,
		[]string{"deepseek-v4.1-flash", "glm-5.2", "glm-5.3"},
		contentChunks("dense summary of earlier work", 5, 2),
		contentChunks("done", 30, 3),
		&calls,
	)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newCompactionAgent(t, gw.URL, jevSrv.URL, nil)
	seed := []llm.Message{
		{Role: "user", Content: strings.Repeat("a", 3000)},
		{Role: "assistant", Content: strings.Repeat("b", 3000)},
		{Role: "user", Content: "q1"},
		{Role: "assistant", Content: "thinking", ToolCalls: []llm.ToolCall{{ID: "call_9", Type: "function", Function: llm.FuncCall{Name: "bash", Arguments: `{"command":"ls"}`}}}},
		{Role: "tool", Content: "file1\nfile2", ToolCallID: "call_9"},
		{Role: "assistant", Content: "a2"},
	}
	ag.messages = append([]llm.Message{}, seed...)

	ag.Run("final question")
	collectEvents(t, ag)

	wantTail := append(append([]llm.Message{}, seed[3:]...), llm.Message{Role: "user", Content: "final question"})
	if len(calls) != 2 {
		t.Fatalf("gateway calls = %d, want 2", len(calls))
	}
	sent := calls[1].Messages
	if len(sent) != 1+preservedTailMessages {
		t.Fatalf("main call sent %d messages, want %d", len(sent), 1+preservedTailMessages)
	}
	if sent[0].Role != "system" || sent[0].Content != "dense summary of earlier work" {
		t.Fatalf("first sent message = %+v, want system message with summary", sent[0])
	}
	if !reflect.DeepEqual(sent[1:], wantTail) {
		t.Fatalf("sent tail = %+v, want byte-identical %+v", sent[1:], wantTail)
	}
	if ag.messages[0].Role != "system" || ag.messages[0].Content != "dense summary of earlier work" {
		t.Fatalf("first agent message = %+v, want system message with summary", ag.messages[0])
	}
	if !reflect.DeepEqual(ag.messages[1:1+preservedTailMessages], wantTail) {
		t.Fatalf("agent tail = %+v, want byte-identical %+v", ag.messages[1:1+preservedTailMessages], wantTail)
	}
}

func TestCompactionUsesFallbackModel(t *testing.T) {
	var calls []chatCall
	gw := compactionGateway(t,
		[]string{"glm-5.2", "glm-5.3"},
		contentChunks("fallback summary", 5, 2),
		contentChunks("done", 30, 3),
		&calls,
	)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newCompactionAgent(t, gw.URL, jevSrv.URL, nil)
	ag.messages = bigHistory()

	ag.Run("final question")
	collectEvents(t, ag)

	if len(calls) != 2 {
		t.Fatalf("gateway calls = %d, want 2", len(calls))
	}
	if calls[0].Model != "glm-5.2" {
		t.Fatalf("summary model = %q, want catalog default glm-5.2 when deepseek-v4.1-flash is unavailable", calls[0].Model)
	}
	if len(calls[0].Messages) != 1 || len(calls[0].Tools) != 0 {
		t.Fatalf("summary call = %+v, want single message without tools", calls[0])
	}
	if calls[1].Model != "glm-5.3" {
		t.Fatalf("main call model = %q, want glm-5.3", calls[1].Model)
	}
}

func TestCompactionEmitsEventAndTranscript(t *testing.T) {
	var calls []chatCall
	gw := compactionGateway(t,
		[]string{"deepseek-v4.1-flash", "glm-5.2", "glm-5.3"},
		contentChunks("dense summary of earlier work", 5, 2),
		contentChunks("done", 30, 3),
		&calls,
	)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	sess := newSessionWriter(t)
	ag := newCompactionAgent(t, gw.URL, jevSrv.URL, sess)
	ag.messages = bigHistory()

	ag.Run("final question")
	events := collectEvents(t, ag)

	var compaction *Event
	for i, e := range events {
		if e.Kind == EventCompaction {
			compaction = &events[i]
		}
	}
	if compaction == nil {
		t.Fatal("no compaction event")
	}
	if compaction.TokensBefore != 1516 || compaction.TokensAfter != 20 {
		t.Fatalf("tokens before/after = %d/%d, want 1516/20", compaction.TokensBefore, compaction.TokensAfter)
	}
	if compaction.TokensBefore <= compaction.TokensAfter {
		t.Fatalf("tokens before/after = %d/%d, want before > after", compaction.TokensBefore, compaction.TokensAfter)
	}

	var transcriptEvent map[string]any
	for _, ev := range transcriptLines(t, sess) {
		if ev["type"] == "compaction" {
			transcriptEvent = ev
		}
	}
	if transcriptEvent == nil {
		t.Fatal("no compaction event in transcript")
	}
	if transcriptEvent["tokens_before"] != float64(compaction.TokensBefore) || transcriptEvent["tokens_after"] != float64(compaction.TokensAfter) {
		t.Fatalf("transcript tokens = %v/%v, want %d/%d", transcriptEvent["tokens_before"], transcriptEvent["tokens_after"], compaction.TokensBefore, compaction.TokensAfter)
	}
}

func TestNeedsCompactionThreshold(t *testing.T) {
	ag := &Agent{}
	ag.messages = []llm.Message{{Role: "user", Content: strings.Repeat("x", 5596)}}
	if ag.needsCompaction(2000) {
		t.Fatal("needsCompaction at exactly 70% of window, want false (strictly greater)")
	}
	ag.messages = []llm.Message{{Role: "user", Content: strings.Repeat("x", 5600)}}
	if !ag.needsCompaction(2000) {
		t.Fatal("needsCompaction above 70% of window, want true")
	}
}

func TestCompactionTokensWithRealMeasurement(t *testing.T) {
	var calls []chatCall
	gw := compactionGateway(t,
		[]string{"deepseek-v4.1-flash", "glm-5.2", "glm-5.3"},
		contentChunks("dense summary of earlier work", 5, 2),
		contentChunks("answer", 100, 3),
		&calls,
	)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newCompactionAgent(t, gw.URL, jevSrv.URL, nil)

	ag.Run(strings.Repeat("x", 396))
	waitForEvent(t, ag.Events, EventTurnDone)
	if ag.lastPromptTokens != 100 || ag.lastEstimateChars != 400 {
		t.Fatalf("measurement = %d/%d, want 100/400 after first call", ag.lastPromptTokens, ag.lastEstimateChars)
	}

	ag.messages = bigHistory()
	ag.Run("final question")
	events := collectEvents(t, ag)

	var compaction *Event
	for i, e := range events {
		if e.Kind == EventCompaction {
			compaction = &events[i]
		}
	}
	if compaction == nil {
		t.Fatal("no compaction event")
	}
	if compaction.TokensBefore != 1516 {
		t.Fatalf("tokens before = %d, want 1516 (100 + 5665/4)", compaction.TokensBefore)
	}
	if compaction.TokensAfter != 21 {
		t.Fatalf("tokens after = %d, want 21 (100 + (81-400)/4)", compaction.TokensAfter)
	}
	if compaction.TokensBefore <= compaction.TokensAfter {
		t.Fatalf("tokens before/after = %d/%d, want before > after", compaction.TokensBefore, compaction.TokensAfter)
	}
}

func TestCompactionSkipsWhenHistoryFitsTail(t *testing.T) {
	var calls []chatCall
	gw := compactionGateway(t,
		[]string{"deepseek-v4.1-flash", "glm-5.2", "glm-5.3"},
		contentChunks("dense summary of earlier work", 5, 2),
		contentChunks("done", 30, 3),
		&calls,
	)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newCompactionAgent(t, gw.URL, jevSrv.URL, nil)
	ag.messages = []llm.Message{
		{Role: "user", Content: strings.Repeat("a", 3000)},
		{Role: "assistant", Content: strings.Repeat("b", 3000)},
		{Role: "user", Content: "q1"},
	}

	ag.Run("final question")
	events := collectEvents(t, ag)

	if len(calls) != 1 {
		t.Fatalf("gateway calls = %d, want 1 (no summary call when history fits the preserved tail)", len(calls))
	}
	if calls[0].Model != "glm-5.3" || len(calls[0].Messages) != 4 {
		t.Fatalf("main call = model %q with %d messages, want glm-5.3 with full 4-message history", calls[0].Model, len(calls[0].Messages))
	}
	for _, e := range events {
		if e.Kind == EventCompaction {
			t.Fatal("compaction event emitted without old turns to compact")
		}
		if e.Kind == EventError {
			t.Fatalf("unexpected error event: %s", e.Text)
		}
	}
}

func TestSummaryFailureAbortsCompactionSilently(t *testing.T) {
	cases := []struct {
		name          string
		summaryChunks []string
		wantCalls     int
	}{
		{"gateway error", nil, 4},
		{"empty summary", contentChunks("", 5, 2), 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls []chatCall
			gw := compactionGateway(t,
				[]string{"deepseek-v4.1-flash", "glm-5.2", "glm-5.3"},
				tc.summaryChunks,
				contentChunks("done", 30, 3),
				&calls,
			)
			jevSrv := mockJev(t, "glm-5.3", 0.9)
			ag := newCompactionAgent(t, gw.URL, jevSrv.URL, nil)
			ag.messages = []llm.Message{
				{Role: "user", Content: "run the command"},
				{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "call_1", Type: "function", Function: llm.FuncCall{Name: "bash", Arguments: `{"command":"cat big.log"}`}}}},
				{Role: "tool", Content: strings.Repeat("x", 50000), ToolCallID: "call_1"},
				{Role: "assistant", Content: "ok"},
				{Role: "user", Content: "q2"},
				{Role: "assistant", Content: "a2"},
			}

			ag.Run("final question")
			events := collectEvents(t, ag)

			if len(calls) != tc.wantCalls {
				t.Fatalf("gateway calls = %d, want %d", len(calls), tc.wantCalls)
			}
			main := calls[tc.wantCalls-1]
			if len(main.Messages) != 7 {
				t.Fatalf("main call sent %d messages, want 7 (truncation shortens content, never drops messages)", len(main.Messages))
			}
			var toolContent string
			for _, m := range main.Messages {
				if m.Role == "tool" {
					toolContent = m.Content
				}
			}
			want := strings.Repeat("x", toolResultTruncateChars) + "… (truncated)"
			if toolContent != want {
				t.Fatalf("tool content = %d bytes, want %d bytes truncated by the fallback after failed summary", len(toolContent), len(want))
			}
			sawDone := false
			for _, e := range events {
				switch e.Kind {
				case EventError:
					t.Fatalf("summary failure must be silent, got error event: %s", e.Text)
				case EventCompaction:
					t.Fatal("summary failure must not emit compaction event")
				case EventTurnDone:
					sawDone = true
				}
			}
			if !sawDone {
				t.Fatal("no turn_done — session must continue after failed summary")
			}
		})
	}
}

func TestTruncationRescuesGiantToolResult(t *testing.T) {
	var calls []chatCall
	gw := compactionGateway(t,
		[]string{"deepseek-v4.1-flash", "glm-5.2", "glm-5.3"},
		nil,
		contentChunks("done", 30, 3),
		&calls,
	)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newCompactionAgent(t, gw.URL, jevSrv.URL, nil)
	ag.messages = []llm.Message{
		{Role: "user", Content: "run the command"},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "call_1", Type: "function", Function: llm.FuncCall{Name: "bash", Arguments: `{"command":"cat big.log"}`}}}},
		{Role: "tool", Content: strings.Repeat("x", 50000), ToolCallID: "call_1"},
		{Role: "assistant", Content: "ok"},
		{Role: "user", Content: "q2"},
		{Role: "assistant", Content: "a2"},
	}

	ag.Run("final question")
	events := collectEvents(t, ag)

	if len(calls) != 4 {
		t.Fatalf("gateway calls = %d, want 4 (3 retried summary attempts + rescued main call)", len(calls))
	}
	main := calls[3]
	var toolContent string
	for _, m := range main.Messages {
		if m.Role == "tool" {
			toolContent = m.Content
		}
	}
	want := strings.Repeat("x", toolResultTruncateChars) + "… (truncated)"
	if toolContent != want {
		t.Fatalf("tool content = %d bytes, want %d bytes (first 2000 + truncation suffix)", len(toolContent), len(want))
	}
	chars := 0
	for _, m := range main.Messages {
		chars += len(m.Role) + len(m.Content) + len(m.ToolCallID)
		for _, tc := range m.ToolCalls {
			chars += len(tc.ID) + len(tc.Function.Name) + len(tc.Function.Arguments)
		}
	}
	if int64(chars/4) > 2000 {
		t.Fatalf("post-truncation prompt estimate = %d tokens, want within the 2000-token window", chars/4)
	}
	sawDone := false
	for _, e := range events {
		switch e.Kind {
		case EventError:
			t.Fatalf("unexpected error event: %s", e.Text)
		case EventTurnDone:
			sawDone = true
		}
	}
	if !sawDone {
		t.Fatal("no turn_done — the call after truncation must complete")
	}
}

func TestTruncationPreservesTail(t *testing.T) {
	var calls []chatCall
	gw := compactionGateway(t,
		[]string{"deepseek-v4.1-flash", "glm-5.2", "glm-5.3"},
		contentChunks("dense summary of earlier work", 5, 2),
		contentChunks("done", 30, 3),
		&calls,
	)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newCompactionAgent(t, gw.URL, jevSrv.URL, nil)
	giant := strings.Repeat("g", 50000)
	seed := []llm.Message{
		{Role: "user", Content: "run the command"},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "call_1", Type: "function", Function: llm.FuncCall{Name: "bash", Arguments: `{"command":"cat old.log"}`}}}},
		{Role: "tool", Content: strings.Repeat("x", 50000), ToolCallID: "call_1"},
		{Role: "assistant", Content: "ok1"},
		{Role: "user", Content: "q1"},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "call_2", Type: "function", Function: llm.FuncCall{Name: "bash", Arguments: `{"command":"cat tail.log"}`}}}},
		{Role: "tool", Content: giant, ToolCallID: "call_2"},
		{Role: "assistant", Content: "ok2"},
	}
	ag.messages = append([]llm.Message{}, seed...)

	ag.Run("final question")
	events := collectEvents(t, ag)

	if len(calls) != 2 {
		t.Fatalf("gateway calls = %d, want 2 (summary + main call)", len(calls))
	}
	main := calls[1]
	if len(main.Messages) != 1+preservedTailMessages {
		t.Fatalf("main call sent %d messages, want %d", len(main.Messages), 1+preservedTailMessages)
	}
	if main.Messages[0].Role != "system" || main.Messages[0].Content != "dense summary of earlier work" {
		t.Fatalf("first sent message = %+v, want system message with summary", main.Messages[0])
	}
	wantTail := append(append([]llm.Message{}, seed[5:]...), llm.Message{Role: "user", Content: "final question"})
	if !reflect.DeepEqual(main.Messages[1:], wantTail) {
		t.Fatalf("sent tail = %+v, want byte-identical %+v — giant tool result inside the preserved tail must never be truncated", main.Messages[1:], wantTail)
	}
	sawCompaction, sawDone := false, false
	for _, e := range events {
		switch e.Kind {
		case EventError:
			t.Fatalf("unexpected error event: %s", e.Text)
		case EventCompaction:
			sawCompaction = true
		case EventTurnDone:
			sawDone = true
		}
	}
	if !sawCompaction {
		t.Fatal("no compaction event — compaction must succeed and keep the tail intact")
	}
	if !sawDone {
		t.Fatal("no turn_done")
	}
}

func TestSummaryFailureFallsBackToTruncation(t *testing.T) {
	var calls []chatCall
	gw := compactionGateway(t,
		[]string{"deepseek-v4.1-flash", "glm-5.2", "glm-5.3"},
		contentChunks("", 5, 2),
		contentChunks("done", 30, 3),
		&calls,
	)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newCompactionAgent(t, gw.URL, jevSrv.URL, nil)
	ag.messages = []llm.Message{
		{Role: "user", Content: "run the command"},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "call_1", Type: "function", Function: llm.FuncCall{Name: "bash", Arguments: `{"command":"cat big1.log"}`}}}},
		{Role: "tool", Content: strings.Repeat("x", 50000), ToolCallID: "call_1"},
		{Role: "assistant", Content: "ok1"},
		{Role: "user", Content: "q1"},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "call_2", Type: "function", Function: llm.FuncCall{Name: "bash", Arguments: `{"command":"cat big2.log"}`}}}},
		{Role: "tool", Content: strings.Repeat("y", 2500), ToolCallID: "call_2"},
		{Role: "assistant", Content: "ok2"},
		{Role: "user", Content: "q2"},
		{Role: "assistant", Content: "a2"},
	}

	ag.Run("final question")
	events := collectEvents(t, ag)

	if len(calls) != 2 {
		t.Fatalf("gateway calls = %d, want 2 (failed summary + main call)", len(calls))
	}
	if len(calls[0].Tools) != 0 {
		t.Fatalf("summary call sent %d tools, want none", len(calls[0].Tools))
	}
	main := calls[1]
	var oldTool, recentTool string
	for _, m := range main.Messages {
		if m.Role != "tool" {
			continue
		}
		switch m.ToolCallID {
		case "call_1":
			oldTool = m.Content
		case "call_2":
			recentTool = m.Content
		}
	}
	wantTruncated := strings.Repeat("x", toolResultTruncateChars) + "… (truncated)"
	if oldTool != wantTruncated {
		t.Fatalf("oldest tool content = %d bytes, want %d bytes truncated after summary failure", len(oldTool), len(wantTruncated))
	}
	if wantIntact := strings.Repeat("y", 2500); recentTool != wantIntact {
		t.Fatalf("second tool content = %d bytes, want %d bytes intact — loop must stop once the estimate fits", len(recentTool), len(wantIntact))
	}
	sawDone := false
	for _, e := range events {
		switch e.Kind {
		case EventError:
			t.Fatalf("summary failure must degrade to truncation without error event, got: %s", e.Text)
		case EventCompaction:
			t.Fatal("failed summary must not emit compaction event")
		case EventTurnDone:
			sawDone = true
		}
	}
	if !sawDone {
		t.Fatal("no turn_done — turn must complete via truncation fallback")
	}
}

func TestTruncateOldToolResultsBoundaries(t *testing.T) {
	ag := &Agent{}
	ag.messages = []llm.Message{
		{Role: "tool", Content: strings.Repeat("a", toolResultTruncateChars), ToolCallID: "call_1"},
		{Role: "tool", Content: strings.Repeat("b", toolResultTruncateChars+1), ToolCallID: "call_2"},
		{Role: "tool", Content: strings.Repeat("c", 50000), ToolCallID: "call_3"},
		{Role: "user", Content: "filler"},
		{Role: "tool", Content: strings.Repeat("d", 50000), ToolCallID: "call_4"},
		{Role: "tool", Content: strings.Repeat("e", 50000), ToolCallID: "call_5"},
		{Role: "user", Content: "q"},
		{Role: "assistant", Content: "a"},
	}

	if !ag.truncateOldToolResults() {
		t.Fatal("expected truncation of the oldest oversized tool result outside the tail")
	}
	if got := ag.messages[0].Content; got != strings.Repeat("a", toolResultTruncateChars) {
		t.Fatalf("content at the exact %d-char boundary must remain untouched, got %d bytes", toolResultTruncateChars, len(got))
	}
	wantB := strings.Repeat("b", toolResultTruncateChars) + "… (truncated)"
	if got := ag.messages[1].Content; got != wantB {
		t.Fatalf("content of %d bytes must truncate to %d bytes, got %d bytes", toolResultTruncateChars+1, len(wantB), len(got))
	}
	if got := ag.messages[2].Content; got != strings.Repeat("c", 50000) {
		t.Fatalf("one call must truncate a single message, got second message at %d bytes", len(got))
	}

	if !ag.truncateOldToolResults() {
		t.Fatal("expected truncation of the next oversized tool result")
	}
	wantC := strings.Repeat("c", toolResultTruncateChars) + "… (truncated)"
	if got := ag.messages[2].Content; got != wantC {
		t.Fatalf("second call must truncate the next oldest message, got %d bytes", len(got))
	}

	if ag.truncateOldToolResults() {
		t.Fatal("already-truncated content must not truncate again — loop would never stop")
	}
	if got := ag.messages[4].Content; got != strings.Repeat("d", 50000) {
		t.Fatalf("tool result inside the preserved tail must remain untouched, got %d bytes", len(got))
	}
	if got := ag.messages[5].Content; got != strings.Repeat("e", 50000) {
		t.Fatalf("tool result inside the preserved tail must remain untouched, got %d bytes", len(got))
	}
}

func lastTranscriptEvent(t *testing.T, sess *session.Writer) map[string]any {
	t.Helper()
	lines := transcriptLines(t, sess)
	if len(lines) == 0 {
		t.Fatal("empty transcript")
	}
	return lines[len(lines)-1]
}

func snapshotMessages(t *testing.T, ev map[string]any) []llm.Message {
	t.Helper()
	if ev["type"] != "snapshot" {
		t.Fatalf("last transcript event type = %v, want snapshot", ev["type"])
	}
	raw, err := json.Marshal(ev["messages"])
	if err != nil {
		t.Fatalf("marshal snapshot messages: %v", err)
	}
	var got []llm.Message
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal snapshot messages: %v", err)
	}
	return got
}

func TestSnapshotWrittenAfterTurnDone(t *testing.T) {
	gw := mockGateway(t, [][]string{contentChunks("final answer", 10, 2)})
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	sess := newSessionWriter(t)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, sess, false)

	ag.Run("hello")
	waitForEvent(t, ag.Events, EventTurnDone)

	got := snapshotMessages(t, lastTranscriptEvent(t, sess))
	want := []llm.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "final answer"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot messages = %+v, want %+v", got, want)
	}
}

func TestSnapshotWrittenAfterAbort(t *testing.T) {
	var nextMessages []llm.Message
	gw := confirmGateway(t, &nextMessages)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	sess := newSessionWriter(t)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, sess, true)

	ag.Run("run echo hi")
	confirm := waitForEvent(t, ag.Events, EventConfirm)
	ag.Cancel()
	confirm.ApproveCh <- false
	waitForEvent(t, ag.Events, EventTurnAborted)

	got := snapshotMessages(t, lastTranscriptEvent(t, sess))
	want := []llm.Message{
		{Role: "user", Content: "run echo hi"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call_1", Type: "function", Function: llm.FuncCall{Name: "bash", Arguments: `{"command":"echo hi"}`}}}},
		{Role: "tool", Content: "user aborted this turn", ToolCallID: "call_1"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot messages = %+v, want %+v with synthetic abort tool result", got, want)
	}
}

func TestSnapshotWrittenAfterStreamError(t *testing.T) {
	gw := failingChatGateway(t)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	sess := newSessionWriter(t)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, sess, false)

	ag.Run("hello")
	waitForErrorEvent(t, ag.Events)

	var snapshot map[string]any
	for _, ev := range transcriptLines(t, sess) {
		if ev["type"] == "snapshot" {
			snapshot = ev
		}
	}
	if snapshot == nil {
		t.Fatal("no snapshot event in transcript after stream error")
	}
	got := snapshotMessages(t, snapshot)
	want := []llm.Message{{Role: "user", Content: "hello"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot messages = %+v, want %+v (user message preserved for resume)", got, want)
	}
}

func resumeGateway(t *testing.T, modelsCalls *int, captured *[][]llm.Message) *httptest.Server {
	t.Helper()
	responses := [][]string{
		contentChunks("first answer", 10, 2),
		contentChunks("second answer", 10, 2),
		contentChunks("third answer", 10, 2),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		*modelsCalls++
		modelsHandler(t)(w, r)
	})
	var call int
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if call >= len(responses) {
			t.Errorf("unexpected chat call %d", call)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		*captured = append(*captured, decodeMessages(r))
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
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestSetMessagesClearsCandidates(t *testing.T) {
	var modelsCalls int
	var sent [][]llm.Message
	gw := resumeGateway(t, &modelsCalls, &sent)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)

	ag.Run("first")
	waitForEvent(t, ag.Events, EventTurnDone)
	if modelsCalls != 1 {
		t.Fatalf("models calls = %d after first run, want 1", modelsCalls)
	}

	ag.Run("second")
	waitForEvent(t, ag.Events, EventTurnDone)
	if modelsCalls != 1 {
		t.Fatalf("models calls = %d after second run, want 1 (candidates cached)", modelsCalls)
	}

	loaded := []llm.Message{
		{Role: "user", Content: "old question"},
		{Role: "assistant", Content: "old answer"},
	}
	ag.SetMessages(loaded)
	if !reflect.DeepEqual(ag.messages, loaded) {
		t.Fatalf("messages = %+v, want loaded history", ag.messages)
	}
	if ag.candidates != nil {
		t.Fatal("candidates not cleared by SetMessages")
	}
	if ag.sessionCost != 0 {
		t.Fatalf("sessionCost = %v, want 0 after SetMessages", ag.sessionCost)
	}

	ag.Run("third")
	waitForEvent(t, ag.Events, EventTurnDone)

	if modelsCalls != 2 {
		t.Fatalf("models calls = %d after resume run, want 2 (candidates revalidated against gateway)", modelsCalls)
	}
	if len(sent) != 3 {
		t.Fatalf("chat calls = %d, want 3", len(sent))
	}
	third := sent[2]
	if len(third) != 3 || third[0].Content != "old question" || third[1].Content != "old answer" || third[2].Content != "third" {
		t.Fatalf("third call messages = %+v, want loaded history followed by the new user message", third)
	}
}

func TestAgentRecordsTPSAfterCall(t *testing.T) {
	store := telemetry.New(filepath.Join(t.TempDir(), "telemetry.json"))
	gw := slowGateway(t, 25*time.Millisecond, [][]string{
		contentChunks("answer", 10, 5),
		contentChunks("quiet", 10, 0),
	})
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)
	ag.Telemetry = store

	ag.Run("hello")
	done := waitForEvent(t, ag.Events, EventTurnDone)
	if done.TPS <= 0 {
		t.Fatalf("turn tps = %v, want > 0 with completion tokens", done.TPS)
	}

	mean, samples := store.GetMean("glm-5.3")
	if samples != 1 {
		t.Fatalf("samples = %d, want 1 after the call", samples)
	}
	if mean != done.TPS {
		t.Fatalf("GetMean = %v, want the exact sample recorded from the call %v", mean, done.TPS)
	}
	if m, n := store.GetMean("glm-5.2"); m != 0 || n != 0 {
		t.Fatalf("GetMean(glm-5.2) = %v/%d, want 0/0 — only the called model is recorded", m, n)
	}

	ag.Run("hello again")
	waitForEvent(t, ag.Events, EventTurnDone)

	mean, samples = store.GetMean("glm-5.3")
	if samples != 1 {
		t.Fatalf("samples = %d, want still 1 — a call without completion tokens must not record", samples)
	}
	if mean != done.TPS {
		t.Fatalf("GetMean = %v, want unchanged %v after the call without completion tokens", mean, done.TPS)
	}
}

func TestAgentRecordsTPSAfterEachStreamStep(t *testing.T) {
	store := telemetry.New(filepath.Join(t.TempDir(), "telemetry.json"))
	gw := slowGateway(t, 25*time.Millisecond, [][]string{
		toolCallChunks("call_1", "bash", `{"command":"echo hi"}`),
		contentChunks("done", 20, 1),
	})
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)
	ag.Telemetry = store

	ag.Run("run echo hi")
	waitForEvent(t, ag.Events, EventTurnDone)

	_, samples := store.GetMean("glm-5.3")
	if samples != 2 {
		t.Fatalf("samples = %d, want 2 — every stream step with completion tokens records", samples)
	}
}

func TestCandidatesCarryMeasuredTPS(t *testing.T) {
	store := telemetry.New(filepath.Join(t.TempDir(), "telemetry.json"))
	for i := 0; i < 5; i++ {
		store.Record("glm-5.3", 50)
	}
	var criteria map[string]string
	jevSrv := capturingJev(t, "glm-5.3", 0.9, &criteria)
	gw := mockGateway(t, [][]string{contentChunks("answer", 10, 5)})
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)
	ag.Telemetry = store

	ag.Run("hello")
	waitForEvent(t, ag.Events, EventTurnDone)

	if criteria == nil {
		t.Fatal("jev received no criteria")
	}
	got, ok := criteria["glm-5.3"]
	if !ok {
		t.Fatalf("criteria missing glm-5.3: %v", criteria)
	}
	if !strings.Contains(got, "~50 tok/s") || !strings.Contains(got, "measured over 5 calls") {
		t.Fatalf("glm-5.3 criteria = %q, want ~50 tok/s measured over 5 calls", got)
	}
	other, ok := criteria["glm-5.2"]
	if !ok {
		t.Fatalf("criteria missing glm-5.2: %v", criteria)
	}
	if strings.Contains(other, "measured") {
		t.Fatalf("glm-5.2 criteria = %q, want estimate text without measured", other)
	}
}

func fixedClock(at time.Time) func() time.Time {
	return func() time.Time { return at }
}

func numberedLines(from, to int) string {
	lines := make([]string, 0, to-from+1)
	for i := from; i <= to; i++ {
		lines = append(lines, fmt.Sprintf("line-%d", i))
	}
	return strings.Join(lines, "\n")
}

func seqLines(from, to int) string {
	lines := make([]string, 0, to-from+1)
	for i := from; i <= to; i++ {
		lines = append(lines, fmt.Sprintf("%d", i))
	}
	return strings.Join(lines, "\n")
}

func TestToolOutputThrottled(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	current := base
	clock := func() time.Time { return current }
	var events []Event
	collector := newToolOutputCollector("bash", func(e Event) { events = append(events, e) }, clock)

	for i := 1; i <= 100; i++ {
		collector.onLine(fmt.Sprintf("line-%d", i))
	}
	if len(events) != 0 {
		t.Fatalf("events = %d, want 0 while 100 lines arrive inside the same throttle window", len(events))
	}

	current = base.Add(60 * time.Millisecond)
	collector.onLine("line-101")
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1 grouped event after the window elapses", len(events))
	}
	if events[0].Kind != EventToolOutput || events[0].Tool != "bash" {
		t.Fatalf("event = %+v, want tool_output for bash", events[0])
	}
	if events[0].Text != numberedLines(1, 101) {
		t.Fatalf("grouped text = %q, want the 100 same-instant lines grouped with the window trigger", events[0].Text)
	}

	for i := 102; i <= 110; i++ {
		collector.onLine(fmt.Sprintf("line-%d", i))
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want still 1 inside the second window", len(events))
	}

	current = base.Add(120 * time.Millisecond)
	collector.onLine("line-111")
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2 after the second window elapses", len(events))
	}
	if events[1].Text != numberedLines(102, 111) {
		t.Fatalf("second event text = %q, want lines 102-111 joined", events[1].Text)
	}

	for i := 112; i <= 115; i++ {
		collector.onLine(fmt.Sprintf("line-%d", i))
	}
	collector.flush()
	if len(events) != 3 {
		t.Fatalf("events = %d, want 3 after the final flush", len(events))
	}
	if events[2].Text != numberedLines(112, 115) {
		t.Fatalf("flush text = %q, want the remaining lines 112-115 joined", events[2].Text)
	}

	collector.flush()
	if len(events) != 3 {
		t.Fatal("flush without pending lines emitted an extra event")
	}

	var rebuilt []string
	for _, e := range events {
		rebuilt = append(rebuilt, strings.Split(e.Text, "\n")...)
	}
	var want []string
	for i := 1; i <= 115; i++ {
		want = append(want, fmt.Sprintf("line-%d", i))
	}
	if !reflect.DeepEqual(rebuilt, want) {
		t.Fatalf("events rebuilt %d lines, want all 115 lines in order with none lost", len(rebuilt))
	}
}

func TestToolOutputFlushedBeforeToolResult(t *testing.T) {
	gw := mockGateway(t, [][]string{
		toolCallChunks("call_1", "bash", `{"command":"seq 1 100"}`),
		toolCallChunks("call_2", "bash", `{"command":"seq 101 110"}`),
		contentChunks("done", 20, 1),
	})
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)
	ag.now = fixedClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	ag.Run("run verbose commands")
	events := collectEvents(t, ag)

	var seq []EventKind
	var outputs []string
	results := 0
	for _, e := range events {
		switch e.Kind {
		case EventToolStart, EventToolOutput, EventToolResult:
			seq = append(seq, e.Kind)
			if e.Kind == EventToolOutput {
				outputs = append(outputs, e.Text)
			}
			if e.Kind == EventToolResult {
				results++
			}
		case EventError:
			t.Fatalf("unexpected error event: %s", e.Text)
		}
	}
	wantSeq := []EventKind{
		EventToolStart, EventToolOutput, EventToolResult,
		EventToolStart, EventToolOutput, EventToolResult,
	}
	if !reflect.DeepEqual(seq, wantSeq) {
		t.Fatalf("tool event sequence = %v, want %v (final flush right before each result)", seq, wantSeq)
	}
	if results != 2 {
		t.Fatalf("tool_result events = %d, want 2", results)
	}
	if len(outputs) != 2 {
		t.Fatalf("tool_output events = %d, want 2 (one grouped event per command)", len(outputs))
	}
	if outputs[0] != seqLines(1, 100) {
		t.Fatalf("first output = %q, want lines 1-100 grouped in a single event", outputs[0])
	}
	if outputs[1] != seqLines(101, 110) {
		t.Fatalf("second output = %q, want lines 101-110 grouped in a single event", outputs[1])
	}
}

func toolTurnGateway(t *testing.T, command string, captured *[]llm.Message) *httptest.Server {
	t.Helper()
	responses := [][]string{
		toolCallChunks("call_1", "bash", fmt.Sprintf(`{"command":%s}`, mustJSON(command))),
		contentChunks("done", 20, 1),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", modelsHandler(t))
	var call int
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if call >= len(responses) {
			t.Errorf("unexpected chat call %d", call)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if call == 1 {
			*captured = decodeMessages(r)
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
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestToolOutputNotWrittenToTranscript(t *testing.T) {
	var captured []llm.Message
	gw := toolTurnGateway(t, "seq 1 20", &captured)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	sess := newSessionWriter(t)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, sess, false)
	ag.now = fixedClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	ag.Run("run a verbose command")
	events := collectEvents(t, ag)

	outputs := 0
	var output *Event
	var result *Event
	for i, e := range events {
		switch e.Kind {
		case EventToolOutput:
			outputs++
			output = &events[i]
		case EventToolResult:
			result = &events[i]
		case EventError:
			t.Fatalf("unexpected error event: %s", e.Text)
		}
	}
	if outputs != 1 {
		t.Fatalf("tool_output events = %d, want 1 for the verbose command", outputs)
	}
	if output.Text != seqLines(1, 20) {
		t.Fatalf("tool_output text = %q, want lines 1-20 grouped", output.Text)
	}
	if result == nil {
		t.Fatal("no tool_result event")
	}
	if result.Result != seqLines(1, 20)+"\n" {
		t.Fatalf("tool_result = %q, want consolidated seq output", result.Result)
	}

	for _, ev := range transcriptLines(t, sess) {
		if ev["type"] == "tool_output" {
			t.Fatalf("transcript recorded a tool_output event (REQ-005): %+v", ev)
		}
	}
	var transcriptResult map[string]any
	for _, ev := range transcriptLines(t, sess) {
		if ev["type"] == "tool_result" && ev["tool"] == "bash" {
			transcriptResult = ev
		}
	}
	if transcriptResult == nil {
		t.Fatal("no tool_result in transcript")
	}
	if transcriptResult["result"] != seqLines(1, 20)+"\n" {
		t.Fatalf("transcript tool_result = %v, want consolidated output only", transcriptResult["result"])
	}

	if len(captured) != 3 {
		t.Fatalf("gateway received %d messages, want 3 (user, assistant tool call, consolidated tool result)", len(captured))
	}
	toolMsg := captured[2]
	if toolMsg.Role != "tool" || toolMsg.ToolCallID != "call_1" {
		t.Fatalf("third message = %+v, want consolidated tool result for call_1", toolMsg)
	}
	if toolMsg.Content != seqLines(1, 20)+"\n" {
		t.Fatalf("tool message content = %q, want consolidated output only (gateway contract unchanged)", toolMsg.Content)
	}
}

func TestToolOutputPartialTailNotLost(t *testing.T) {
	args := fmt.Sprintf(`{"command":%s}`, mustJSON(`printf 'one\ntwo\npartial-tail'`))
	gw := mockGateway(t, [][]string{
		toolCallChunks("call_1", "bash", args),
		contentChunks("done", 20, 1),
	})
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)
	ag.now = fixedClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	ag.Run("print without trailing newline")
	events := collectEvents(t, ag)

	outputs := 0
	var output *Event
	var result *Event
	for i, e := range events {
		switch e.Kind {
		case EventToolOutput:
			outputs++
			output = &events[i]
		case EventToolResult:
			result = &events[i]
		case EventError:
			t.Fatalf("unexpected error event: %s", e.Text)
		}
	}
	if outputs != 1 {
		t.Fatalf("tool_output events = %d, want 1", outputs)
	}
	if output.Text != "one\ntwo\npartial-tail" {
		t.Fatalf("tool_output text = %q, want the partial tail line flushed with the earlier lines", output.Text)
	}
	if result == nil {
		t.Fatal("no tool_result event")
	}
	if result.Result != "one\ntwo\npartial-tail" {
		t.Fatalf("tool_result = %q, want consolidated output including the partial tail", result.Result)
	}
}

func TestSilentToolEmitsNoToolOutput(t *testing.T) {
	gw := mockGateway(t, [][]string{
		toolCallChunks("call_1", "bash", `{"command":"true"}`),
		contentChunks("done", 20, 1),
	})
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)
	ag.now = fixedClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	ag.Run("run a silent command")
	events := collectEvents(t, ag)

	outputs := 0
	var result *Event
	for i, e := range events {
		switch e.Kind {
		case EventToolOutput:
			outputs++
		case EventToolResult:
			result = &events[i]
		case EventError:
			t.Fatalf("unexpected error event: %s", e.Text)
		}
	}
	if outputs != 0 {
		t.Fatalf("tool_output events = %d, want 0 for a silent command", outputs)
	}
	if result == nil {
		t.Fatal("no tool_result event")
	}
	if result.Result != "(no output)" {
		t.Fatalf("tool_result = %q, want (no output)", result.Result)
	}
}

func testImageURI() string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("pngdata"))
}

func imageParts(text string) []llm.ContentPart {
	return []llm.ContentPart{
		{Type: "text", Text: text},
		{Type: "image_url", ImageURL: &llm.ImageURL{URL: testImageURI()}},
	}
}

func testAttachmentMeta() []session.AttachmentMeta {
	return []session.AttachmentMeta{{Name: "shot.png", Size: 7}}
}

func catalogNames(cat *catalog.Catalog) []string {
	names := make([]string, 0, len(cat.Models))
	for _, m := range cat.Models {
		names = append(names, m.Name)
	}
	return names
}

func bodyCaptureGateway(t *testing.T, remoteModels []string, chunks []string, bodies *[]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		data := []map[string]string{}
		for _, m := range remoteModels {
			data = append(data, map[string]string{"id": m})
		}
		json.NewEncoder(w).Encode(map[string]any{"data": data})
	})
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read chat body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		*bodies = append(*bodies, string(raw))
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for _, c := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", c)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func stateCapturingJev(t *testing.T, choice string, confidence float64, capturedState *string, capturedCriteria *map[string]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/systemone", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			State     string `json:"state"`
			Questions map[string]struct {
				Criteria map[string]string `json:"criteria"`
			} `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode jev request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		*capturedState = body.State
		*capturedCriteria = body.Questions["model_choice"].Criteria
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

func visionCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	data := `
models:
  - name: deepseek-v4-flash
    description: fast cheap model
    strengths: speed and cost
    input_price_per_m: 0.30
    output_price_per_m: 1.20
    tps_estimate: 120
    context_window: 128000
    tags: [fast]
  - name: glm-5.2
    description: balanced model with vision
    strengths: everyday work
    input_price_per_m: 1.00
    output_price_per_m: 4.00
    tps_estimate: 70
    context_window: 200000
    tags: [balanced]
    vision: true
  - name: glm-5.3
    description: strongest model with vision
    strengths: hard problems
    input_price_per_m: 2.00
    output_price_per_m: 8.00
    tps_estimate: 50
    context_window: 200000
    tags: [quality]
    vision: true
default_model: glm-5.2
`
	cat, err := catalog.Parse([]byte(data))
	if err != nil {
		t.Fatalf("parse vision catalog: %v", err)
	}
	return cat
}

func newCatalogAgent(t *testing.T, cat *catalog.Catalog, gwURL, jevURL string, sess *session.Writer, confirm bool) *Agent {
	t.Helper()
	jevClient := jev.New("test-key")
	jevClient.BaseURL = jevURL
	return New(
		llm.New(gwURL, "gw-key", false),
		router.NewJev(jevClient),
		&router.HeuristicRouter{Default: cat.DefaultModel},
		cat,
		tools.NewRegistry(),
		sess,
		confirm,
	)
}

func TestVisionFilterRoutesOnlyVisionModels(t *testing.T) {
	cat := visionCatalog(t)
	var bodies []string
	gw := bodyCaptureGateway(t, []string{"deepseek-v4-flash", "glm-5.2", "glm-5.3"}, contentChunks("vision answer", 10, 3), &bodies)
	var state string
	var criteria map[string]string
	jevSrv := stateCapturingJev(t, "glm-5.3", 0.88, &state, &criteria)
	ag := newCatalogAgent(t, cat, gw.URL, jevSrv.URL, nil, false)

	ag.RunWithAttachments("what is in this image?", imageParts("what is in this image?"), testAttachmentMeta())
	events := collectEvents(t, ag)

	for _, e := range events {
		if e.Kind == EventError {
			t.Fatalf("unexpected error event: %s", e.Text)
		}
	}
	if criteria == nil {
		t.Fatal("jev received no criteria — filtering must not skip the jev when multiple vision candidates exist")
	}
	if len(criteria) != 2 {
		t.Fatalf("jev criteria = %v, want exactly the 2 vision models", criteria)
	}
	for name := range criteria {
		if name != "glm-5.2" && name != "glm-5.3" {
			t.Fatalf("non-vision model %q offered to jev: %v", name, criteria)
		}
	}
	if !strings.Contains(state, "image attachments") {
		t.Fatalf("jev state = %q, want image attachments mention", state)
	}
	if strings.Contains(state, "base64") {
		t.Fatalf("jev state leaked image data: %q", state)
	}
	if len(bodies) != 1 {
		t.Fatalf("chat calls = %d, want 1", len(bodies))
	}
	if !strings.Contains(bodies[0], `"model":"glm-5.3"`) {
		t.Fatalf("chat request must go to the jev choice glm-5.3: %s", bodies[0])
	}
}

func TestNoVisionModelPreservesAttachments(t *testing.T) {
	var bodies []string
	gw := bodyCaptureGateway(t, []string{"glm-5.2"}, contentChunks("unused", 5, 1), &bodies)
	jevSrv := mockJev(t, "glm-5.2", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)

	ag.RunWithAttachments("describe this image", imageParts("describe this image"), testAttachmentMeta())
	ev := waitForErrorEvent(t, ag.Events)

	if ev.Text != "no vision-capable model available" {
		t.Fatalf("error = %q, want no vision-capable model available", ev.Text)
	}
	if len(ag.messages) != 0 {
		t.Fatalf("messages = %+v, want empty — the turn must not be consumed", ag.messages)
	}
	if len(bodies) != 0 {
		t.Fatalf("chat calls = %d, want 0 — no LLM call without vision candidates", len(bodies))
	}
}

func TestRequestContainsContentParts(t *testing.T) {
	cat := testCatalog(t)
	var bodies []string
	gw := bodyCaptureGateway(t, catalogNames(cat), contentChunks("described", 10, 2), &bodies)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)

	ag.RunWithAttachments("describe the screenshot", imageParts("describe the screenshot"), testAttachmentMeta())
	waitForEvent(t, ag.Events, EventTurnDone)

	if len(bodies) != 1 {
		t.Fatalf("chat calls = %d, want 1", len(bodies))
	}
	wantContent := `"content":[{"type":"text","text":"describe the screenshot"},{"type":"image_url","image_url":{"url":"` + testImageURI() + `"}}]`
	if !strings.Contains(bodies[0], wantContent) {
		t.Fatalf("request content must be a parts array with text then image_url:\n%s", bodies[0])
	}
	var req struct {
		Messages []llm.Message `json:"messages"`
	}
	if err := json.Unmarshal([]byte(bodies[0]), &req); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	if len(req.Messages) != 1 || req.Messages[0].Role != "user" {
		t.Fatalf("messages = %+v, want single user message", req.Messages)
	}
	parts := req.Messages[0].ContentParts
	if len(parts) != 2 {
		t.Fatalf("content parts = %d, want 2 (text + image)", len(parts))
	}
	if parts[0].Type != "text" || parts[0].Text != "describe the screenshot" {
		t.Fatalf("first part = %+v, want text part with the prompt", parts[0])
	}
	if parts[1].Type != "image_url" || parts[1].ImageURL == nil || parts[1].ImageURL.URL != testImageURI() {
		t.Fatalf("second part = %+v, want image_url with the data URI", parts[1])
	}
}

func TestTranscriptHasNoBase64(t *testing.T) {
	gw := mockGateway(t, [][]string{contentChunks("it shows a terminal", 10, 2)})
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	sess := newSessionWriter(t)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, sess, false)

	ag.RunWithAttachments("what is in this image?", imageParts("what is in this image?"), testAttachmentMeta())
	waitForEvent(t, ag.Events, EventTurnDone)

	data, err := os.ReadFile(sess.Path())
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	if strings.Contains(string(data), "base64,") {
		t.Fatal("transcript leaked base64 image data")
	}
	if !strings.Contains(string(data), `"[omitted]"`) {
		t.Fatal("snapshot missing [omitted] placeholder for image parts")
	}

	var userEvent map[string]any
	for _, ev := range transcriptLines(t, sess) {
		if ev["type"] == "user" {
			userEvent = ev
		}
	}
	if userEvent == nil {
		t.Fatal("no user event in transcript")
	}
	if userEvent["content"] != "what is in this image?" {
		t.Fatalf("user event content = %v", userEvent["content"])
	}
	atts, ok := userEvent["attachments"].([]any)
	if !ok || len(atts) != 1 {
		t.Fatalf("user event attachments = %v, want 1 entry", userEvent["attachments"])
	}
	entry, _ := atts[0].(map[string]any)
	if entry["name"] != "shot.png" || entry["size"] != float64(7) {
		t.Fatalf("attachment entry = %v, want {name: shot.png, size: 7}", entry)
	}
}

func TestPinWithoutVisionIgnoredWithAttachments(t *testing.T) {
	cat := visionCatalog(t)
	var bodies []string
	gw := bodyCaptureGateway(t, []string{"deepseek-v4-flash", "glm-5.2", "glm-5.3"}, contentChunks("ok", 5, 1), &bodies)
	jevSrv := mockJev(t, "glm-5.2", 0.9)
	ag := newCatalogAgent(t, cat, gw.URL, jevSrv.URL, nil, false)
	ag.SetPinned("deepseek-v4-flash")

	ag.RunWithAttachments("describe this", imageParts("describe this"), nil)
	events := collectEvents(t, ag)

	var route *Event
	for i, e := range events {
		if e.Kind == EventError {
			t.Fatalf("unexpected error event: %s", e.Text)
		}
		if e.Kind == EventRoute {
			route = &events[i]
		}
	}
	if route == nil {
		t.Fatal("no route event")
	}
	if route.Model != "glm-5.2" || route.Router != "jev" {
		t.Fatalf("route = %+v, want glm-5.2 via jev — pin on a non-vision model must be ignored with attachments", route)
	}

	ag.SetPinned("glm-5.3")
	ag.RunWithAttachments("describe again", imageParts("describe again"), nil)
	events = collectEvents(t, ag)
	route = nil
	for i, e := range events {
		if e.Kind == EventError {
			t.Fatalf("unexpected error event: %s", e.Text)
		}
		if e.Kind == EventRoute {
			route = &events[i]
		}
	}
	if route == nil {
		t.Fatal("no route event on second turn")
	}
	if route.Model != "glm-5.3" || route.Router != "pin" {
		t.Fatalf("route = %+v, want pinned glm-5.3 — pin on a vision model must win with attachments", route)
	}
}

func toolNames(defs []llm.Tool) []string {
	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Function.Name
	}
	return names
}

func hasTool(defs []llm.Tool, name string) bool {
	for _, d := range defs {
		if d.Function.Name == name {
			return true
		}
	}
	return false
}

func TestAttachTaskToolDepthGuard(t *testing.T) {
	ag := newTestAgent(t, "http://gateway.unused", "http://jev.unused", nil, false)

	ag.AttachTaskTool()
	if !hasTool(ag.Tools.Definitions(), taskToolName) {
		t.Fatalf("main agent tools = %v, want task registered at depth 0", toolNames(ag.Tools.Definitions()))
	}
	if !ag.Tools.IsMutating(taskToolName) {
		t.Fatal("task must be mutating to inherit --confirm rules")
	}

	sub := ag.newSubagent("subtask", "")
	if hasTool(sub.Tools.Definitions(), taskToolName) {
		t.Fatalf("subagent tools = %v, want no task (fresh registry without it)", toolNames(sub.Tools.Definitions()))
	}
	sub.AttachTaskTool()
	if hasTool(sub.Tools.Definitions(), taskToolName) {
		t.Fatalf("subagent tools = %v, want the depth guard to refuse registration", toolNames(sub.Tools.Definitions()))
	}
}

func TestTaskToolRunsSubagent(t *testing.T) {
	var calls []chatCall
	gw := taskFlowGateway(t, [][]string{
		toolCallChunks("call_1", "task", `{"description":"count the models in the catalog","guidance":"return only the number"}`),
		contentChunks("subagent counted 6 models", 10, 2),
		contentChunks("the catalog holds 6 models", 20, 1),
	}, &calls)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)
	ag.AttachTaskTool()

	ag.Run("delegate the catalog counting")
	events := collectParentTurnEvents(t, ag)

	if len(calls) != 3 {
		t.Fatalf("gateway calls = %d, want 3 (parent, subagent, parent)", len(calls))
	}
	var taskResult, subDone, parentDone *Event
	for i, e := range events {
		switch {
		case e.Kind == EventToolResult && e.Tool == taskToolName:
			taskResult = &events[i]
		case e.Kind == EventTurnDone && e.Depth == 1:
			subDone = &events[i]
		case e.Kind == EventTurnDone && e.Depth == 0:
			parentDone = &events[i]
		}
	}
	if taskResult == nil {
		t.Fatal("no tool_result event for task")
	}
	if taskResult.Result != "subagent counted 6 models" {
		t.Fatalf("task tool result = %q, want the subagent final text as the tool result", taskResult.Result)
	}
	if taskResult.Depth != 0 {
		t.Fatalf("task tool_result depth = %d, want 0 (parent flow)", taskResult.Depth)
	}
	if subDone == nil || subDone.Text != "subagent counted 6 models" {
		t.Fatalf("subagent turn_done = %+v, want the subagent final text", subDone)
	}
	if parentDone == nil || parentDone.Text != "the catalog holds 6 models" {
		t.Fatalf("parent turn_done = %+v, want the parent answering with the result", parentDone)
	}
	last := events[len(events)-1]
	if last.Kind != EventTurnDone || last.Depth != 0 {
		t.Fatalf("last event = %+v, want the parent turn_done closing the turn", last)
	}
}

func TestSubagentDoesNotSeeTaskTool(t *testing.T) {
	var calls []chatCall
	gw := taskFlowGateway(t, [][]string{
		toolCallChunks("call_1", "task", `{"description":"inspect the available tools"}`),
		contentChunks("subagent inspected the tools", 10, 2),
		contentChunks("parent confirmed the tool list", 20, 1),
	}, &calls)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)
	ag.AttachTaskTool()

	ag.Run("delegate a tool inspection")
	collectParentTurnEvents(t, ag)

	if len(calls) != 3 {
		t.Fatalf("gateway calls = %d, want 3 (parent, subagent, parent)", len(calls))
	}
	if !hasTool(calls[0].Tools, taskToolName) {
		t.Fatalf("parent request tools = %v, want task among them", toolNames(calls[0].Tools))
	}
	if hasTool(calls[1].Tools, taskToolName) {
		t.Fatalf("subagent request tools = %v, want no task (structural depth limit)", toolNames(calls[1].Tools))
	}
	if !hasTool(calls[1].Tools, "bash") {
		t.Fatalf("subagent request tools = %v, want the standard tools without task", toolNames(calls[1].Tools))
	}
	if !hasTool(calls[2].Tools, taskToolName) {
		t.Fatalf("second parent request tools = %v, want task still present after the delegation", toolNames(calls[2].Tools))
	}
}

func taskFlowGateway(t *testing.T, responses [][]string, calls *[]chatCall) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", modelsHandler(t))
	var call int
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if call >= len(responses) {
			t.Errorf("unexpected chat call %d", call)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		var body struct {
			Model    string        `json:"model"`
			Messages []llm.Message `json:"messages"`
			Tools    []llm.Tool    `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode chat request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		*calls = append(*calls, chatCall{Model: body.Model, Messages: body.Messages, Tools: body.Tools})
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
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func subagentBlockGateway(t *testing.T, blockOn int, partial string, responses [][]string, calls *[]chatCall) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", modelsHandler(t))
	var call int
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model    string        `json:"model"`
			Messages []llm.Message `json:"messages"`
			Tools    []llm.Tool    `json:"tools"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		*calls = append(*calls, chatCall{Model: body.Model, Messages: body.Messages, Tools: body.Tools})
		idx := call
		call++
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		if idx == blockOn {
			chunk := fmt.Sprintf(`{"choices":[{"delta":{"role":"assistant","content":%s}}]}`, mustJSON(partial))
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
			<-r.Context().Done()
			return
		}
		respIdx := idx
		if idx > blockOn {
			respIdx--
		}
		if respIdx >= len(responses) {
			t.Errorf("unexpected chat call %d", idx)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		for _, c := range responses[respIdx] {
			fmt.Fprintf(w, "data: %s\n\n", c)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func subagentSlowStreamGateway(t *testing.T, deltas []string, spacing time.Duration, responses [][]string, calls *[]chatCall) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", modelsHandler(t))
	var call int
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model    string        `json:"model"`
			Messages []llm.Message `json:"messages"`
			Tools    []llm.Tool    `json:"tools"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		*calls = append(*calls, chatCall{Model: body.Model, Messages: body.Messages, Tools: body.Tools})
		idx := call
		call++
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		if idx == 1 {
			for _, d := range deltas {
				chunk := fmt.Sprintf(`{"choices":[{"delta":{"role":"assistant","content":%s}}]}`, mustJSON(d))
				fmt.Fprintf(w, "data: %s\n\n", chunk)
				flusher.Flush()
				time.Sleep(spacing)
			}
			finish := fmt.Sprintf(`{"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":%d}}`, len(deltas))
			fmt.Fprintf(w, "data: %s\n\n", finish)
			flusher.Flush()
			fmt.Fprint(w, "data: [DONE]\n\n")
			flusher.Flush()
			return
		}
		respIdx := idx
		if idx > 1 {
			respIdx--
		}
		if respIdx >= len(responses) {
			t.Errorf("unexpected chat call %d", idx)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		for _, c := range responses[respIdx] {
			fmt.Fprintf(w, "data: %s\n\n", c)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func stateRoutingJev(t *testing.T, marker, markerChoice, defaultChoice string, states *[]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/systemone", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			State string `json:"state"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode jev request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		*states = append(*states, body.State)
		choice := defaultChoice
		confidence := 0.9
		if strings.Contains(body.State, marker) {
			choice = markerChoice
			confidence = 0.85
		}
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

func collectParentTurnEvents(t *testing.T, ag *Agent) []Event {
	t.Helper()
	var events []Event
	deadline := time.After(10 * time.Second)
	for {
		select {
		case e := <-ag.Events:
			events = append(events, e)
			if e.Kind == EventError {
				t.Fatalf("unexpected error event: %s", e.Text)
			}
			if e.Kind == EventTurnAborted && e.Depth == 0 {
				t.Fatalf("unexpected parent turn abort: %+v", e)
			}
			if e.Kind == EventTurnDone && e.Depth == 0 {
				return events
			}
		case <-deadline:
			t.Fatal("timed out waiting for the parent turn to complete")
		}
	}
}

func waitForDepthEvent(t *testing.T, ch chan Event, kind EventKind, depth int) Event {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case e := <-ch:
			if e.Kind == kind && e.Depth == depth {
				return e
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s at depth %d", kind, depth)
		}
	}
}

func TestSubagentRoutesIndependently(t *testing.T) {
	var calls []chatCall
	var states []string
	gw := taskFlowGateway(t, [][]string{
		toolCallChunks("call_1", "task", `{"description":"count all models in the catalog","guidance":"be quick"}`),
		contentChunks("subagent answer: 6 models", 10, 2),
		contentChunks("parent handled the delegation", 20, 1),
	}, &calls)
	jevSrv := stateRoutingJev(t, "count all models in the catalog", "deepseek-v4.1-flash", "glm-5.3", &states)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)
	ag.AttachTaskTool()

	ag.Run("delegate the heavy lifting please")
	events := collectParentTurnEvents(t, ag)

	if len(calls) != 3 {
		t.Fatalf("gateway calls = %d, want 3 (parent, subagent, parent)", len(calls))
	}
	if calls[0].Model != "glm-5.3" || calls[2].Model != "glm-5.3" {
		t.Fatalf("parent calls used models %q/%q, want glm-5.3 for the main steps", calls[0].Model, calls[2].Model)
	}
	if calls[1].Model != "deepseek-v4.1-flash" {
		t.Fatalf("subagent call used model %q, want deepseek-v4.1-flash — routing must be independent", calls[1].Model)
	}
	if len(calls[1].Messages) != 2 {
		t.Fatalf("subagent call sent %d messages, want 2 (system + user)", len(calls[1].Messages))
	}
	if calls[1].Messages[0].Role != "system" || calls[1].Messages[0].Content != subagentSystemPrompt {
		t.Fatalf("subagent system message = %+v, want the subagent prompt", calls[1].Messages[0])
	}
	if calls[1].Messages[1].Role != "user" || calls[1].Messages[1].Content != "count all models in the catalog\n\nbe quick" {
		t.Fatalf("subagent user message = %+v, want description + guidance", calls[1].Messages[1])
	}
	if len(states) != 3 {
		t.Fatalf("jev decisions = %d, want 3 (one per LLM call)", len(states))
	}
	if !strings.Contains(states[1], "count all models in the catalog") {
		t.Fatalf("subagent jev state = %q, want it to mention the subtask description", states[1])
	}
	var taskResult *Event
	for i, e := range events {
		if e.Kind == EventToolResult && e.Tool == taskToolName {
			taskResult = &events[i]
		}
	}
	if taskResult == nil {
		t.Fatal("no tool_result event for task")
	}
	if !strings.Contains(taskResult.Result, "subagent answer: 6 models") {
		t.Fatalf("task tool result = %q, want the subagent final answer", taskResult.Result)
	}
}

func TestNestedEventsCarryDepth(t *testing.T) {
	var calls []chatCall
	gw := taskFlowGateway(t, [][]string{
		toolCallChunks("call_1", "task", `{"description":"run a quick command"}`),
		toolCallChunks("sub_1", "bash", `{"command":"echo nested"}`),
		contentChunks("nested subagent done", 10, 2),
		contentChunks("parent wrapped up", 20, 1),
	}, &calls)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)
	ag.AttachTaskTool()

	ag.Run("delegate a command run")
	events := collectParentTurnEvents(t, ag)

	depths := map[int]map[EventKind]int{0: {}, 1: {}}
	for _, e := range events {
		if e.Depth != 0 && e.Depth != 1 {
			t.Fatalf("event depth = %d, want 0 or 1: %+v", e.Depth, e)
		}
		if e.Depth == 1 && e.ParentTool != taskToolName {
			t.Fatalf("depth-1 event without parent tool: %+v", e)
		}
		if e.Depth == 0 && e.ParentTool != "" {
			t.Fatalf("depth-0 event with parent tool %q: %+v", e.ParentTool, e)
		}
		depths[e.Depth][e.Kind]++
	}
	for _, kind := range []EventKind{EventRoute, EventToolStart, EventToolResult, EventTurnDone} {
		if depths[1][kind] == 0 {
			t.Fatalf("no %s event from the subagent (depth 1): %+v", kind, depths)
		}
		if depths[0][kind] == 0 {
			t.Fatalf("no %s event from the parent (depth 0): %+v", kind, depths)
		}
	}
	if depths[1][EventToolStart] != 1 || depths[1][EventToolResult] != 1 {
		t.Fatalf("subagent tool events = %d/%d, want 1/1", depths[1][EventToolStart], depths[1][EventToolResult])
	}
	if depths[0][EventToolStart] != 1 || depths[0][EventToolResult] != 1 {
		t.Fatalf("parent tool events = %d/%d, want 1/1 (the task call)", depths[0][EventToolStart], depths[0][EventToolResult])
	}
	var subDone *Event
	for i, e := range events {
		if e.Kind == EventTurnDone && e.Depth == 1 {
			subDone = &events[i]
		}
	}
	if subDone == nil || subDone.Text != "nested subagent done" {
		t.Fatalf("subagent turn_done = %+v, want the subagent final text", subDone)
	}
}

func TestSubagentStepsLimit(t *testing.T) {
	responses := [][]string{
		toolCallChunks("call_1", "task", `{"description":"keep running commands forever"}`),
	}
	for i := 0; i < subagentMaxSteps; i++ {
		responses = append(responses, toolCallChunks(fmt.Sprintf("sub_%d", i), "bash", `{"command":"echo step"}`))
	}
	responses = append(responses, contentChunks("parent recovered from the runaway subtask", 20, 1))
	var calls []chatCall
	gw := taskFlowGateway(t, responses, &calls)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)
	ag.AttachTaskTool()

	ag.Run("delegate something endless")
	events := collectParentTurnEvents(t, ag)

	if len(calls) != 2+subagentMaxSteps {
		t.Fatalf("gateway calls = %d, want %d (parent x2 + subagent limited to %d)", len(calls), 2+subagentMaxSteps, subagentMaxSteps)
	}
	subToolStarts := 0
	var taskResult *Event
	for i, e := range events {
		if e.Kind == EventToolStart && e.Depth == 1 {
			subToolStarts++
		}
		if e.Kind == EventToolResult && e.Tool == taskToolName {
			taskResult = &events[i]
		}
	}
	if subToolStarts != subagentMaxSteps {
		t.Fatalf("subagent tool starts = %d, want exactly %d before the limit", subToolStarts, subagentMaxSteps)
	}
	if taskResult == nil {
		t.Fatal("no tool_result for task")
	}
	want := fmt.Sprintf("error: subtask exceeded max steps (%d)", subagentMaxSteps)
	if taskResult.Result != want {
		t.Fatalf("task result = %q, want %q", taskResult.Result, want)
	}
	if taskResult.Depth != 0 {
		t.Fatalf("task tool_result depth = %d, want 0 (parent flow)", taskResult.Depth)
	}
}

func TestSubagentTimeout(t *testing.T) {
	var calls []chatCall
	gw := subagentBlockGateway(t, 1, "subagent is thinking", [][]string{
		toolCallChunks("call_1", "task", `{"description":"think very hard for a long time"}`),
		contentChunks("parent moved on after the timeout", 20, 1),
	}, &calls)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)
	ag.subagentTimeout = 100 * time.Millisecond
	ag.AttachTaskTool()

	ag.Run("delegate slow work")
	events := collectParentTurnEvents(t, ag)

	var taskResult *Event
	var subAbort *Event
	for i, e := range events {
		if e.Kind == EventToolResult && e.Tool == taskToolName {
			taskResult = &events[i]
		}
		if e.Kind == EventTurnAborted && e.Depth == 1 {
			subAbort = &events[i]
		}
	}
	if taskResult == nil {
		t.Fatal("no tool_result for task")
	}
	want := fmt.Sprintf("error: subtask stalled for %s without progress", 100*time.Millisecond)
	if taskResult.Result != want {
		t.Fatalf("task result = %q, want %q", taskResult.Result, want)
	}
	if subAbort == nil {
		t.Fatal("no turn_aborted event from the subagent (depth 1)")
	}
	if !strings.Contains(subAbort.Text, "subagent is thinking") {
		t.Fatalf("subagent abort text = %q, want the partial streamed before the stall", subAbort.Text)
	}
}

func TestSubagentStallWatchdogResetByProgress(t *testing.T) {
	window := 250 * time.Millisecond
	spacing := 50 * time.Millisecond
	deltas := make([]string, 13)
	for i := range deltas {
		deltas[i] = "x"
	}
	var calls []chatCall
	gw := subagentSlowStreamGateway(t, deltas, spacing, [][]string{
		toolCallChunks("call_1", "task", `{"description":"stream slowly but steadily"}`),
		contentChunks("parent got the full answer", 20, 1),
	}, &calls)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)
	ag.subagentTimeout = window
	ag.AttachTaskTool()

	start := time.Now()
	ag.Run("delegate slow steady work")
	events := collectParentTurnEvents(t, ag)
	elapsed := time.Since(start)

	if elapsed < 12*spacing {
		t.Fatalf("elapsed = %s, want the subagent to stream past the stall window %s", elapsed, window)
	}
	var taskResult *Event
	for i, e := range events {
		if e.Kind == EventTurnAborted && e.Depth == 1 {
			t.Fatalf("subagent aborted despite continuous progress: %+v", e)
		}
		if e.Kind == EventToolResult && e.Tool == taskToolName {
			taskResult = &events[i]
		}
	}
	if taskResult == nil {
		t.Fatal("no tool_result for task")
	}
	want := strings.Repeat("x", len(deltas))
	if taskResult.Result != want {
		t.Fatalf("task result = %q, want the full streamed answer %q", taskResult.Result, want)
	}
}

func TestSubagentParentCancelPropagatesAbort(t *testing.T) {
	var calls []chatCall
	gw := subagentBlockGateway(t, 0, "subagent partial work", [][]string{
		contentChunks("unused", 10, 1),
	}, &calls)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)
	ag.subagentTimeout = 10 * time.Minute

	ctx, cancel := context.WithCancel(context.Background())
	type subagentResult struct {
		text string
		err  error
	}
	done := make(chan subagentResult, 1)
	go func() {
		text, err := ag.RunSync(ctx, "long running subtask", "")
		done <- subagentResult{text, err}
	}()
	delta := waitForDepthEvent(t, ag.Events, EventDelta, 1)
	if delta.ParentTool != taskToolName {
		t.Fatalf("subagent delta parent tool = %q, want task", delta.ParentTool)
	}
	cancel()
	res := <-done
	if res.text != "" {
		t.Fatalf("subagent text = %q, want empty on parent cancel", res.text)
	}
	if !errors.Is(res.err, context.Canceled) {
		t.Fatalf("subagent error = %v, want context.Canceled (abort, not stall)", res.err)
	}
}

func TestEscCancelsSubagent(t *testing.T) {
	var calls []chatCall
	gw := subagentBlockGateway(t, 1, "subagent partial work", [][]string{
		toolCallChunks("call_1", "task", `{"description":"long running subtask"}`),
		contentChunks("follow up answer", 10, 1),
	}, &calls)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)
	ag.AttachTaskTool()

	ag.Run("delegate long work")
	delta := waitForDepthEvent(t, ag.Events, EventDelta, 1)
	if delta.ParentTool != taskToolName {
		t.Fatalf("subagent delta parent tool = %q, want task", delta.ParentTool)
	}
	ag.Cancel()
	subAbort := waitForDepthEvent(t, ag.Events, EventTurnAborted, 1)
	if !strings.Contains(subAbort.Text, "subagent partial work") {
		t.Fatalf("subagent abort text = %q, want the partial streamed before the cancel", subAbort.Text)
	}
	parentAbort := waitForDepthEvent(t, ag.Events, EventTurnAborted, 0)
	if parentAbort.Model != "glm-5.3" {
		t.Fatalf("parent abort model = %q, want glm-5.3", parentAbort.Model)
	}

	ag.Run("follow up")
	done := waitForDepthEvent(t, ag.Events, EventTurnDone, 0)
	if done.Text != "follow up answer" {
		t.Fatalf("follow up answer = %q, want %q", done.Text, "follow up answer")
	}
}

func TestSubagentWritesTranscriptWithDepth(t *testing.T) {
	var calls []chatCall
	gw := taskFlowGateway(t, [][]string{
		toolCallChunks("call_1", "task", `{"description":"inspect the catalog"}`),
		contentChunks("subagent inspected everything", 10, 2),
		contentChunks("parent summarized", 20, 1),
	}, &calls)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	sess := newSessionWriter(t)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, sess, false)
	ag.AttachTaskTool()

	ag.Run("delegate an inspection")
	collectParentTurnEvents(t, ag)

	lines := transcriptLines(t, sess)
	nested := 0
	mainFlow := 0
	snapshots := 0
	for _, ev := range lines {
		if ev["type"] == "snapshot" {
			snapshots++
			if _, ok := ev["depth"]; ok {
				t.Fatalf("snapshot carries depth: %+v", ev)
			}
			continue
		}
		depth, ok := ev["depth"]
		if ok {
			if depth != float64(1) {
				t.Fatalf("nested event depth = %v, want 1: %+v", depth, ev)
			}
			nested++
		} else {
			mainFlow++
		}
	}
	if nested == 0 {
		t.Fatal("no nested (depth 1) events in the transcript")
	}
	if mainFlow == 0 {
		t.Fatal("no main flow events in the transcript")
	}
	if snapshots != 1 {
		t.Fatalf("snapshots = %d, want exactly 1 (parent only — subagent snapshots must not pollute resume state)", snapshots)
	}
	var subAssistant map[string]any
	for _, ev := range lines {
		if ev["type"] == "assistant" && ev["depth"] == float64(1) {
			subAssistant = ev
		}
	}
	if subAssistant == nil {
		t.Fatal("no subagent assistant event with depth 1")
	}
	if subAssistant["content"] != "subagent inspected everything" {
		t.Fatalf("subagent assistant content = %v", subAssistant["content"])
	}
	var taskResult map[string]any
	for _, ev := range lines {
		if ev["type"] == "tool_result" && ev["tool"] == taskToolName {
			taskResult = ev
		}
	}
	if taskResult == nil {
		t.Fatal("no tool_result for task in transcript")
	}
	if _, ok := taskResult["depth"]; ok {
		t.Fatalf("parent tool_result carries depth: %+v", taskResult)
	}
	result, _ := taskResult["result"].(string)
	if !strings.Contains(result, "subagent inspected everything") {
		t.Fatalf("task tool_result = %q, want the subagent final answer", result)
	}
	last := lines[len(lines)-1]
	if last["type"] != "snapshot" {
		t.Fatalf("last transcript event type = %v, want the parent snapshot", last["type"])
	}
}

func TestSubagentConfirmInherited(t *testing.T) {
	var calls []chatCall
	gw := taskFlowGateway(t, [][]string{
		toolCallChunks("call_1", "task", `{"description":"run a mutating command"}`),
		toolCallChunks("sub_1", "bash", `{"command":"echo confirmed"}`),
		contentChunks("subagent finished after approval", 10, 2),
		contentChunks("parent done", 20, 1),
	}, &calls)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, true)
	ag.AttachTaskTool()

	ag.Run("delegate a mutation")
	taskConfirm := waitForDepthEvent(t, ag.Events, EventConfirm, 0)
	if taskConfirm.Tool != taskToolName {
		t.Fatalf("parent confirm tool = %q, want task (mutating tool must inherit --confirm)", taskConfirm.Tool)
	}
	taskConfirm.ApproveCh <- true

	confirm := waitForDepthEvent(t, ag.Events, EventConfirm, 1)
	if confirm.ParentTool != taskToolName {
		t.Fatalf("confirm parent tool = %q, want task", confirm.ParentTool)
	}
	if confirm.Tool != "bash" {
		t.Fatalf("confirm tool = %q, want bash", confirm.Tool)
	}
	confirm.ApproveCh <- true

	events := collectParentTurnEvents(t, ag)
	var subResult *Event
	for i, e := range events {
		if e.Kind == EventToolResult && e.Depth == 1 {
			subResult = &events[i]
		}
	}
	if subResult == nil {
		t.Fatal("no subagent tool_result after approval")
	}
	if !strings.Contains(subResult.Result, "confirmed") {
		t.Fatalf("subagent tool result = %q, want the approved command output", subResult.Result)
	}
}

func TestAttachAskUserToolDepthGuard(t *testing.T) {
	ag := newTestAgent(t, "http://gateway.unused", "http://jev.unused", nil, false)

	ag.AttachAskUserTool()
	if !hasTool(ag.Tools.Definitions(), tools.AskUserToolName) {
		t.Fatalf("main agent tools = %v, want ask_user registered at depth 0", toolNames(ag.Tools.Definitions()))
	}
	if ag.Tools.IsMutating(tools.AskUserToolName) {
		t.Fatal("ask_user must not be mutating — it must bypass the --confirm gate")
	}

	sub := ag.newSubagent("subtask", "")
	if hasTool(sub.Tools.Definitions(), tools.AskUserToolName) {
		t.Fatalf("subagent tools = %v, want no ask_user (fresh registry without it)", toolNames(sub.Tools.Definitions()))
	}
	sub.AttachAskUserTool()
	if hasTool(sub.Tools.Definitions(), tools.AskUserToolName) {
		t.Fatalf("subagent tools = %v, want the depth guard to refuse registration", toolNames(sub.Tools.Definitions()))
	}
}

func askUserArgs() string {
	return mustJSON(map[string]any{
		"questions": []map[string]any{
			{
				"question": "Which platform?",
				"header":   "Platform",
				"options": []map[string]any{
					{"label": "Claude Code", "description": "native skills"},
					{"label": "Cursor"},
				},
			},
			{
				"question": "Which versions?",
				"multiple": true,
				"options": []map[string]any{
					{"label": "v1"},
					{"label": "v2"},
				},
			},
		},
	})
}

func TestAskUserToolFlow(t *testing.T) {
	gw := mockGateway(t, [][]string{
		toolCallChunks("call_1", tools.AskUserToolName, askUserArgs()),
		contentChunks("all questions answered", 20, 1),
	})
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)
	ag.AttachAskUserTool()

	ag.Run("ask me anything")
	ev := waitForEvent(t, ag.Events, EventAskUser)
	if len(ev.Questions) != 2 {
		t.Fatalf("questions = %d, want 2", len(ev.Questions))
	}
	if ev.Questions[0].Question != "Which platform?" || ev.Questions[0].Header != "Platform" {
		t.Fatalf("questions[0] = %+v", ev.Questions[0])
	}
	if len(ev.Questions[0].Options) != 2 || ev.Questions[0].Options[0].Description != "native skills" {
		t.Fatalf("questions[0].options = %+v", ev.Questions[0].Options)
	}
	if !ev.Questions[1].Multiple {
		t.Fatalf("questions[1] = %+v, want multiple", ev.Questions[1])
	}
	if ev.AnswerCh == nil {
		t.Fatal("AnswerCh must be set on the event")
	}

	ev.AnswerCh <- []string{"Claude Code", "v1, v2"}

	events := collectParentTurnEvents(t, ag)
	var askResult *Event
	for i, e := range events {
		if e.Kind == EventToolResult && e.Tool == tools.AskUserToolName {
			askResult = &events[i]
		}
	}
	if askResult == nil {
		t.Fatal("no tool_result event for ask_user")
	}
	want := "Q: Which platform?\nA: Claude Code\n\nQ: Which versions?\nA: v1, v2"
	if askResult.Result != want {
		t.Fatalf("tool result = %q, want %q", askResult.Result, want)
	}
}

func TestAskUserToolDecline(t *testing.T) {
	gw := mockGateway(t, [][]string{
		toolCallChunks("call_1", tools.AskUserToolName, askUserArgs()),
		contentChunks("no problem", 20, 1),
	})
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)
	ag.AttachAskUserTool()

	ag.Run("ask me anything")
	ev := waitForEvent(t, ag.Events, EventAskUser)
	ev.AnswerCh <- nil

	events := collectParentTurnEvents(t, ag)
	var askResult *Event
	for i, e := range events {
		if e.Kind == EventToolResult && e.Tool == tools.AskUserToolName {
			askResult = &events[i]
		}
	}
	if askResult == nil {
		t.Fatal("no tool_result event for ask_user")
	}
	if askResult.Result != "user declined to answer" {
		t.Fatalf("tool result = %q, want the decline message", askResult.Result)
	}
}

func TestAskUserToolResultInTranscript(t *testing.T) {
	askArgs := mustJSON(map[string]any{
		"questions": []map[string]any{
			{
				"question": "Which platform?",
				"options": []map[string]any{
					{"label": "Claude Code"},
					{"label": "Cursor"},
				},
			},
		},
	})
	gw := mockGateway(t, [][]string{
		toolCallChunks("call_1", tools.AskUserToolName, askArgs),
		contentChunks("noted", 20, 1),
	})
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	sess := newSessionWriter(t)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, sess, false)
	ag.AttachAskUserTool()

	ag.Run("ask me")
	ev := waitForEvent(t, ag.Events, EventAskUser)
	ev.AnswerCh <- []string{"Cursor"}
	collectParentTurnEvents(t, ag)

	var askResult map[string]any
	for _, line := range transcriptLines(t, sess) {
		if line["type"] == "tool_result" && line["tool"] == tools.AskUserToolName {
			askResult = line
		}
	}
	if askResult == nil {
		t.Fatal("no ask_user tool_result in the transcript")
	}
	if askResult["result"] != "Q: Which platform?\nA: Cursor" {
		t.Fatalf("transcript result = %v, want the formatted answer", askResult["result"])
	}
}

func kspecStoreAt(t *testing.T, dir string) *kspec.Store {
	t.Helper()
	t.Chdir(dir)
	return kspec.Load()
}

func writeProjectRule(t *testing.T, dir, name, content string) {
	t.Helper()
	rulesDir := filepath.Join(dir, ".agents", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeProjectSkill(t *testing.T, dir, name, body string) {
	t.Helper()
	skillDir := filepath.Join(dir, ".agents", "skills", name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skill := "---\nname: " + name + "\nversion: 9.9.9\ndescription: Custom.\n---\n" + body
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skill), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSystemPromptBaseOnly(t *testing.T) {
	ag := &Agent{}
	ag.AttachKspec(kspecStoreAt(t, t.TempDir()))
	if ag.systemPrompt != baseSystemPrompt {
		t.Fatalf("system prompt = %q, want base only", ag.systemPrompt)
	}
}

func TestSystemPromptWithoutStoreStaysEmpty(t *testing.T) {
	ag := &Agent{}
	if ag.systemPrompt != "" {
		t.Fatalf("system prompt = %q, want empty without a store", ag.systemPrompt)
	}
	if got := ag.withSystemPrompt(); got != nil {
		t.Fatalf("withSystemPrompt = %+v, want nil messages", got)
	}
}

func TestSystemPromptWithProjectRules(t *testing.T) {
	dir := t.TempDir()
	writeProjectRule(t, dir, "my-rule.md", "# My Rule\n\nPROJECT RULE MARKER\n")
	ag := &Agent{}
	ag.AttachKspec(kspecStoreAt(t, dir))
	if !strings.Contains(ag.systemPrompt, baseSystemPrompt) {
		t.Fatal("system prompt missing the base prompt")
	}
	if !strings.Contains(ag.systemPrompt, "PROJECT RULE MARKER") {
		t.Fatal("system prompt missing project rules")
	}
	if !strings.Contains(ag.systemPrompt, "# Rules") {
		t.Fatal("system prompt missing the rules section header")
	}
	if strings.Contains(ag.systemPrompt, "Vitest") {
		t.Fatal("embedded rules leaked into the prompt when project rules exist")
	}
}

func TestSystemPromptSkillWithEmbeddedRulesFallback(t *testing.T) {
	ag := &Agent{}
	ag.AttachKspec(kspecStoreAt(t, t.TempDir()))
	if err := ag.ActivateSkill("kspec-version"); err != nil {
		t.Fatalf("ActivateSkill: %v", err)
	}
	if !strings.Contains(ag.systemPrompt, baseSystemPrompt) {
		t.Fatal("system prompt missing the base prompt")
	}
	if !strings.Contains(ag.systemPrompt, kspecPreamble) {
		t.Fatal("system prompt missing the kterminal preamble")
	}
	if !strings.Contains(ag.systemPrompt, "Exibe a versão atual do kspec") {
		t.Fatal("system prompt missing the resolved skill content")
	}
	if !strings.Contains(ag.systemPrompt, "Vitest") {
		t.Fatal("embedded rules missing from the prompt with an active skill in a non-bootstrapped project")
	}
}

func TestSystemPromptSkillWithoutEmbeddedRulesInBootstrappedProject(t *testing.T) {
	dir := t.TempDir()
	writeProjectSkill(t, dir, "kspec-demo", "DEMO SKILL MARKER\n")
	ag := &Agent{}
	ag.AttachKspec(kspecStoreAt(t, dir))
	if err := ag.ActivateSkill("kspec-demo"); err != nil {
		t.Fatalf("ActivateSkill: %v", err)
	}
	if !strings.Contains(ag.systemPrompt, "DEMO SKILL MARKER") {
		t.Fatal("system prompt missing the project skill content")
	}
	if strings.Contains(ag.systemPrompt, "Vitest") {
		t.Fatal("embedded rules leaked into a bootstrapped project without project rules")
	}
}

func TestSkillActivationSubstitutionAndClear(t *testing.T) {
	ag := &Agent{}
	ag.AttachKspec(kspecStoreAt(t, t.TempDir()))
	if ag.ActiveSkill() != "" {
		t.Fatalf("active skill = %q, want empty before activation", ag.ActiveSkill())
	}
	if err := ag.ActivateSkill("kspec-prd"); err != nil {
		t.Fatalf("ActivateSkill: %v", err)
	}
	if ag.ActiveSkill() != "kspec-prd" {
		t.Fatalf("active skill = %q, want kspec-prd", ag.ActiveSkill())
	}
	if !strings.Contains(ag.systemPrompt, "especialista em criar PRDs") {
		t.Fatal("system prompt missing the prd skill content")
	}
	if err := ag.ActivateSkill("kspec-qa"); err != nil {
		t.Fatalf("ActivateSkill: %v", err)
	}
	if ag.ActiveSkill() != "kspec-qa" {
		t.Fatalf("active skill = %q, want kspec-qa after substitution", ag.ActiveSkill())
	}
	if !strings.Contains(ag.systemPrompt, "TestSprite") {
		t.Fatal("system prompt missing the qa skill content")
	}
	if strings.Contains(ag.systemPrompt, "especialista em criar PRDs") {
		t.Fatal("replaced skill still present in the system prompt")
	}
	ag.Reset()
	if ag.ActiveSkill() != "" {
		t.Fatalf("active skill = %q, want empty after Reset", ag.ActiveSkill())
	}
	if ag.systemPrompt != baseSystemPrompt {
		t.Fatalf("system prompt after Reset = %q, want base only", ag.systemPrompt)
	}
}

func TestActivateSkillErrors(t *testing.T) {
	ag := &Agent{}
	if err := ag.ActivateSkill("kspec-prd"); err == nil || err.Error() != "kspec store not attached" {
		t.Fatalf("ActivateSkill without store = %v, want kspec store not attached", err)
	}
	ag.AttachKspec(kspecStoreAt(t, t.TempDir()))
	if err := ag.ActivateSkill("kspec-nope"); err == nil || !strings.Contains(err.Error(), `skill "kspec-nope" not found`) {
		t.Fatalf("ActivateSkill unknown = %v, want not-found error", err)
	}
	if ag.ActiveSkill() != "" {
		t.Fatalf("active skill = %q, want empty after failed activation", ag.ActiveSkill())
	}
	if strings.Contains(ag.systemPrompt, kspecPreamble) {
		t.Fatal("failed activation leaked the preamble into the system prompt")
	}
}

func TestWithSystemPromptMergesCompactionSummary(t *testing.T) {
	ag := &Agent{systemPrompt: "SYSTEM PROMPT"}
	ag.messages = []llm.Message{
		{Role: "system", Content: "dense summary"},
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
	}
	out := ag.withSystemPrompt()
	if len(out) != 3 {
		t.Fatalf("merged messages = %d, want 3", len(out))
	}
	if out[0].Role != "system" || out[0].Content != "SYSTEM PROMPT\n\ndense summary" {
		t.Fatalf("merged system message = %+v, want base prompt followed by the summary", out[0])
	}
	if out[1].Role != "user" || out[1].Content != "hi" {
		t.Fatalf("second message = %+v, want the original user message", out[1])
	}
	if out[2].Role != "assistant" || out[2].Content != "hello" {
		t.Fatalf("third message = %+v, want the original assistant message", out[2])
	}
}

func TestWithSystemPromptPrefixesWithoutSummary(t *testing.T) {
	ag := &Agent{systemPrompt: "SYSTEM PROMPT"}
	ag.messages = []llm.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
	}
	out := ag.withSystemPrompt()
	if len(out) != 3 {
		t.Fatalf("prefixed messages = %d, want 3", len(out))
	}
	if out[0].Role != "system" || out[0].Content != "SYSTEM PROMPT" {
		t.Fatalf("first message = %+v, want the system prompt", out[0])
	}
	if out[1].Content != "hi" || out[2].Content != "hello" {
		t.Fatalf("tail messages = %+v, want the original messages unchanged", out[1:])
	}
}

func TestWithSystemPromptEmptyReturnsMessages(t *testing.T) {
	ag := &Agent{}
	ag.messages = []llm.Message{{Role: "system", Content: subagentSystemPrompt}, {Role: "user", Content: "hi"}}
	out := ag.withSystemPrompt()
	if len(out) != 2 || out[0].Role != "system" || out[0].Content != subagentSystemPrompt {
		t.Fatalf("withSystemPrompt = %+v, want messages unchanged without an assembled prompt", out)
	}
}

func TestPromptCharsIncludesSystemPrompt(t *testing.T) {
	ag := &Agent{systemPrompt: "123456"}
	ag.messages = []llm.Message{{Role: "user", Content: "hi"}}
	want := 6 + len("user") + 2
	if got := ag.promptChars(); got != want {
		t.Fatalf("promptChars = %d, want %d including the system prompt", got, want)
	}
}

func TestSkillActivationWritesSessionEvent(t *testing.T) {
	sess := newSessionWriter(t)
	ag := &Agent{Session: sess}
	ag.AttachKspec(kspecStoreAt(t, t.TempDir()))
	if err := ag.ActivateSkill("kspec-version"); err != nil {
		t.Fatalf("ActivateSkill: %v", err)
	}
	var activation map[string]any
	for _, ev := range transcriptLines(t, sess) {
		if ev["type"] == "skill_activated" {
			activation = ev
		}
	}
	if activation == nil {
		t.Fatal("no skill_activated event in the transcript")
	}
	if activation["skill"] != "kspec-version" {
		t.Fatalf("event skill = %v, want kspec-version", activation["skill"])
	}
	if activation["source"] != kspec.SourceEmbedded {
		t.Fatalf("event source = %v, want embedded", activation["source"])
	}
}

func TestSystemPromptSentToGatewayButNotStored(t *testing.T) {
	var calls []chatCall
	gw := taskFlowGateway(t, [][]string{
		contentChunks("first answer", 10, 1),
		contentChunks("second answer", 10, 1),
	}, &calls)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)
	ag.AttachKspec(kspecStoreAt(t, t.TempDir()))
	if err := ag.ActivateSkill("kspec-version"); err != nil {
		t.Fatalf("ActivateSkill: %v", err)
	}

	ag.Run("hello")
	waitForEvent(t, ag.Events, EventTurnDone)
	ag.Run("again")
	waitForEvent(t, ag.Events, EventTurnDone)

	if len(calls) != 2 {
		t.Fatalf("gateway calls = %d, want 2", len(calls))
	}
	for i, call := range calls {
		if len(call.Messages) < 2 || call.Messages[0].Role != "system" {
			t.Fatalf("call %d messages = %+v, want system prompt followed by the conversation", i, call.Messages)
		}
		for j, m := range call.Messages[1:] {
			if m.Role == "system" {
				t.Fatalf("call %d has a second system message at position %d", i, j+1)
			}
		}
		if !strings.Contains(call.Messages[0].Content, baseSystemPrompt) {
			t.Fatalf("call %d system message missing the base prompt", i)
		}
		if !strings.Contains(call.Messages[0].Content, "Exibe a versão atual do kspec") {
			t.Fatalf("call %d system message missing the active skill — it must persist across turns", i)
		}
	}
	if calls[0].Messages[1].Role != "user" || calls[0].Messages[1].Content != "hello" {
		t.Fatalf("first call = %+v, want the system prompt followed by the user message", calls[0].Messages)
	}
	if len(calls[1].Messages) != 4 || calls[1].Messages[3].Content != "again" {
		t.Fatalf("second call = %+v, want the full conversation after the system prompt", calls[1].Messages)
	}
	for _, m := range ag.messages {
		if m.Role == "system" {
			t.Fatalf("system prompt leaked into a.messages: %+v", m)
		}
	}
}

func TestCompactionMergesSummaryIntoSingleSystem(t *testing.T) {
	var calls []chatCall
	gw := compactionGateway(t,
		[]string{"deepseek-v4.1-flash", "glm-5.2", "glm-5.3"},
		contentChunks("dense summary of earlier work", 5, 2),
		contentChunks("done", 30, 3),
		&calls,
	)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newCompactionAgent(t, gw.URL, jevSrv.URL, nil)
	ag.AttachKspec(kspecStoreAt(t, t.TempDir()))
	ag.messages = bigHistory()

	ag.Run("final question")
	events := collectEvents(t, ag)

	if len(calls) != 2 {
		t.Fatalf("gateway calls = %d, want 2 (summary + main)", len(calls))
	}
	main := calls[1]
	systemCount := 0
	for _, m := range main.Messages {
		if m.Role == "system" {
			systemCount++
		}
	}
	if systemCount != 1 {
		t.Fatalf("main call has %d system messages, want exactly 1 (summary merged into the system prompt)", systemCount)
	}
	if !strings.Contains(main.Messages[0].Content, baseSystemPrompt) {
		t.Fatal("merged system message missing the base prompt — compaction destroyed the system prompt")
	}
	if !strings.Contains(main.Messages[0].Content, "dense summary of earlier work") {
		t.Fatal("merged system message missing the compaction summary")
	}
	sawDone := false
	for _, e := range events {
		if e.Kind == EventError {
			t.Fatalf("unexpected error event: %s", e.Text)
		}
		if e.Kind == EventTurnDone {
			sawDone = true
		}
	}
	if !sawDone {
		t.Fatal("no turn_done — session did not continue after compaction")
	}
}

func TestSnapshotSkillRoundtripRestoresProjectFirst(t *testing.T) {
	dir := t.TempDir()
	sess := newSessionWriter(t)
	gw := mockGateway(t, [][]string{contentChunks("done", 10, 1)})
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, sess, false)
	ag.AttachKspec(kspecStoreAt(t, dir))
	if err := ag.ActivateSkill("kspec-version"); err != nil {
		t.Fatalf("ActivateSkill: %v", err)
	}

	ag.Run("hello")
	waitForEvent(t, ag.Events, EventTurnDone)

	snap, err := session.Load(sess.Path())
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if snap.Skill != "kspec-version" {
		t.Fatalf("snapshot skill = %q, want kspec-version", snap.Skill)
	}
	if len(snap.Messages) != 2 {
		t.Fatalf("snapshot messages = %d, want 2", len(snap.Messages))
	}

	writeProjectSkill(t, dir, "kspec-version", "PROJECT VERSION SKILL MARKER\n")

	resumed := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)
	resumed.SetMessages(snap.Messages)
	resumed.AttachKspec(kspec.Load())
	resumed.RestoreSkill(snap.Skill)
	if resumed.ActiveSkill() != "kspec-version" {
		t.Fatalf("active skill after restore = %q, want kspec-version", resumed.ActiveSkill())
	}
	if !strings.Contains(resumed.systemPrompt, "PROJECT VERSION SKILL MARKER") {
		t.Fatal("restored skill not re-resolved project-first after bootstrap between sessions")
	}
	if strings.Contains(resumed.systemPrompt, "Exibe a versão atual do kspec") {
		t.Fatal("restored skill used the embedded copy despite the project override")
	}
}

func TestRestoreSkillUnresolvableClearsWithWarning(t *testing.T) {
	ag := newTestAgent(t, "http://gateway.unused", "http://jev.unused", nil, false)
	ag.AttachKspec(kspecStoreAt(t, t.TempDir()))

	ag.RestoreSkill("kspec-gone")

	if ag.ActiveSkill() != "" {
		t.Fatalf("active skill = %q, want empty after unresolvable restore", ag.ActiveSkill())
	}
	if strings.Contains(ag.systemPrompt, kspecPreamble) {
		t.Fatal("unresolvable restore leaked the preamble into the system prompt")
	}
	ev := waitForErrorEvent(t, ag.Events)
	if !strings.Contains(ev.Text, "kspec-gone") {
		t.Fatalf("warning = %q, want the skill name in the warning", ev.Text)
	}
	if !strings.Contains(ev.Text, "could not be restored") {
		t.Fatalf("warning = %q, want a restore warning", ev.Text)
	}
}

func TestActiveSkillResolveFailureWarnsAndTurnProceeds(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, ".agents", "skills", "kspec-mismatched")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("setup skill dir: %v", err)
	}
	skill := "---\nname: kspec-phantom\nversion: 1.0\ndescription: Phantom.\n---\nPHANTOM SKILL MARKER\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skill), 0o644); err != nil {
		t.Fatalf("setup skill file: %v", err)
	}

	var calls []chatCall
	gw := taskFlowGateway(t, [][]string{contentChunks("done anyway", 10, 1)}, &calls)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	sess := newSessionWriter(t)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, sess, false)
	ag.AttachKspec(kspecStoreAt(t, dir))

	if err := ag.ActivateSkill("kspec-phantom"); err != nil {
		t.Fatalf("ActivateSkill: %v", err)
	}
	warn := waitForErrorEvent(t, ag.Events)
	if !strings.Contains(warn.Text, "kspec-phantom") {
		t.Fatalf("warning = %q, want the skill name", warn.Text)
	}
	if !strings.Contains(warn.Text, "system prompt") {
		t.Fatalf("warning = %q, want the system prompt context", warn.Text)
	}
	if ag.ActiveSkill() != "kspec-phantom" {
		t.Fatalf("active skill = %q, want it kept for the snapshot", ag.ActiveSkill())
	}
	if strings.Contains(ag.systemPrompt, "PHANTOM SKILL MARKER") {
		t.Fatal("system prompt must omit the unresolvable skill section")
	}
	if !strings.Contains(ag.systemPrompt, baseSystemPrompt) {
		t.Fatal("system prompt must keep the base section")
	}

	ag.Run("hello")
	waitForEvent(t, ag.Events, EventTurnDone)

	if len(calls) != 1 {
		t.Fatalf("gateway calls = %d, want 1", len(calls))
	}
	sys := calls[0].Messages[0]
	if sys.Role != "system" || !strings.Contains(sys.Content, baseSystemPrompt) || strings.Contains(sys.Content, "PHANTOM SKILL MARKER") {
		t.Fatalf("system message = %+v, want the base prompt without the skill section", sys)
	}
	snap, err := session.Load(sess.Path())
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if snap.Skill != "kspec-phantom" {
		t.Fatalf("snapshot skill = %q, want the active skill preserved for resume", snap.Skill)
	}
}

func waitTurnIdle(t *testing.T, ag *Agent) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for ag.turnRunning() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the turn to finish")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestSkillMutatorsRefuseDuringActiveTurn(t *testing.T) {
	var nextMessages []llm.Message
	gw := blockingGateway(t, "partial answer", &nextMessages)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)
	ag.AttachKspec(kspecStoreAt(t, t.TempDir()))
	if err := ag.ActivateSkill("kspec-version"); err != nil {
		t.Fatalf("ActivateSkill before turn: %v", err)
	}

	ag.Run("long running question")
	waitForEvent(t, ag.Events, EventDelta)

	if err := ag.ActivateSkill("kspec-qa"); err == nil || err.Error() != "agent is running a turn" {
		t.Fatalf("ActivateSkill during turn = %v, want the turn guard error", err)
	}
	if err := ag.ClearSkill(); err == nil || err.Error() != "agent is running a turn" {
		t.Fatalf("ClearSkill during turn = %v, want the turn guard error", err)
	}
	if ag.ActiveSkill() != "kspec-version" {
		t.Fatalf("active skill = %q, want kspec-version preserved by the guard", ag.ActiveSkill())
	}

	ag.Cancel()
	waitForEvent(t, ag.Events, EventTurnAborted)
	waitTurnIdle(t, ag)

	if err := ag.ActivateSkill("kspec-qa"); err != nil {
		t.Fatalf("ActivateSkill after turn: %v", err)
	}
	if err := ag.ClearSkill(); err != nil {
		t.Fatalf("ClearSkill after turn: %v", err)
	}
	if ag.ActiveSkill() != "" {
		t.Fatalf("active skill = %q, want empty after clear", ag.ActiveSkill())
	}
}

func TestRestoreSkillWritesDistinctEvent(t *testing.T) {
	sess := newSessionWriter(t)
	ag := newTestAgent(t, "http://gateway.unused", "http://jev.unused", sess, false)
	ag.AttachKspec(kspecStoreAt(t, t.TempDir()))

	ag.RestoreSkill("kspec-version")

	if ag.ActiveSkill() != "kspec-version" {
		t.Fatalf("active skill = %q, want kspec-version after restore", ag.ActiveSkill())
	}
	var restored map[string]any
	for _, ev := range transcriptLines(t, sess) {
		if ev["type"] == "skill_activated" {
			t.Fatal("restore must not write skill_activated — repeated resumes would duplicate activations")
		}
		if ev["type"] == "skill_restored" {
			restored = ev
		}
	}
	if restored == nil {
		t.Fatal("no skill_restored event in the transcript")
	}
	if restored["skill"] != "kspec-version" {
		t.Fatalf("restored event skill = %v, want kspec-version", restored["skill"])
	}
	if restored["source"] != kspec.SourceEmbedded {
		t.Fatalf("restored event source = %v, want embedded", restored["source"])
	}
}

func TestSDDFlowSkillPromptAskUserAndArtifact(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	artifactRel := filepath.Join("spec", "tasks", "012-prd-demo", "prd.md")
	writeArgs := mustJSON(map[string]any{"path": artifactRel, "content": "# PRD — Demo\n\nclarified via ask_user\n"})

	var calls []chatCall
	gw := taskFlowGateway(t, [][]string{
		toolCallChunks("call_1", tools.AskUserToolName, askUserArgs()),
		toolCallChunks("call_2", "write", writeArgs),
		contentChunks("PRD written to spec/tasks", 20, 1),
	}, &calls)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	sess := newSessionWriter(t)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, sess, false)
	ag.AttachAskUserTool()
	ag.AttachKspec(kspec.Load())
	if err := ag.ActivateSkill("kspec-prd"); err != nil {
		t.Fatalf("ActivateSkill: %v", err)
	}

	ag.Run("Begin the kspec-prd workflow now.")
	ask := waitForEvent(t, ag.Events, EventAskUser)
	ask.AnswerCh <- []string{"Claude Code", "v1, v2"}
	collectParentTurnEvents(t, ag)

	if len(calls) != 3 {
		t.Fatalf("gateway calls = %d, want 3 (ask_user, write, final answer)", len(calls))
	}
	first := calls[0].Messages
	if len(first) == 0 || first[0].Role != "system" {
		t.Fatalf("first call messages = %+v, want the system prompt leading the conversation", first)
	}
	sys := first[0].Content
	if !strings.Contains(sys, baseSystemPrompt) {
		t.Fatal("system prompt missing the base prompt")
	}
	if !strings.Contains(sys, kspecPreamble) {
		t.Fatal("system prompt missing the kterminal preamble")
	}
	if !strings.Contains(sys, "especialista em criar PRDs") {
		t.Fatal("system prompt missing the active kspec-prd skill content")
	}
	if !strings.Contains(sys, "# Rules") || !strings.Contains(sys, "Vitest") {
		t.Fatal("system prompt missing the embedded rules fallback alongside the active skill")
	}
	for i, m := range first[1:] {
		if m.Role == "system" {
			t.Fatalf("first call has a second system message at position %d", i+1)
		}
	}

	askResultSeen := false
	for _, m := range calls[1].Messages {
		if m.Role == "tool" && m.ToolCallID == "call_1" {
			askResultSeen = true
			want := "Q: Which platform?\nA: Claude Code\n\nQ: Which versions?\nA: v1, v2"
			if m.Content != want {
				t.Fatalf("ask_user tool result = %q, want %q", m.Content, want)
			}
		}
	}
	if !askResultSeen {
		t.Fatal("second call missing the ask_user tool result")
	}

	data, err := os.ReadFile(filepath.Join(project, artifactRel))
	if err != nil {
		t.Fatalf("artifact not written to spec/tasks of the current project: %v", err)
	}
	if !strings.Contains(string(data), "# PRD — Demo") {
		t.Fatalf("artifact content = %q, want the PRD body", data)
	}

	snap, err := session.Load(sess.Path())
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if snap.Skill != "kspec-prd" {
		t.Fatalf("snapshot skill = %q, want kspec-prd", snap.Skill)
	}
	var activation map[string]any
	for _, ev := range transcriptLines(t, sess) {
		if ev["type"] == "skill_activated" {
			activation = ev
		}
	}
	if activation == nil {
		t.Fatal("no skill_activated event in the transcript")
	}
	if activation["skill"] != "kspec-prd" || activation["source"] != kspec.SourceEmbedded {
		t.Fatalf("skill_activated = %+v, want kspec-prd from the embedded source", activation)
	}
}

func TestActivateModeSquad(t *testing.T) {
	squadStore := squad.Load()
	ag := New(nil, nil, nil, testCatalog(t), tools.NewRegistry(), nil, false)
	ag.AttachSquad(squadStore)

	if got := ag.Mode(); got != "sdd" {
		t.Fatalf("default mode = %q, want sdd", got)
	}

	if err := ag.ActivateMode("squad"); err != nil {
		t.Fatalf("activate squad: %v", err)
	}
	if got := ag.Mode(); got != "squad" {
		t.Fatalf("mode = %q, want squad", got)
	}
	if ag.systemPrompt == "" {
		t.Fatal("system prompt empty after activating squad")
	}
	if !strings.Contains(ag.systemPrompt, "maestro") && !strings.Contains(ag.systemPrompt, "squad") {
		t.Fatalf("system prompt does not contain maestro/squad content")
	}

	if err := ag.ActivateMode("sdd"); err != nil {
		t.Fatalf("activate sdd: %v", err)
	}
	if got := ag.Mode(); got != "sdd" {
		t.Fatalf("mode = %q, want sdd", got)
	}
}

func TestActivateModeInvalid(t *testing.T) {
	ag := New(nil, nil, nil, testCatalog(t), tools.NewRegistry(), nil, false)
	if err := ag.ActivateMode("invalid"); err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestRestoreSkillPreservesSquadPrompt(t *testing.T) {
	ag := New(nil, nil, nil, testCatalog(t), tools.NewRegistry(), nil, false)
	ag.AttachKspec(kspec.Load())
	ag.AttachSquad(squad.Load())

	if err := ag.ActivateMode("squad"); err != nil {
		t.Fatalf("activate squad: %v", err)
	}
	maestroPrompt := ag.systemPrompt
	if maestroPrompt == "" {
		t.Fatal("maestro prompt empty after activating squad")
	}

	ag.RestoreSkill("kspec-prd")
	if ag.systemPrompt != maestroPrompt {
		t.Fatalf("RestoreSkill clobbered the squad maestro prompt")
	}
	if got := ag.Mode(); got != "squad" {
		t.Fatalf("mode = %q, want squad", got)
	}

	if err := ag.ActivateMode("sdd"); err != nil {
		t.Fatalf("activate sdd: %v", err)
	}
	if ag.systemPrompt == maestroPrompt {
		t.Fatal("switching to sdd kept the maestro prompt")
	}
	if !strings.Contains(ag.systemPrompt, "kterminal") {
		t.Fatalf("sdd system prompt missing base content")
	}
}

func TestEnqueueUserMessage(t *testing.T) {
	ag := New(nil, nil, nil, testCatalog(t), tools.NewRegistry(), nil, false)

	ag.EnqueueUserMessage("first")
	ag.EnqueueUserMessage("second")

	msgs := ag.drainUserQueue()
	if len(msgs) != 2 || msgs[0] != "first" || msgs[1] != "second" {
		t.Fatalf("drained = %v, want [first second]", msgs)
	}

	if msgs := ag.drainUserQueue(); len(msgs) != 0 {
		t.Fatalf("second drain = %v, want empty", msgs)
	}
}

func TestStepLimitSquadMode(t *testing.T) {
	ag := New(nil, nil, nil, testCatalog(t), tools.NewRegistry(), nil, false)
	ag.mode = "squad"
	if got := ag.stepLimit(); got != skillMaxSteps {
		t.Fatalf("stepLimit in squad = %d, want %d", got, skillMaxSteps)
	}
}

func TestEventAgentField(t *testing.T) {
	ev := Event{Kind: EventDelta, Agent: "architect"}
	if ev.Agent != "architect" {
		t.Fatalf("agent field = %q, want architect", ev.Agent)
	}
}

func TestRunSyncPersonaSetsAgentName(t *testing.T) {
	cat := testCatalog(t)
	gw := mockGateway(t, [][]string{contentChunks("architect says hello", 10, 3)})
	jevSrv := mockJev(t, "glm-5.3", 0.9)

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

	squadStore := squad.Load()
	persona, err := squadStore.Resolve("architect")
	if err != nil {
		t.Fatalf("resolve architect: %v", err)
	}

	ctx := context.Background()
	text, err := ag.RunSyncPersona(ctx, persona, "review the code", "", "", tools.NewRegistry())
	if err != nil {
		t.Fatalf("RunSyncPersona: %v", err)
	}
	if !strings.Contains(text, "architect says hello") {
		t.Fatalf("text = %q, want architect says hello", text)
	}
}

func TestConvokePersonaWithoutKickoffFails(t *testing.T) {
	ag := New(nil, nil, nil, testCatalog(t), tools.NewRegistry(), nil, false)
	ag.AttachSquad(squad.Load())

	_, err := ag.convokePersona(context.Background(), "architect", "review the design", "")
	if err == nil {
		t.Fatal("expected error without a registered kickoff")
	}
	if !strings.Contains(err.Error(), "squad_kickoff") {
		t.Fatalf("error = %q, want an instruction to call squad_kickoff first", err.Error())
	}
}

func TestConvokePersonaUnknownRoleFails(t *testing.T) {
	ag := New(nil, nil, nil, testCatalog(t), tools.NewRegistry(), nil, false)
	ag.AttachSquad(squad.Load())
	ag.RegisterKickoff(squad.Kickoff{Roles: []string{"architect", "qa"}, MaxConvocations: 4, TokenBudget: 100000})

	_, err := ag.convokePersona(context.Background(), "frontend", "review the design", "")
	if err == nil {
		t.Fatal("expected error for a persona not in the mesa")
	}
	if !strings.Contains(err.Error(), "frontend") {
		t.Fatalf("error = %q, want it to mention the offending persona", err.Error())
	}
}

func TestConvokePersonaExceedsConvocationLimit(t *testing.T) {
	ag := New(nil, nil, nil, testCatalog(t), tools.NewRegistry(), nil, false)
	ag.AttachSquad(squad.Load())
	ag.RegisterKickoff(squad.Kickoff{Roles: []string{"architect"}, MaxConvocations: 2, TokenBudget: 100000})
	ag.turnConvocations = 2

	_, err := ag.convokePersona(context.Background(), "architect", "review", "")
	if err == nil {
		t.Fatal("expected error when the convocation limit is reached")
	}
	if !strings.Contains(err.Error(), "convocation limit") {
		t.Fatalf("error = %q, want a convocation limit signal", err.Error())
	}
}

func TestConvokePersonaExceedsTokenBudget(t *testing.T) {
	ag := New(nil, nil, nil, testCatalog(t), tools.NewRegistry(), nil, false)
	ag.AttachSquad(squad.Load())
	ag.RegisterKickoff(squad.Kickoff{Roles: []string{"architect"}, MaxConvocations: 8, TokenBudget: 1000})
	ag.turnTokens = 1000

	_, err := ag.convokePersona(context.Background(), "architect", "review", "")
	if err == nil {
		t.Fatal("expected error when the token budget is exhausted")
	}
	if !strings.Contains(err.Error(), "token budget") {
		t.Fatalf("error = %q, want a token budget signal", err.Error())
	}
}

func TestSquadKickoffToolRegistered(t *testing.T) {
	ag := New(nil, nil, nil, testCatalog(t), tools.NewRegistry(), nil, false)
	ag.AttachSquad(squad.Load())
	ag.AttachSquadKickoffTool()

	if !hasTool(ag.Tools.Definitions(), tools.SquadKickoffToolName) {
		t.Fatalf("tools = %v, want %s registered", toolNames(ag.Tools.Definitions()), tools.SquadKickoffToolName)
	}
}

func TestPersonaRegistryIsReadOnly(t *testing.T) {
	ag := New(nil, nil, nil, testCatalog(t), tools.NewRegistry(), nil, false)
	reg := ag.personaRegistry()
	if hasTool(reg.Definitions(), "write") || hasTool(reg.Definitions(), "edit") || hasTool(reg.Definitions(), "bash") {
		t.Fatalf("persona registry = %v, want read-only tools only", toolNames(reg.Definitions()))
	}
	if !hasTool(reg.Definitions(), "read") {
		t.Fatalf("persona registry = %v, want read", toolNames(reg.Definitions()))
	}
}

func TestPersonaRulesSubsetByDiscipline(t *testing.T) {
	ag := New(nil, nil, nil, testCatalog(t), tools.NewRegistry(), nil, false)
	ag.AttachKspec(kspec.Load())
	persona, err := squad.Load().Resolve("architect")
	if err != nil {
		t.Fatalf("resolve architect: %v", err)
	}
	rules := ag.personaRules(persona)
	for _, r := range rules {
		if len(r.Disciplines) == 0 {
			t.Fatalf("rule %q has no disciplines but was included in the persona subset", r.Name)
		}
	}
}

func TestPersonaPromptIncludesPersonaBody(t *testing.T) {
	ag := New(nil, nil, nil, testCatalog(t), tools.NewRegistry(), nil, false)
	persona, err := squad.Load().Resolve("backend")
	if err != nil {
		t.Fatalf("resolve backend: %v", err)
	}
	prompt := ag.buildPersonaPrompt(persona)
	if !strings.Contains(prompt, persona.Prompt) {
		t.Fatal("persona prompt does not include the persona body")
	}
}

func TestTaskToolConvenesPersonas(t *testing.T) {
	var calls []chatCall
	gw := taskFlowGateway(t, [][]string{
		toolCallChunks("call_0", tools.SquadKickoffToolName, `{"roles":["architect","backend"],"max_convocations":4,"token_budget":100000,"exit_criterion":"all agree"}`),
		toolCallChunks("call_1", taskToolName, `{"description":"review the design","persona":"architect"}`),
		contentChunks("architect contribution", 10, 2),
		toolCallChunks("call_2", taskToolName, `{"description":"assess the implementation","persona":"backend"}`),
		contentChunks("backend contribution", 10, 2),
		contentChunks("plan converged", 20, 1),
	}, &calls)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)
	ag.AttachSquad(squad.Load())
	ag.AttachTaskTool()
	ag.AttachSquadKickoffTool()

	ag.Run("design the squad feature")
	events := collectParentTurnEvents(t, ag)

	if len(calls) != 6 {
		t.Fatalf("gateway calls = %d, want 6 (kickoff, 2 convocations, parent)", len(calls))
	}
	if ag.Kickoff() == nil {
		t.Fatal("kickoff was not registered by the tool")
	}
	if ag.turnConvocations != 2 {
		t.Fatalf("turnConvocations = %d, want 2", ag.turnConvocations)
	}
	roles := map[string]bool{}
	for _, e := range events {
		if e.Depth == 1 && e.Kind == EventTurnDone {
			roles[e.Agent] = true
		}
	}
	if !roles["architect"] || !roles["backend"] {
		t.Fatalf("subagent contributions = %v, want architect and backend labelled", roles)
	}
	last := events[len(events)-1]
	if last.Kind != EventTurnDone || last.Depth != 0 {
		t.Fatalf("last event = %+v, want the parent turn_done", last)
	}
}

func TestConvocationsAccumulateIntoTokenBudget(t *testing.T) {
	var calls []chatCall
	gw := taskFlowGateway(t, [][]string{
		toolCallChunks("call_0", tools.SquadKickoffToolName, `{"roles":["architect"],"max_convocations":4,"token_budget":100000,"exit_criterion":"all agree"}`),
		toolCallChunks("call_1", taskToolName, `{"description":"review the design","persona":"architect"}`),
		contentChunks("architect contribution", 10, 2),
		contentChunks("plan converged", 20, 1),
	}, &calls)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)
	ag.AttachSquad(squad.Load())
	ag.AttachTaskTool()
	ag.AttachSquadKickoffTool()

	ag.Run("design the squad feature")
	collectParentTurnEvents(t, ag)

	// kickoff call 15 + parent task call 15 + subagent 12 + final parent call 21.
	// The subagent usage must accumulate into the parent turn budget so the
	// ceiling is measured across every LLM call in the turn.
	if ag.turnTokens != 63 {
		t.Fatalf("turnTokens = %d, want 63 (convocation usage must accumulate into the parent budget)", ag.turnTokens)
	}
}

func TestTaskToolPersonaWithoutKickoffFailsInline(t *testing.T) {
	var calls []chatCall
	gw := taskFlowGateway(t, [][]string{
		toolCallChunks("call_1", taskToolName, `{"description":"review the design","persona":"architect"}`),
		contentChunks("no kickoff, converge", 20, 1),
	}, &calls)
	jevSrv := mockJev(t, "glm-5.3", 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)
	ag.AttachSquad(squad.Load())
	ag.AttachTaskTool()

	ag.Run("try to convene without kickoff")
	events := collectParentTurnEvents(t, ag)

	var taskResult *Event
	for i, e := range events {
		if e.Kind == EventToolResult && e.Tool == taskToolName {
			taskResult = &events[i]
		}
	}
	if taskResult == nil {
		t.Fatal("no tool_result event for task")
	}
	if !strings.Contains(taskResult.Result, "squad_kickoff") {
		t.Fatalf("task result = %q, want an instruction to call squad_kickoff first", taskResult.Result)
	}
}

func TestPersonaRulesSubsetFiltersEmbeddedWithoutDisciplines(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, ".agents", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	goRule := "---\ndisciplines: [backend, architecture]\n---\n# Go\ngofmt, no comments.\n"
	if err := os.WriteFile(filepath.Join(rulesDir, "go.md"), []byte(goRule), 0o644); err != nil {
		t.Fatal(err)
	}
	pyRule := "---\ndisciplines: [data-science]\n---\n# Python\n"
	if err := os.WriteFile(filepath.Join(rulesDir, "python.md"), []byte(pyRule), 0o644); err != nil {
		t.Fatal(err)
	}
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(orig) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	ag := New(nil, nil, nil, testCatalog(t), tools.NewRegistry(), nil, false)
	ag.AttachKspec(kspec.Load())
	persona, err := squad.Load().Resolve("architect")
	if err != nil {
		t.Fatalf("resolve architect: %v", err)
	}
	rules := ag.personaRules(persona)
	if len(rules) != 1 || rules[0].Name != "go" {
		t.Fatalf("persona rules = %v, want only the go rule", ruleNames(rules))
	}
}

func ruleNames(rules []kspec.Rule) []string {
	out := make([]string, len(rules))
	for i, r := range rules {
		out[i] = r.Name
	}
	return out
}
