package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"kterminal/internal/catalog"
	"kterminal/internal/kspec"
	"kterminal/internal/llm"
	"kterminal/internal/router"
	"kterminal/internal/session"
	"kterminal/internal/squad"
	"kterminal/internal/telemetry"
	"kterminal/internal/tools"
)

const maxSteps = 50
const skillMaxSteps = 100
const subagentMaxSteps = 20
const subagentTimeout = 5 * time.Minute
const taskToolName = "task"
const subagentSystemPrompt = "You are a subagent handling a focused subtask for a parent agent. Be concise; return only the final result."
const maxStateChars = 4000
const compactionThreshold = 0.7
const preservedTailMessages = 4
const summaryModel = "deepseek-v4.1-flash"
const summaryTurnChars = 2000
const toolResultTruncateChars = 2000
const toolOutputThrottle = 50 * time.Millisecond

var errMaxSteps = errors.New("reached max tool steps")

var errTurnActive = errors.New("agent is running a turn")

type EventKind string

const (
	EventRoute       EventKind = "route"
	EventDelta       EventKind = "delta"
	EventToolStart   EventKind = "tool_start"
	EventToolOutput  EventKind = "tool_output"
	EventToolResult  EventKind = "tool_result"
	EventConfirm     EventKind = "confirm"
	EventAskUser     EventKind = "ask_user"
	EventTurnDone    EventKind = "turn_done"
	EventTurnAborted EventKind = "turn_aborted"
	EventError       EventKind = "error"
	EventCompaction  EventKind = "compaction"
	EventKickoff     EventKind = "kickoff"
)

type AskOption = tools.AskOption

type AskQuestion = tools.AskQuestion

type Event struct {
	Kind          EventKind
	Model         string
	Router        string
	Confidence    float64
	Probabilities map[string]float64
	Reason        string
	Text          string
	Tool          string
	Args          string
	Result        string
	Diff          []tools.DiffLine
	ApproveCh     chan bool
	Questions     []AskQuestion
	AnswerCh      chan []string
	SessionCost   float64
	TPS           float64
	Tokens        int64
	TokensBefore  int64
	TokensAfter   int64
	Depth         int
	ParentTool    string
	Agent         string
}

type Agent struct {
	LLM       *llm.Client
	Router    router.Router
	Fallback  router.Router
	Catalog   *catalog.Catalog
	Tools     *tools.Registry
	Session   *session.Writer
	Telemetry *telemetry.Store
	Confirm   bool

	Events            chan Event
	messages          []llm.Message
	turnMessage       llm.Message
	kspecStore        *kspec.Store
	squadStore        *squad.Store
	activeSkill       string
	systemPrompt      string
	mode              string
	lastPromptTokens  int64
	lastEstimateChars int
	candidates        []catalog.Model
	pinned            string
	sessionCost       float64
	ready             bool
	mu                sync.Mutex
	cancel            context.CancelFunc
	turnActive        bool
	now               func() time.Time
	depth             int
	subagentTimeout   time.Duration
	agentName         string
	persona           *squad.Persona
	userQueue         []string
	turnConvocations  int
	turnTokens        int64
	kickoff           *squad.Kickoff
	squadPins         map[string]string
	squadLimits       squad.Limits
}

func New(llmClient *llm.Client, r router.Router, fallback router.Router, cat *catalog.Catalog, reg *tools.Registry, sess *session.Writer, confirm bool) *Agent {
	return &Agent{
		LLM:             llmClient,
		Router:          r,
		Fallback:        fallback,
		Catalog:         cat,
		Tools:           reg,
		Session:         sess,
		Confirm:         confirm,
		Events:          make(chan Event, 512),
		subagentTimeout: subagentTimeout,
		ready:           llmClient != nil,
		now:             time.Now,
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
	a.lastPromptTokens = 0
	a.lastEstimateChars = 0
	a.turnConvocations = 0
	a.turnTokens = 0
	a.ClearSkill()
}

func (a *Agent) SetMessages(messages []llm.Message) {
	a.messages = messages
	a.candidates = nil
	a.sessionCost = 0
}

func (a *Agent) AttachSquad(s *squad.Store) {
	a.squadStore = s
}

func (a *Agent) SetSquadPins(pins map[string]string) {
	a.squadPins = pins
}

func (a *Agent) SetSquadLimits(l squad.Limits) {
	a.squadLimits = l
}

func (a *Agent) SquadStore() *squad.Store {
	return a.squadStore
}

func (a *Agent) ActivateMode(mode string) error {
	if a.turnRunning() {
		return errTurnActive
	}
	if mode != "sdd" && mode != "squad" {
		return fmt.Errorf("invalid mode %q: must be \"sdd\" or \"squad\"", mode)
	}
	a.mode = mode
	a.rebuildSystemPrompt()
	return nil
}

func (a *Agent) Mode() string {
	if a.mode == "" {
		return "sdd"
	}
	return a.mode
}

func (a *Agent) EnqueueUserMessage(text string) {
	a.mu.Lock()
	a.userQueue = append(a.userQueue, text)
	a.mu.Unlock()
}

func (a *Agent) RegisterKickoff(k squad.Kickoff) {
	a.mu.Lock()
	a.kickoff = &k
	a.mu.Unlock()
}

func (a *Agent) Kickoff() *squad.Kickoff {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.kickoff
}

func (a *Agent) drainUserQueue() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.userQueue) == 0 {
		return nil
	}
	msgs := a.userQueue
	a.userQueue = nil
	return msgs
}

