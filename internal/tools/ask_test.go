package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func validAskArgs(t *testing.T) string {
	t.Helper()
	return mustArgs(t, map[string]any{
		"questions": []any{
			map[string]any{
				"question": "Which platform?",
				"options": []any{
					map[string]any{"label": "Claude Code"},
					map[string]any{"label": "Cursor"},
				},
			},
		},
	})
}

type askOutcome struct {
	res Result
	err error
}

func TestAskUserToolRegisteredNotMutating(t *testing.T) {
	reg := NewRegistry()
	reg.Register(AskUserTool(func(questions []AskQuestion, answerCh chan []string) {}))
	found := false
	for _, def := range reg.Definitions() {
		if def.Function.Name == AskUserToolName {
			found = true
		}
	}
	if !found {
		t.Fatalf("definitions missing %s", AskUserToolName)
	}
	if reg.IsMutating(AskUserToolName) {
		t.Fatal("ask_user must not be mutating — it must bypass the --confirm gate")
	}
}

func TestParseAskQuestionsValid(t *testing.T) {
	args := map[string]any{
		"questions": []any{
			map[string]any{
				"question": "Which platform?",
				"header":   "Platform",
				"multiple": true,
				"options": []any{
					map[string]any{"label": "Claude Code", "description": "native skills"},
					map[string]any{"label": "Cursor"},
				},
			},
			map[string]any{"question": "Anything else?"},
		},
	}
	questions, err := ParseAskQuestions(args)
	if err != nil {
		t.Fatalf("ParseAskQuestions: %v", err)
	}
	if len(questions) != 2 {
		t.Fatalf("questions = %d, want 2", len(questions))
	}
	q := questions[0]
	if q.Question != "Which platform?" || q.Header != "Platform" || !q.Multiple {
		t.Fatalf("questions[0] = %+v", q)
	}
	if len(q.Options) != 2 {
		t.Fatalf("questions[0].options = %d, want 2", len(q.Options))
	}
	if q.Options[0].Label != "Claude Code" || q.Options[0].Description != "native skills" {
		t.Fatalf("questions[0].options[0] = %+v", q.Options[0])
	}
	if q.Options[1].Label != "Cursor" || q.Options[1].Description != "" {
		t.Fatalf("questions[0].options[1] = %+v", q.Options[1])
	}
	second := questions[1]
	if second.Question != "Anything else?" || second.Header != "" || second.Multiple || len(second.Options) != 0 {
		t.Fatalf("questions[1] = %+v, want free-text-only question", second)
	}
}

func TestParseAskQuestionsInvalid(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"missing questions", map[string]any{}, `"questions"`},
		{"empty questions", map[string]any{"questions": []any{}}, "empty"},
		{"questions not array", map[string]any{"questions": "x"}, "array"},
		{"question not object", map[string]any{"questions": []any{"x"}}, "object"},
		{"missing question text", map[string]any{"questions": []any{map[string]any{"header": "h"}}}, `"question"`},
		{"options not array", map[string]any{"questions": []any{map[string]any{"question": "q", "options": "x"}}}, "array"},
		{"option not object", map[string]any{"questions": []any{map[string]any{"question": "q", "options": []any{"x"}}}}, "object"},
		{"option missing label", map[string]any{"questions": []any{map[string]any{"question": "q", "options": []any{map[string]any{"description": "d"}}}}}, `"label"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseAskQuestions(tc.args)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %q, want it to mention %q", err.Error(), tc.want)
			}
		})
	}
}

func TestAskUserToolRejectsInvalidArgsWithoutAsking(t *testing.T) {
	reg := NewRegistry()
	called := false
	reg.Register(AskUserTool(func(questions []AskQuestion, answerCh chan []string) {
		called = true
	}))
	_, err := reg.Execute(context.Background(), AskUserToolName, `{"questions":[]}`)
	if err == nil {
		t.Fatal("expected error for empty questions")
	}
	if called {
		t.Fatal("ask callback must not run for invalid args")
	}
}

func TestAskUserToolBlocksUntilAnswered(t *testing.T) {
	reg := NewRegistry()
	asked := make(chan chan []string, 1)
	reg.Register(AskUserTool(func(questions []AskQuestion, answerCh chan []string) {
		if len(questions) != 1 || questions[0].Question != "Which platform?" {
			t.Errorf("questions = %+v, want the parsed question", questions)
		}
		asked <- answerCh
	}))
	outcome := make(chan askOutcome, 1)
	go func() {
		res, err := reg.Execute(context.Background(), AskUserToolName, validAskArgs(t))
		outcome <- askOutcome{res, err}
	}()

	var answerCh chan []string
	select {
	case answerCh = <-asked:
	case <-time.After(2 * time.Second):
		t.Fatal("ask callback not invoked")
	}
	select {
	case <-outcome:
		t.Fatal("Execute returned before the channel was answered")
	case <-time.After(50 * time.Millisecond):
	}

	answerCh <- []string{"Claude Code"}
	select {
	case out := <-outcome:
		if out.err != nil {
			t.Fatalf("Execute: %v", out.err)
		}
		want := "Q: Which platform?\nA: Claude Code"
		if out.res.Output != want {
			t.Fatalf("Output = %q, want %q", out.res.Output, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Execute did not return after the channel was answered")
	}
}

func TestAskUserToolDeclineOnNil(t *testing.T) {
	reg := NewRegistry()
	asked := make(chan chan []string, 1)
	reg.Register(AskUserTool(func(questions []AskQuestion, answerCh chan []string) {
		asked <- answerCh
	}))
	outcome := make(chan askOutcome, 1)
	go func() {
		res, err := reg.Execute(context.Background(), AskUserToolName, validAskArgs(t))
		outcome <- askOutcome{res, err}
	}()

	var answerCh chan []string
	select {
	case answerCh = <-asked:
	case <-time.After(2 * time.Second):
		t.Fatal("ask callback not invoked")
	}
	answerCh <- nil
	select {
	case out := <-outcome:
		if out.err != nil {
			t.Fatalf("Execute: %v", out.err)
		}
		if out.res.Output != "user declined to answer" {
			t.Fatalf("Output = %q, want the decline message", out.res.Output)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Execute did not return after the decline")
	}
}

func TestAskUserToolAbortsOnContextCancel(t *testing.T) {
	reg := NewRegistry()
	asked := make(chan chan []string, 1)
	reg.Register(AskUserTool(func(questions []AskQuestion, answerCh chan []string) {
		asked <- answerCh
	}))
	ctx, cancel := context.WithCancel(context.Background())
	outcome := make(chan askOutcome, 1)
	go func() {
		res, err := reg.Execute(ctx, AskUserToolName, validAskArgs(t))
		outcome <- askOutcome{res, err}
	}()

	select {
	case <-asked:
	case <-time.After(2 * time.Second):
		t.Fatal("ask callback not invoked")
	}
	cancel()
	select {
	case out := <-outcome:
		if out.err == nil || !errors.Is(out.err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", out.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Execute did not return after context cancel")
	}
}

func TestFormatAskResult(t *testing.T) {
	questions := []AskQuestion{
		{Question: "Which platform?"},
		{Question: "Which versions?", Multiple: true},
	}
	got := FormatAskResult(questions, []string{"Claude Code", "v1, v2"})
	want := "Q: Which platform?\nA: Claude Code\n\nQ: Which versions?\nA: v1, v2"
	if got != want {
		t.Fatalf("FormatAskResult = %q, want %q", got, want)
	}
	if got := FormatAskResult(questions[:1], nil); got != "" {
		t.Fatalf("FormatAskResult with no answers = %q, want empty", got)
	}
}
