package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"kterminal/internal/catalog"
	"kterminal/internal/llm"
	"kterminal/internal/router"
	"kterminal/internal/session"
	"kterminal/internal/tools"
)

const maxSteps = 25
const maxStateChars = 4000

type EventKind string

const (
	EventRoute      EventKind = "route"
	EventDelta      EventKind = "delta"
	EventToolStart  EventKind = "tool_start"
	EventToolResult EventKind = "tool_result"
	EventConfirm    EventKind = "confirm"
	EventTurnDone   EventKind = "turn_done"
	EventError      EventKind = "error"
)

type Event struct {
	Kind          EventKind
	Model         string
	Router        string
	Confidence    float64
	Probabilities map[string]float64
	Text          string
	Tool          string
	Args          string
	Result        string
	ApproveCh     chan bool
	SessionCost   float64
	TPS           float64
	Tokens        int64
}

type Agent struct {
	LLM      *llm.Client
	Router   router.Router
	Fallback router.Router
	Catalog  *catalog.Catalog
	Tools    *tools.Registry
	Session  *session.Writer
	Confirm  bool

	Events      chan Event
	messages    []llm.Message
	candidates  []catalog.Model
	pinned      string
	sessionCost float64
	ready       bool
}

func New(llmClient *llm.Client, r router.Router, fallback router.Router, cat *catalog.Catalog, reg *tools.Registry, sess *session.Writer, confirm bool) *Agent {
	return &Agent{
		LLM:      llmClient,
		Router:   r,
		Fallback: fallback,
		Catalog:  cat,
		Tools:    reg,
		Session:  sess,
		Confirm:  confirm,
		Events:   make(chan Event, 512),
		ready:    llmClient != nil,
	}
}

func (a *Agent) SetPinned(model string) { a.pinned = model }
func (a *Agent) Pinned() string         { return a.pinned }

func (a *Agent) SetLLM(c *llm.Client) {
	a.LLM = c
	a.ready = c != nil
	a.candidates = nil
}

func (a *Agent) SetRouters(r router.Router, fallback router.Router) {
	a.Router = r
	a.Fallback = fallback
}

func (a *Agent) Reset() {
	a.messages = nil
	a.sessionCost = 0
}

func (a *Agent) emit(e Event) {
	select {
	case a.Events <- e:
	default:
	}
}

func (a *Agent) emitError(msg string) {
	a.emit(Event{Kind: EventError, Text: msg})
	a.Session.Write(session.Event{Type: "error", Error: msg})
}

func (a *Agent) Run(userInput string) {
	if !a.ready {
		a.emitError("no LLM gateway configured — run /config first")
		return
	}
	go a.loop(userInput)
}