func (a *Agent) emit(e Event) {
	select {
	case a.Events <- e:
	default:
	}
}

func (a *Agent) emitError(msg string) {
	a.emit(Event{Kind: EventError, Text: msg})
	a.Session.Write(session.Event{Type: "error", Error: msg, Depth: a.depth, Agent: a.agentName})
}

func (a *Agent) stepLimit() int {
	if a.depth > 0 {
		return subagentMaxSteps
	}
	if a.activeSkill != "" || a.mode == "squad" {
		return skillMaxSteps
	}
	return maxSteps
}

func (a *Agent) writeSnapshot() {
	if a.depth > 0 {
		return
	}
	a.Session.WriteSnapshot(a.messages, a.activeSkill, a.mode)
}

func (a *Agent) Run(userInput string) {
	a.RunWithAttachments(userInput, nil, nil)
}

func (a *Agent) RunWithAttachments(text string, parts []llm.ContentPart, meta []session.AttachmentMeta) {
	if !a.ready {
		a.emitError("no LLM gateway configured — run /config first")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	a.cancel = cancel
	a.turnActive = true
	a.mu.Unlock()
	a.loop(ctx, llm.Message{Role: "user", Content: text, ContentParts: parts}, meta)
}

func (a *Agent) endTurn() {
	a.mu.Lock()
	a.turnActive = false
	a.mu.Unlock()
}

func (a *Agent) turnRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.turnActive
}

func (a *Agent) Cancel() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
	}
}

func (a *Agent) RunSync(ctx context.Context, description, guidance string) (string, error) {
	sub := a.newSubagent(description, guidance)
	return a.runSubagent(ctx, sub)
}

func (a *Agent) RunSyncPersona(ctx context.Context, persona squad.Persona, description, guidance string, pinned string, reg *tools.Registry) (string, error) {
	content := description
	if guidance != "" {
		content = description + "\n\n" + guidance
	}
	sub := &Agent{
		LLM:             a.LLM,
		Router:          a.Router,
		Fallback:        a.Fallback,
		Catalog:         a.Catalog,
		Tools:           reg,
		Session:         a.Session,
		Telemetry:       a.Telemetry,
		Confirm:         a.Confirm,
		Events:          make(chan Event, 512),
		depth:           a.depth + 1,
		subagentTimeout: a.subagentTimeout,
		ready:           a.LLM != nil,
		now:             a.now,
		agentName:       persona.Name,
		persona:         &persona,
	}
	if pinned != "" {
		sub.pinned = pinned
	}
	sub.messages = []llm.Message{
		{Role: "system", Content: a.buildPersonaPrompt(persona)},
		{Role: "user", Content: content},
	}
	sub.turnMessage = llm.Message{Role: "user", Content: content}
	return a.runSubagent(ctx, sub)
}

func (a *Agent) buildPersonaPrompt(persona squad.Persona) string {
	sections := []string{persona.Prompt}
	if rules := a.personaRules(persona); len(rules) > 0 {
		sections = append(sections, rulesSection(rules))
	}
	return strings.Join(sections, "\n\n")
}

