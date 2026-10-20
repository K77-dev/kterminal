package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kterminal/internal/kspec"
	"kterminal/internal/llm"
	"kterminal/internal/router"
)

func writeProjectFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestSystemPromptIncludesProjectContext(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, filepath.Join(dir, "AGENTS.md"), "# Guide\n\nRun with `go run .`\n")

	ag := &Agent{}
	ag.AttachKspec(kspecStoreAt(t, t.TempDir()))
	ag.AttachProject(dir)

	if !strings.Contains(ag.systemPrompt, baseSystemPrompt) {
		t.Fatal("system prompt missing the base prompt")
	}
	if !strings.Contains(ag.systemPrompt, "# Project context (AGENTS.md)") {
		t.Fatal("system prompt missing the project context section header")
	}
	if !strings.Contains(ag.systemPrompt, "Run with `go run .`") {
		t.Fatal("system prompt missing the AGENTS.md content")
	}
}

func TestProjectContextFallsBackToClaudeMd(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, filepath.Join(dir, "CLAUDE.md"), "CLAUDE GUIDE MARKER\n")

	ag := &Agent{}
	ag.AttachKspec(kspecStoreAt(t, t.TempDir()))
	ag.AttachProject(dir)

	if !strings.Contains(ag.systemPrompt, "# Project context (CLAUDE.md)") {
		t.Fatal("system prompt missing the CLAUDE.md context section")
	}
	if !strings.Contains(ag.systemPrompt, "CLAUDE GUIDE MARKER") {
		t.Fatal("system prompt missing the CLAUDE.md content")
	}
}

func TestProjectContextAbsentWithoutFiles(t *testing.T) {
	ag := &Agent{}
	ag.AttachKspec(kspecStoreAt(t, t.TempDir()))
	ag.AttachProject(t.TempDir())

	if strings.Contains(ag.systemPrompt, "# Project context") {
		t.Fatal("project context section present without context files")
	}
}

func TestRefreshProjectPicksUpNewContext(t *testing.T) {
	dir := t.TempDir()
	ag := &Agent{}
	ag.AttachKspec(kspecStoreAt(t, t.TempDir()))
	ag.AttachProject(dir)

	if strings.Contains(ag.systemPrompt, "# Project context") {
		t.Fatal("project context section present before the file exists")
	}
	writeProjectFile(t, filepath.Join(dir, "AGENTS.md"), "FRESH GUIDE MARKER\n")
	ag.refreshProject()
	if !strings.Contains(ag.systemPrompt, "# Project context (AGENTS.md)") {
		t.Fatal("refreshProject did not pick up the new AGENTS.md")
	}
	if !strings.Contains(ag.systemPrompt, "FRESH GUIDE MARKER") {
		t.Fatal("system prompt missing the refreshed context content")
	}
}

func TestRunRefreshesProjectContextAtTurnStart(t *testing.T) {
	dir := t.TempDir()
	var calls []chatCall
	gw := taskFlowGateway(t, [][]string{contentChunks("done", 10, 1)}, &calls)
	jevSrv := mockJev(t, "glm-5.3", 0.9)

	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)
	ag.AttachKspec(kspecStoreAt(t, t.TempDir()))
	ag.AttachProject(dir)
	writeProjectFile(t, filepath.Join(dir, "AGENTS.md"), "TURN START GUIDE MARKER\n")

	ag.Run("hello")
	waitForEvent(t, ag.Events, EventTurnDone)

	if len(calls) != 1 {
		t.Fatalf("gateway calls = %d, want 1", len(calls))
	}
	sys := calls[0].Messages[0]
	if sys.Role != "system" || !strings.Contains(sys.Content, "TURN START GUIDE MARKER") {
		t.Fatalf("system message = %+v, want the project context refreshed at turn start", sys)
	}
}

func TestActiveRulesFilteredByProjectFiles(t *testing.T) {
	dir := t.TempDir()
	writeProjectRule(t, dir, "ts-rule.md", "---\npaths:\n  - \"**/*.ts\"\n---\n\nTS RULE MARKER\n")
	writeProjectRule(t, dir, "generic-rule.md", "GENERIC RULE MARKER\n")
	writeProjectFile(t, filepath.Join(dir, "go.mod"), "module demo\n")

	ag := &Agent{}
	ag.AttachKspec(kspecStoreAt(t, dir))
	ag.AttachProject(dir)

	if strings.Contains(ag.systemPrompt, "TS RULE MARKER") {
		t.Fatal("path-scoped ts rule leaked into a Go project")
	}
	if !strings.Contains(ag.systemPrompt, "GENERIC RULE MARKER") {
		t.Fatal("generic rule missing from the prompt")
	}

	writeProjectFile(t, filepath.Join(dir, "src", "a.ts"), "export {}\n")
	ag.refreshProject()
	if !strings.Contains(ag.systemPrompt, "TS RULE MARKER") {
		t.Fatal("ts rule missing after a matching file appeared")
	}
}