func (a *Agent) loop(userInput string) {
	ctx := context.Background()
	if err := a.ensureCandidates(ctx); err != nil {
		a.emitError(err.Error())
		return
	}
	a.messages = append(a.messages, llm.Message{Role: "user", Content: userInput})
	a.Session.Write(session.Event{Type: "user", Content: userInput})

	for step := 0; step < maxSteps; step++ {
		decision, err := a.decide(ctx, userInput, step)
		if err != nil {
			a.emitError(err.Error())
			return
		}
		a.emit(Event{
			Kind:          EventRoute,
			Model:         decision.Model,
			Router:        decision.Router,
			Confidence:    decision.Confidence,
			Probabilities: decision.Probabilities,
		})
		a.Session.Write(session.Event{
			Type:          "route",
			Model:         decision.Model,
			Router:        decision.Router,
			Confidence:    decision.Confidence,
			Probabilities: decision.Probabilities,
		})

		model, _ := a.Catalog.Get(decision.Model)
		var deltas int
		result, err := a.LLM.ChatStream(ctx, decision.Model, a.messages, a.Tools.Definitions(), func(s string) {
			deltas++
			a.emit(Event{Kind: EventDelta, Model: decision.Model, Text: s})
		})
		if err != nil {
			a.emitError(err.Error())
			return
		}

		cost := model.Cost(result.Usage.PromptTokens, result.Usage.CompletionTokens)
		a.sessionCost += cost
		tps := 0.0
		if result.StreamSeconds > 0 && result.Usage.CompletionTokens > 0 {
			tps = float64(result.Usage.CompletionTokens) / result.StreamSeconds
		}

		if len(result.ToolCalls) > 0 {
			a.messages = append(a.messages, llm.Message{Role: "assistant", Content: result.Content, ToolCalls: result.ToolCalls})
			for _, tc := range result.ToolCalls {
				a.Session.Write(session.Event{Type: "tool_call", Model: decision.Model, Tool: tc.Function.Name, Args: tc.Function.Arguments, Cost: cost})
				a.emit(Event{Kind: EventToolStart, Model: decision.Model, Tool: tc.Function.Name, Args: tc.Function.Arguments})
				approved := true
				if a.Confirm && a.Tools.IsMutating(tc.Function.Name) {
					ch := make(chan bool, 1)
					a.emit(Event{Kind: EventConfirm, Tool: tc.Function.Name, Args: tc.Function.Arguments, ApproveCh: ch})
					approved = <-ch
				}
				var toolResult string
				if !approved {
					toolResult = "user declined this tool call"
				} else {
					out, err := a.Tools.Execute(tc.Function.Name, tc.Function.Arguments)
					if err != nil {
						toolResult = "error: " + err.Error()
					} else {
						toolResult = out
					}
				}
				a.Session.Write(session.Event{Type: "tool_result", Tool: tc.Function.Name, Result: toolResult})
				a.emit(Event{Kind: EventToolResult, Tool: tc.Function.Name, Result: toolResult})
				a.messages = append(a.messages, llm.Message{Role: "tool", Content: toolResult, ToolCallID: tc.ID})
			}
			continue
		}

		a.messages = append(a.messages, llm.Message{Role: "assistant", Content: result.Content})
		a.Session.Write(session.Event{
			Type:    "assistant",
			Model:   decision.Model,
			Content: result.Content,
			Cost:    cost,
			TPS:     tps,
		})
		a.emit(Event{
			Kind:        EventTurnDone,
			Model:       decision.Model,
			Text:        result.Content,
			SessionCost: a.sessionCost,
			TPS:         tps,
			Tokens:      result.Usage.PromptTokens + result.Usage.CompletionTokens,
		})
		return
	}
	a.emitError(fmt.Sprintf("reached max tool steps (%d) without a final answer", maxSteps))
}

func (a *Agent) ensureCandidates(ctx context.Context) error {
	if a.candidates != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	remote, err := a.LLM.ListModels(ctx)
	if err != nil {
		return fmt.Errorf("listing gateway models: %w (check /config: base URL and API key)", err)
	}
	a.candidates = a.Catalog.Available(remote)
	if len(a.candidates) == 0 {
		return fmt.Errorf("gateway %s exposes %d models but none are in the kterminal catalog", a.LLM.BaseURL, len(remote))
	}
	return nil
}

func (a *Agent) decide(ctx context.Context, userInput string, step int) (router.Decision, error) {
	if a.pinned != "" {
		if _, ok := a.Catalog.Get(a.pinned); ok {
			return router.Decision{Model: a.pinned, Confidence: 1, Router: "pin"}, nil
		}
	}
	state := a.buildState(userInput, step)
	decision, err := a.Router.Route(ctx, state, a.candidates)
	if err != nil {
		if a.Fallback == nil {
			return router.Decision{}, err
		}
		decision, fbErr := a.Fallback.Route(ctx, state, a.candidates)
		if fbErr != nil {
			return router.Decision{}, fmt.Errorf("jev router: %v; heuristic router: %v", err, fbErr)
		}
		decision.Reason = "fallback after jev error: " + err.Error()
		return decision, nil
	}
	if decision.Confidence > 0 && decision.Confidence < router.MinConfidence {
		if m, ok := a.Catalog.Get(a.Catalog.DefaultModel); ok {
			available := false
			for _, c := range a.candidates {
				if c.Name == m.Name {
					available = true
					break
				}
			}
			if available {
				return router.Decision{
					Model:      m.Name,
					Confidence: decision.Confidence,
					Router:     decision.Router,
					Reason:     fmt.Sprintf("low confidence (%.2f), using default", decision.Confidence),
				}, nil
			}
		}
	}
	return decision, nil
}

func (a *Agent) buildState(userInput string, step int) string {
	var b strings.Builder
	b.WriteString("Development task in a terminal-based coding agent. User request: ")
	b.WriteString(userInput)
	if step > 0 {
		fmt.Fprintf(&b, " The agent is mid-task at step %d, continuing after tool execution.", step+1)
	}
	b.WriteString(" Choose the model for the next LLM call, balancing quality, cost and speed for what this step needs.")
	s := b.String()
	if len(s) > maxStateChars {
		s = s[:maxStateChars]
	}
	return s
}