func (a *Agent) personaRules(persona squad.Persona) []kspec.Rule {
	if a.kspecStore == nil {
		return nil
	}
	disciplines := squad.PersonaDisciplines(persona)
	var out []kspec.Rule
	for _, r := range a.kspecStore.Rules() {
		if len(r.Disciplines) == 0 {
			continue
		}
		if squad.MatchesAny(disciplines, r.Disciplines) {
			out = append(out, r)
		}
	}
	return out
}

func (a *Agent) runSubagent(ctx context.Context, sub *Agent) (string, error) {
	timeout := a.subagentTimeout
	if timeout <= 0 {
		timeout = subagentTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	drained := make(chan struct{})
	go func() {
		for ev := range sub.Events {
			ev.Depth = sub.depth
			ev.ParentTool = taskToolName
			if sub.agentName != "" {
				ev.Agent = sub.agentName
			}
			a.emit(ev)
		}
		close(drained)
	}()

	if err := sub.ensureCandidates(ctx); err != nil {
		close(sub.Events)
		<-drained
		return "", err
	}
	text, err := sub.runLoop(ctx)
	a.turnTokens += sub.turnTokens
	close(sub.Events)
	<-drained
	if err != nil {
		if errors.Is(err, errMaxSteps) {
			return fmt.Sprintf("error: subtask exceeded max steps (%d)", subagentMaxSteps), nil
		}
		if ctx.Err() == context.DeadlineExceeded {
			return "error: subtask timed out after 5m", nil
		}
		return "", err
	}
	return text, nil
}

func (a *Agent) newSubagent(description, guidance string) *Agent {
	content := description
	if guidance != "" {
		content = description + "\n\n" + guidance
	}
	sub := &Agent{
		LLM:             a.LLM,
		Router:          a.Router,
		Fallback:        a.Fallback,
		Catalog:         a.Catalog,
		Tools:           tools.NewRegistry(),
		Session:         a.Session,
		Telemetry:       a.Telemetry,
		Confirm:         a.Confirm,
		Events:          make(chan Event, 512),
		depth:           a.depth + 1,
		subagentTimeout: a.subagentTimeout,
		ready:           a.LLM != nil,
		now:             a.now,
	}
	sub.messages = []llm.Message{
		{Role: "system", Content: subagentSystemPrompt},
		{Role: "user", Content: content},
	}
	sub.turnMessage = llm.Message{Role: "user", Content: content}
	return sub
}

func (a *Agent) AttachTaskTool() {
	if a.depth != 0 {
		return
	}
	a.Tools.Register(tools.Tool{
		Name:     taskToolName,
		Mutating: true,
		Schema: llm.Tool{
			Type: "function",
			Function: llm.Function{
				Name:        taskToolName,
				Description: "Delegate a focused subtask to an isolated subagent and get its final answer. In squad mode, pass persona to convene a squad persona (read-only) instead.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"description": map[string]any{"type": "string", "description": "What the subagent must accomplish"},
						"guidance":    map[string]any{"type": "string", "description": "Optional context, constraints or hints"},
						"persona":     map[string]any{"type": "string", "description": "Convene a squad persona by name (requires a registered kickoff)"},
					},
					"required": []string{"description"},
				},
			},
		},
		Execute: func(ctx context.Context, args map[string]any) (tools.Result, error) {
			description, _ := args["description"].(string)
			guidance, _ := args["guidance"].(string)
			persona, _ := args["persona"].(string)
			if persona != "" {
				text, err := a.convokePersona(ctx, persona, description, guidance)
				if err != nil {
					return tools.Result{}, err
				}
				return tools.Result{Output: text}, nil
			}
			text, err := a.RunSync(ctx, description, guidance)
			if err != nil {
				return tools.Result{}, err
			}
			return tools.Result{Output: text}, nil
		},
	})
}

func (a *Agent) convokePersona(ctx context.Context, name, description, guidance string) (string, error) {
	k := a.Kickoff()
	if k == nil {
		return "", fmt.Errorf("no squad kickoff registered: call the %s tool first", tools.SquadKickoffToolName)
	}
	if a.squadStore == nil {
		return "", fmt.Errorf("squad personas unavailable: no squad store attached")
	}
	if !a.roleInKickoff(k, name) {
		return "", fmt.Errorf("persona %q is not part of the registered mesa (%v)", name, k.Roles)
	}
	if a.turnConvocations >= k.MaxConvocations {
		return "", fmt.Errorf("mesa reached its convocation limit (%d): converge and hand off to execution", k.MaxConvocations)
	}
	if a.turnTokens >= k.TokenBudget {
		return "", fmt.Errorf("mesa reached its token budget (%d): converge and hand off to execution", k.TokenBudget)
	}
	persona, err := a.squadStore.Resolve(name)
	if err != nil {
		return "", err
	}
	a.turnConvocations++
	reg := a.personaRegistry()
	pin := a.personaPin(name)
	return a.RunSyncPersona(ctx, persona, description, guidance, pin, reg)
}