func TestRulesSectionOmitsOversizedRules(t *testing.T) {
	big := strings.Repeat("x", maxRulesBytes)
	section := rulesSection([]kspec.Rule{
		{Name: "small", Content: "small rule body\n"},
		{Name: "big-a", Content: big},
		{Name: "big-b", Content: big},
	})
	if !strings.Contains(section, "small rule body") {
		t.Fatal("small rule omitted from the section")
	}
	if strings.Contains(section, strings.Repeat("x", 64)) {
		t.Fatal("oversized rule content leaked into the section")
	}
	if !strings.Contains(section, "rules omitted for size: big-a, big-b") {
		t.Fatalf("section missing the omission note: %q", section)
	}
}

func TestSubagentPromptCarriesProjectContext(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, filepath.Join(dir, "AGENTS.md"), "SUBAGENT GUIDE MARKER\n")

	ag := &Agent{}
	ag.AttachKspec(kspecStoreAt(t, t.TempDir()))
	ag.AttachProject(dir)

	sub := ag.newSubagent("do things", "with care")
	sys := sub.messages[0]
	if sys.Role != "system" || !strings.Contains(sys.Content, subagentSystemPrompt) {
		t.Fatalf("subagent system message = %+v, want the subagent prompt", sys)
	}
	if !strings.Contains(sys.Content, "SUBAGENT GUIDE MARKER") {
		t.Fatal("subagent prompt missing the project context")
	}
}

func TestSubagentPromptWithoutProject(t *testing.T) {
	ag := &Agent{}
	ag.AttachKspec(kspecStoreAt(t, t.TempDir()))
	sub := ag.newSubagent("do things", "")
	if got := sub.messages[0].Content; got != subagentSystemPrompt {
		t.Fatalf("subagent prompt = %q, want the bare subagent prompt", got)
	}
}

func TestDecideReusesTurnDecision(t *testing.T) {
	hits := &atomic.Int32{}
	jevSrv := countingJev(t, hits)
	ag := newTestAgent(t, "", jevSrv.URL, nil, false)
	ag.candidates = testCatalog(t).Models

	ag.turnDecision = &router.Decision{Model: "glm-5.3", Router: "jev", Confidence: 0.9}
	d, err := ag.decide(context.Background(), llm.Message{Role: "user", Content: "continue"}, 1)
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if d.Model != "glm-5.3" || d.Reason != "turn cache" {
		t.Fatalf("decision = %+v, want glm-5.3 from the turn cache", d)
	}
	if hits.Load() != 0 {
		t.Fatalf("jev hits = %d, want 0 with a valid cached decision", hits.Load())
	}

	d, err = ag.decide(context.Background(), llm.Message{Role: "user", Content: "continue"}, 0)
	if err != nil {
		t.Fatalf("decide step 0: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("jev hits = %d, want 1 after a fresh step-0 route", hits.Load())
	}
	if d.Reason == "turn cache" {
		t.Fatalf("step 0 decision reason = %q, want a fresh route", d.Reason)
	}
}

func TestDecideTurnCacheSkipsBlockedModel(t *testing.T) {
	hits := &atomic.Int32{}
	jevSrv := countingJev(t, hits)
	ag := newTestAgent(t, "", jevSrv.URL, nil, false)
	ag.candidates = testCatalog(t).Models

	ag.turnDecision = &router.Decision{Model: "glm-5.3", Router: "jev", Confidence: 0.9}
	ag.modelHealth.block("glm-5.3")
	d, err := ag.decide(context.Background(), llm.Message{Role: "user", Content: "continue"}, 1)
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if d.Model == "glm-5.3" {
		t.Fatal("decision kept the blocked model")
	}
	if hits.Load() != 1 {
		t.Fatalf("jev hits = %d, want 1 after re-routing around the blocked model", hits.Load())
	}
}

func TestDecideTurnCacheSkipsNonVisionModelForImages(t *testing.T) {
	hits := &atomic.Int32{}
	jevSrv := countingJev(t, hits)
	ag := newTestAgent(t, "", jevSrv.URL, nil, false)
	ag.candidates = testCatalog(t).Models

	ag.turnDecision = &router.Decision{Model: "deepseek-v4-flash", Router: "jev", Confidence: 0.9}
	msg := llm.Message{Role: "user", Content: "look", ContentParts: []llm.ContentPart{{Type: "image_url"}}}
	d, err := ag.decide(context.Background(), msg, 1)
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if d.Model == "deepseek-v4-flash" {
		t.Fatal("decision kept the non-vision cached model for an image step")
	}
	if d.Model != "glm-5.3" {
		t.Fatalf("decision model = %q, want the only vision-capable model", d.Model)
	}
}

func brokenGateway(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestEnsureCandidatesUsesFreshCache(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "models-cache.json")
	cat := testCatalog(t)
	if err := llm.SaveModelsCache(cachePath, []string{cat.Models[0].Name, "not-in-catalog"}); err != nil {
		t.Fatalf("save cache: %v", err)
	}

	ag := newTestAgent(t, brokenGateway(t).URL, "", nil, false)
	ag.SetModelsCache(cachePath, 15*time.Minute)

	if err := ag.ensureCandidates(context.Background()); err != nil {
		t.Fatalf("ensureCandidates: %v", err)
	}
	if len(ag.candidates) != 1 || ag.candidates[0].Name != cat.Models[0].Name {
		t.Fatalf("candidates = %v, want only %q from the cache", ag.candidates, cat.Models[0].Name)
	}
}

func TestEnsureCandidatesStaleCacheFallsBack(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "models-cache.json")
	cat := testCatalog(t)
	stale := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	cache := fmt.Sprintf(`{"ts":%q,"models":[%q]}`, stale, cat.Models[0].Name)
	if err := os.WriteFile(cachePath, []byte(cache), 0o600); err != nil {
		t.Fatalf("write cache: %v", err)
	}

	ag := newTestAgent(t, brokenGateway(t).URL, "", nil, false)
	ag.SetModelsCache(cachePath, 15*time.Minute)

	if err := ag.ensureCandidates(context.Background()); err != nil {
		t.Fatalf("ensureCandidates: %v (stale cache must cover a broken gateway)", err)
	}
	if len(ag.candidates) != 1 || ag.candidates[0].Name != cat.Models[0].Name {
		t.Fatalf("candidates = %v, want the stale cache entry", ag.candidates)
	}
}