func (a *Agent) roleInKickoff(k *squad.Kickoff, name string) bool {
	for _, role := range k.Roles {
		if role == name {
			return true
		}
	}
	return false
}

func (a *Agent) personaPin(name string) string {
	if a.squadPins == nil {
		return ""
	}
	return a.squadPins[name]
}

func (a *Agent) personaRegistry() *tools.Registry {
	reg := tools.NewReadOnlyRegistry()
	reg.SetOnGoEdit(a.Tools.OnGoEdit)
	return reg
}

func (a *Agent) AttachSquadKickoffTool() {
	if a.depth != 0 {
		return
	}
	if a.squadStore == nil {
		return
	}
	limits := a.squadLimits
	if limits.MaxConvocations == 0 && limits.TokenBudget == 0 {
		limits = squad.DefaultLimits()
	}
	a.Tools.Register(tools.SquadKickoffTool(a.squadStore, limits, func(k squad.Kickoff) {
		a.RegisterKickoff(k)
		a.emit(Event{Kind: EventKickoff, Text: formatKickoffText(k)})
	}))
}

func formatKickoffText(k squad.Kickoff) string {
	return fmt.Sprintf("mesa: roles=%v max_convocations=%d token_budget=%d exit_criterion=%q",
		k.Roles, k.MaxConvocations, k.TokenBudget, k.ExitCriterion)
}

func (a *Agent) AttachAskUserTool() {
	if a.depth != 0 {
		return
	}
	a.Tools.Register(tools.AskUserTool(func(questions []AskQuestion, answerCh chan []string) {
		a.emit(Event{Kind: EventAskUser, Questions: questions, AnswerCh: answerCh})
	}))
}

func (a *Agent) loop(ctx context.Context, userMessage llm.Message, meta []session.AttachmentMeta) {
	go func() {
		defer a.endTurn()
		var partial strings.Builder
		if err := a.ensureCandidates(ctx); err != nil {
			if isAbort(ctx, err) {
				a.abortTurn("", &partial, nil)
				return
			}
			a.emitError(err.Error())
			return
		}
		if ctx.Err() != nil {
			a.abortTurn("", &partial, nil)
			return
		}
		if hasImageParts(userMessage) && len(filterVision(a.candidates)) == 0 {
			a.emitError("no vision-capable model available")
			return
		}
		a.messages = append(a.messages, userMessage)
		a.Session.Write(session.Event{Type: "user", Content: userMessage.Content, Attachments: meta, Depth: a.depth, Agent: a.agentName})
		a.turnMessage = userMessage
		_, _ = a.runLoop(ctx)
	}()
}

func (a *Agent) runLoop(ctx context.Context) (finalText string, err error) {
	userMessage := a.turnMessage
	var partial strings.Builder
	limit := a.stepLimit()
	if a.depth == 0 {
		a.turnConvocations = 0
		a.turnTokens = 0
	}
	for step := 0; step < limit; step++ {
		partial.Reset()
		if a.depth == 0 && a.mode == "squad" {
			if msgs := a.drainUserQueue(); len(msgs) > 0 {
				for _, msg := range msgs {
					a.messages = append(a.messages, llm.Message{Role: "user", Content: msg})
					a.Session.Write(session.Event{Type: "user", Content: msg, Depth: a.depth, Agent: a.agentName})
				}
			}
		}
		decision, err := a.decide(ctx, userMessage, step)
		if isAbort(ctx, err) {
			a.abortTurn(decision.Model, &partial, nil)
			return "", ctx.Err()
		}
		if err != nil {
			a.writeSnapshot()
			a.emitError(err.Error())
			return "", err
		}
		a.emit(Event{
			Kind:          EventRoute,
			Model:         decision.Model,
			Router:        decision.Router,
			Confidence:    decision.Confidence,
			Probabilities: decision.Probabilities,
			Reason:        decision.Reason,
		})
		a.Session.Write(session.Event{
			Type:          "route",
			Model:         decision.Model,
			Router:        decision.Router,
			Confidence:    decision.Confidence,
			Probabilities: decision.Probabilities,
			Reason:        decision.Reason,
			Depth:         a.depth,
			Agent:         a.agentName,
		})

		model, _ := a.Catalog.Get(decision.Model)
		if a.needsCompaction(model.ContextWindow) {
			_ = a.compact(ctx)
		}
		for a.needsCompaction(model.ContextWindow) {
			if !a.truncateOldToolResults() {
				break
			}
		}
		var deltas int
		result, err := a.LLM.ChatStream(ctx, decision.Model, a.withSystemPrompt(), a.Tools.Definitions(), func(s string) {
			deltas++
			partial.WriteString(s)
			a.emit(Event{Kind: EventDelta, Model: decision.Model, Text: s})
		})
		if err != nil {
			if isAbort(ctx, err) {
				a.abortTurn(decision.Model, &partial, nil)
				return "", ctx.Err()
			}
			a.writeSnapshot()
			a.emitError(err.Error())
			return "", err
		}
		a.lastPromptTokens = result.Usage.PromptTokens
		a.lastEstimateChars = a.promptChars()
		a.turnTokens += result.Usage.PromptTokens + result.Usage.CompletionTokens
		if ctx.Err() != nil && len(result.ToolCalls) > 0 {
			a.abortTurn(decision.Model, &partial, nil)
			return "", ctx.Err()
		}

		cost := model.Cost(result.Usage.PromptTokens, result.Usage.CompletionTokens)
		a.sessionCost += cost
		tps := 0.0
		if result.StreamSeconds > 0 && result.Usage.CompletionTokens > 0 {
			tps = float64(result.Usage.CompletionTokens) / result.StreamSeconds
		}
		if result.Usage.CompletionTokens > 0 {
			a.Telemetry.Record(decision.Model, tps)
		}

		if len(result.ToolCalls) > 0 {
			a.messages = append(a.messages, llm.Message{Role: "assistant", Content: result.Content, ToolCalls: result.ToolCalls})
			pending := result.ToolCalls
			for _, tc := range result.ToolCalls {
				a.Session.Write(session.Event{Type: "tool_call", Model: decision.Model, Tool: tc.Function.Name, Args: tc.Function.Arguments, Cost: cost, Depth: a.depth, Agent: a.agentName})
				a.emit(Event{Kind: EventToolStart, Model: decision.Model, Tool: tc.Function.Name, Args: tc.Function.Arguments})
				approved := true
				if a.Confirm && a.Tools.IsMutating(tc.Function.Name) {
					diff, _ := a.Tools.PendingDiff(ctx, tc.Function.Name, tc.Function.Arguments)
					ch := make(chan bool, 1)
					a.emit(Event{Kind: EventConfirm, Tool: tc.Function.Name, Args: tc.Function.Arguments, Diff: diff, ApproveCh: ch})
					approved = <-ch
					if ctx.Err() != nil {
						a.abortTurn(decision.Model, &partial, pending)
						return "", ctx.Err()
					}
				}
				var toolResult string
				var diff []tools.DiffLine
				if !approved {
					toolResult = "user declined this tool call"
				} else {
					res, err := a.executeTool(ctx, tc.Function.Name, tc.Function.Arguments)
					if isAbort(ctx, err) {
						a.abortTurn(decision.Model, &partial, pending)
						return "", ctx.Err()
					}
					if err != nil {
						toolResult = "error: " + err.Error()
					} else {
						toolResult = res.Output
						diff = res.Diff
					}
				}
				a.Session.Write(session.Event{Type: "tool_result", Tool: tc.Function.Name, Result: toolResult, Diff: formatDiff(diff), Depth: a.depth, Agent: a.agentName})
				a.emit(Event{Kind: EventToolResult, Tool: tc.Function.Name, Result: toolResult, Diff: diff})
				a.messages = append(a.messages, llm.Message{Role: "tool", Content: toolResult, ToolCallID: tc.ID})
				pending = pending[1:]
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
			Depth:   a.depth,
			Agent:   a.agentName,
		})
		a.writeSnapshot()
		a.emit(Event{
			Kind:        EventTurnDone,
			Model:       decision.Model,
			Text:        result.Content,
			SessionCost: a.sessionCost,
			TPS:         tps,
			Tokens:      result.Usage.PromptTokens + result.Usage.CompletionTokens,
		})
		return result.Content, nil
	}
	err = fmt.Errorf("%w (%d) without a final answer — type continue to resume", errMaxSteps, limit)
	a.writeSnapshot()
	if a.depth == 0 {
		a.emitError(err.Error())
	}
	return "", err
}