func TestEnsureCandidatesFetchesAndSaves(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "models-cache.json")
	mux := http.NewServeMux()
	modelHits := &atomic.Int32{}
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		modelHits.Add(1)
		modelsHandler(t)(w, r)
	})
	gw := httptest.NewServer(mux)
	t.Cleanup(gw.Close)

	ag := newTestAgent(t, gw.URL, "", nil, false)
	ag.SetModelsCache(cachePath, 15*time.Minute)
	if err := ag.ensureCandidates(context.Background()); err != nil {
		t.Fatalf("ensureCandidates: %v", err)
	}
	if modelHits.Load() != 1 {
		t.Fatalf("model list hits = %d, want 1", modelHits.Load())
	}
	if _, _, ok := llm.LoadModelsCache(cachePath); !ok {
		t.Fatal("fetch did not persist the models cache")
	}

	second := newTestAgent(t, brokenGateway(t).URL, "", nil, false)
	second.SetModelsCache(cachePath, 15*time.Minute)
	if err := second.ensureCandidates(context.Background()); err != nil {
		t.Fatalf("second ensureCandidates: %v (persisted cache must serve the next session)", err)
	}
	if len(second.candidates) == 0 {
		t.Fatal("second agent has no candidates")
	}
}

func TestEnsureCandidatesCacheDisabled(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "models-cache.json")
	cat := testCatalog(t)
	if err := llm.SaveModelsCache(cachePath, []string{cat.Models[0].Name}); err != nil {
		t.Fatalf("save cache: %v", err)
	}
	before, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}

	mux := http.NewServeMux()
	modelHits := &atomic.Int32{}
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		modelHits.Add(1)
		modelsHandler(t)(w, r)
	})
	gw := httptest.NewServer(mux)
	t.Cleanup(gw.Close)

	ag := newTestAgent(t, gw.URL, "", nil, false)
	ag.SetModelsCache(cachePath, 0)
	if err := ag.ensureCandidates(context.Background()); err != nil {
		t.Fatalf("ensureCandidates: %v", err)
	}
	if modelHits.Load() != 1 {
		t.Fatalf("model list hits = %d, want 1 with the cache disabled", modelHits.Load())
	}
	if len(ag.candidates) == 0 {
		t.Fatal("no candidates with the cache disabled")
	}
	after, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("cache file rewritten despite the cache being disabled")
	}
}

func TestTurnDecisionFollowsFallbackModel(t *testing.T) {
	const badModel = "glm-5.3"
	var badHits atomic.Int32
	responses := [][]string{
		toolCallChunks("call_1", "bash", `{"command":"echo hi"}`),
		contentChunks("done", 20, 1),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", modelsHandler(t))
	var call int
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.Model == badModel {
			badHits.Add(1)
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":{"message":"model unavailable"}}`)
			return
		}
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
	})
	gw := httptest.NewServer(mux)
	t.Cleanup(gw.Close)

	jevSrv := mockJev(t, badModel, 0.9)
	ag := newTestAgent(t, gw.URL, jevSrv.URL, nil, false)

	ag.Run("run echo hi")
	events := collectEvents(t, ag)
	for _, e := range events {
		if e.Kind == EventError {
			t.Fatalf("unexpected error event: %s", e.Text)
		}
	}
	if badHits.Load() != 1 {
		t.Fatalf("bad model hits = %d, want 1 (404 then fallback)", badHits.Load())
	}
	if ag.turnDecision == nil || ag.turnDecision.Model == badModel {
		t.Fatalf("turn decision = %+v, want the fallback model cached for the next step", ag.turnDecision)
	}
	var lastRoute *Event
	for i, e := range events {
		if e.Kind == EventRoute {
			lastRoute = &events[i]
		}
	}
	if lastRoute == nil || lastRoute.Model == badModel {
		t.Fatalf("last route = %+v, want the fallback model", lastRoute)
	}
}