func isAbort(ctx context.Context, err error) bool {
	return errors.Is(err, context.Canceled) || ctx.Err() != nil
}

func (a *Agent) executeTool(ctx context.Context, name, argsJSON string) (tools.Result, error) {
	clock := a.now
	if clock == nil {
		clock = time.Now
	}
	collector := newToolOutputCollector(name, a.emit, clock)
	res, err := a.Tools.ExecuteStream(ctx, name, argsJSON, collector.onLine)
	collector.flush()
	return res, err
}

type toolOutputCollector struct {
	tool     string
	emit     func(Event)
	clock    func() time.Time
	lines    []string
	lastEmit time.Time
}

func newToolOutputCollector(tool string, emit func(Event), clock func() time.Time) *toolOutputCollector {
	return &toolOutputCollector{
		tool:     tool,
		emit:     emit,
		clock:    clock,
		lines:    make([]string, 0, 16),
		lastEmit: clock(),
	}
}

func (c *toolOutputCollector) onLine(line string) {
	c.lines = append(c.lines, line)
	now := c.clock()
	if now.Sub(c.lastEmit) >= toolOutputThrottle {
		c.emit(Event{Kind: EventToolOutput, Tool: c.tool, Text: strings.Join(c.lines, "\n")})
		c.lines = c.lines[:0]
		c.lastEmit = now
	}
}

func (c *toolOutputCollector) flush() {
	if len(c.lines) == 0 {
		return
	}
	c.emit(Event{Kind: EventToolOutput, Tool: c.tool, Text: strings.Join(c.lines, "\n")})
	c.lines = c.lines[:0]
}

func formatDiff(diff []tools.DiffLine) []string {
	if len(diff) == 0 {
		return nil
	}
	lines := make([]string, len(diff))
	for i, d := range diff {
		lines[i] = string(d.Kind) + d.Text
	}
	return lines
}

func (a *Agent) abortTurn(model string, partial *strings.Builder, pending []llm.ToolCall) {
	for _, tc := range pending {
		a.messages = append(a.messages, llm.Message{Role: "tool", Content: "user aborted this turn", ToolCallID: tc.ID})
	}
	a.Session.Write(session.Event{Type: "turn_aborted", Model: model, Content: partial.String(), Depth: a.depth})
	a.writeSnapshot()
	a.emit(Event{Kind: EventTurnAborted, Model: model, Text: partial.String()})
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

func (a *Agent) decide(ctx context.Context, userMessage llm.Message, step int) (router.Decision, error) {
	hasImages := hasImageParts(userMessage)
	if a.pinned != "" {
		if m, ok := a.Catalog.Get(a.pinned); ok && (!hasImages || m.Vision) {
			return router.Decision{Model: a.pinned, Confidence: 1, Router: "pin"}, nil
		}
	}
	state := a.buildState(userMessage.Content, step, hasImages)
	candidates := make([]catalog.Model, len(a.candidates))
	for i, m := range a.candidates {
		if tp, n := a.Telemetry.Get(m.Name); tp > 0 {
			m.MeasuredTPS, m.MeasuredSamples = tp, n
		}
		candidates[i] = m
	}
	if hasImages {
		candidates = filterVision(candidates)
	}
	decision, err := a.Router.Route(ctx, state, candidates)
	if err != nil {
		if a.Fallback == nil {
			return router.Decision{}, err
		}
		decision, fbErr := a.Fallback.Route(ctx, state, candidates)
		if fbErr != nil {
			return router.Decision{}, fmt.Errorf("jev router: %v; heuristic router: %v", err, fbErr)
		}
		decision.Reason = "fallback after jev error: " + err.Error()
		return decision, nil
	}
	if decision.Confidence > 0 && decision.Confidence < router.MinConfidence(len(candidates)) {
		if m, ok := a.Catalog.Get(a.Catalog.DefaultModel); ok {
			for _, c := range candidates {
				if c.Name == m.Name {
					return router.Decision{
						Model:      m.Name,
						Confidence: decision.Confidence,
						Router:     decision.Router,
						Reason:     "low confidence, using default",
					}, nil
				}
			}
		}
	}
	return decision, nil
}

func hasImageParts(m llm.Message) bool {
	for _, p := range m.ContentParts {
		if p.Type == "image_url" {
			return true
		}
	}
	return false
}

func filterVision(models []catalog.Model) []catalog.Model {
	var out []catalog.Model
	for _, m := range models {
		if m.Vision {
			out = append(out, m)
		}
	}
	return out
}

func (a *Agent) buildState(userInput string, step int, hasImages bool) string {
	var b strings.Builder
	if a.persona != nil {
		fmt.Fprintf(&b, "You are the %s persona %q in a engineering squad. Preferred model tags: %s. ", a.persona.Discipline, a.persona.Name, strings.Join(a.persona.ModelTags, ", "))
	}
	b.WriteString("Development task in a terminal-based coding agent. User request: ")
	b.WriteString(userInput)
	if step > 0 {
		fmt.Fprintf(&b, " The agent is mid-task at step %d, continuing after tool execution.", step+1)
	}
	b.WriteString(" Choose the model for the next LLM call, balancing quality, cost and speed for what this step needs.")
	if hasImages {
		b.WriteString(" This step includes image attachments.")
	}
	s := b.String()
	if len(s) > maxStateChars {
		s = s[:maxStateChars]
	}
	return s
}

func (a *Agent) estimateTokens() int64 {
	chars := a.promptChars()
	if a.lastPromptTokens > 0 {
		return a.lastPromptTokens + int64(chars-a.lastEstimateChars)/4
	}
	return int64(chars / 4)
}

func (a *Agent) promptChars() int {
	total := len(a.systemPrompt)
	for _, m := range a.messages {
		total += len(m.Role) + len(m.Content) + len(m.ToolCallID)
		for _, tc := range m.ToolCalls {
			total += len(tc.ID) + len(tc.Function.Name) + len(tc.Function.Arguments)
		}
	}
	return total
}

func (a *Agent) needsCompaction(window int) bool {
	return float64(a.estimateTokens()) > float64(window)*compactionThreshold
}

func buildSummaryPrompt(old []llm.Message) string {
	var b strings.Builder
	b.WriteString("Summarize the conversation history below for a coding agent. Write a dense summary preserving: decisions made, files touched, and the current state of the task. Keep only the information needed to continue the work.\n\n")
	for _, m := range old {
		content := m.Content
		if len(content) > summaryTurnChars {
			content = content[:summaryTurnChars]
		}
		b.WriteString(m.Role)
		b.WriteString(": ")
		b.WriteString(content)
		b.WriteString("\n\n")
	}
	return b.String()
}

func (a *Agent) modelAvailable(name string) bool {
	for _, c := range a.candidates {
		if c.Name == name {
			return true
		}
	}
	return false
}

func (a *Agent) compact(ctx context.Context) error {
	if len(a.messages) <= preservedTailMessages {
		return nil
	}
	summaryInput := buildSummaryPrompt(a.messages[:len(a.messages)-preservedTailMessages])
	model := summaryModel
	if _, ok := a.Catalog.Get(model); !ok || !a.modelAvailable(model) {
		model = a.Catalog.DefaultModel
	}
	res, err := a.LLM.ChatStream(ctx, model, []llm.Message{{Role: "user", Content: summaryInput}}, nil, nil)
	if err != nil {
		return err
	}
	if res.Content == "" {
		return errors.New("summary model returned empty summary")
	}
	before := a.estimateTokens()
	tail := append([]llm.Message{}, a.messages[len(a.messages)-preservedTailMessages:]...)
	a.messages = append([]llm.Message{{Role: "system", Content: res.Content}}, tail...)
	after := a.estimateTokens()
	a.emit(Event{Kind: EventCompaction, TokensBefore: before, TokensAfter: after})
	a.Session.Write(session.Event{Type: "compaction", TokensBefore: before, TokensAfter: after, Depth: a.depth})
	return nil
}

func (a *Agent) truncateOldToolResults() bool {
	limit := len(a.messages) - preservedTailMessages
	for i := 0; i < limit; i++ {
		m := &a.messages[i]
		if m.Role != "tool" || len(m.Content) <= toolResultTruncateChars {
			continue
		}
		truncated := m.Content[:toolResultTruncateChars] + "… (truncated)"
		if truncated == m.Content {
			continue
		}
		m.Content = truncated
		return true
	}
	return false
}
