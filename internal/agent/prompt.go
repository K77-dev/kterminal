package agent

import (
	"errors"
	"fmt"
	"strings"

	"kterminal/internal/kspec"
	"kterminal/internal/llm"
	"kterminal/internal/project"
	"kterminal/internal/session"
)

const maxRulesBytes = 12 * 1024

const baseSystemPrompt = `You are kterminal, a terminal-based coding agent.

Identity:
- You help with software engineering: reading and writing code, fixing bugs, running commands and tests, and navigating codebases.
- You operate in the current working directory; tool paths are absolute or relative to it.
- Be direct and concise. Talk to the user in the user's language; write code, identifiers and error messages in English.

Tools:
- read, glob, grep: inspect files and search the codebase.
- write, edit: create and modify files; edit replaces an exact existing string.
- bash: run shell commands (builds, tests, git); with run_in_background: true long-running processes (dev servers, watchers) return immediately and keep running — the user stops them via /bg.
- task: delegate a focused subtask to an isolated subagent and get its final answer.
- ask_user: ask the user structured questions with keyboard-navigable options.
- kspec_bootstrap: write the kspec structure (.agents/, spec/tasks/) into the current project.

Conventions:
- Make minimal, focused changes and follow the conventions of the codebase you are editing.
- For simple operational requests (run, build, test), use the project context and act directly in the fewest steps; skip build, lint and test verification for requests that do not modify anything.
- Verify your work with the project's build, lint and test commands when available.
- Never run destructive git commands (push, reset --hard, clean, branch -D) without explicit user permission.
- Do not commit unless the user explicitly asks.`

const kspecPreamble = `The skill below was authored for multiple AI coding harnesses (Claude Code, Codex CLI, Cursor). While following it, map its harness-specific concepts to kterminal:
- AskUserQuestion or request_user_input: ask through the ask_user tool.
- The agents kspec-task-runner, kspec-review-runner and kspec-qa-runner: dispatch each unit of work through the task tool, passing the runner's instructions as guidance.
- Spec artifacts (PRDs, tech specs, task lists, reviews): write them under spec/tasks/ in the current project, following the skill's conventions.

The skill content follows:`

func (a *Agent) AttachKspec(s *kspec.Store) {
	a.kspecStore = s
	a.systemPrompt = a.buildSystemPrompt()
}

func (a *Agent) ActivateSkill(name string) error {
	if a.turnRunning() {
		return errTurnActive
	}
	if a.kspecStore == nil {
		return errors.New("kspec store not attached")
	}
	if !a.skillExists(name) {
		return fmt.Errorf("skill %q not found", name)
	}
	a.applySkill(name, "skill_activated")
	return nil
}

func (a *Agent) ActiveSkill() string {
	return a.activeSkill
}

func (a *Agent) ClearSkill() error {
	if a.turnRunning() {
		return errTurnActive
	}
	if a.activeSkill == "" {
		return nil
	}
	a.activeSkill = ""
	a.rebuildSystemPrompt()
	return nil
}

func (a *Agent) RestoreSkill(name string) {
	if a.kspecStore == nil || name == "" {
		return
	}
	if _, err := a.kspecStore.Resolve(name); err != nil {
		a.ClearSkill()
		a.emitError(fmt.Sprintf("skill %q could not be restored: %v", name, err))
		return
	}
	a.applySkill(name, "skill_restored")
}

func (a *Agent) skillExists(name string) bool {
	for _, sk := range a.kspecStore.List() {
		if sk.Name == name {
			return true
		}
	}
	return false
}

func (a *Agent) applySkill(name, eventType string) {
	a.activeSkill = name
	a.rebuildSystemPrompt()
	source := ""
	if a.kspecStore != nil {
		source = a.kspecStore.Source()
	}
	a.Session.Write(session.Event{Type: eventType, Skill: name, Source: source, Depth: a.depth})
}

func maestroAddendum() string {
	return fmt.Sprintf("\n\nMesa active: you are the maestro. Delegate work through convocations instead of implementing directly, and your step limit is now %d steps.", skillMaxSteps)
}

const planAddendum = `

Plan mode active: research and plan only. Mutating tools (write, edit, bash, kspec_bootstrap) are blocked, and so are execution handoffs via task without a persona — persona convocations stay available for deliberation. Explore the codebase with read/glob/grep, ask the user questions with ask_user when you need decisions, and present a complete implementation plan. The user will review the plan and switch to build mode to execute it.`

func (a *Agent) rebuildSystemPrompt() {
	if a.mode == ModeSquad && a.squadStore != nil {
		a.systemPrompt = a.squadStore.MaestroPrompt()
	} else {
		a.systemPrompt = a.buildSystemPrompt()
		if a.maestroActive() {
			a.systemPrompt += maestroAddendum()
		}
	}
	if a.planRestricted() {
		a.systemPrompt += planAddendum
	}
}

func (a *Agent) buildSystemPrompt() string {
	if a.kspecStore == nil {
		return ""
	}
	sections := []string{baseSystemPrompt}
	if a.projectCtx != "" {
		sections = append(sections, a.projectContextSection())
	}
	if rules := a.activeRules(); len(rules) > 0 {
		sections = append(sections, rulesSection(rules))
	}
	if a.activeSkill != "" {
		content, err := a.kspecStore.Resolve(a.activeSkill)
		if err != nil {
			a.emitError(fmt.Sprintf("skill %q could not be loaded into the system prompt: %v", a.activeSkill, err))
		} else {
			sections = append(sections, kspecPreamble+"\n\n"+content)
		}
	}
	return strings.Join(sections, "\n\n")
}

func (a *Agent) projectContextSection() string {
	return "# Project context (" + a.projectCtxName + ")\n\n" + a.projectCtx
}

func (a *Agent) subagentSystemPromptWithProject() string {
	if a.projectCtx == "" {
		return subagentSystemPrompt
	}
	return subagentSystemPrompt + "\n\n" + a.projectContextSection()
}

func (a *Agent) activeRules() []kspec.Rule {
	if a.kspecStore == nil {
		return nil
	}
	var rules []kspec.Rule
	if a.kspecStore.HasProjectRules() {
		rules = a.kspecStore.Rules()
	} else if a.activeSkill != "" && a.kspecStore.Source() == kspec.SourceEmbedded {
		rules = a.kspecStore.Rules()
	}
	return filterRulesForProject(rules, a.projectFiles)
}

func filterRulesForProject(rules []kspec.Rule, files []string) []kspec.Rule {
	if len(rules) == 0 {
		return nil
	}
	out := make([]kspec.Rule, 0, len(rules))
	for _, r := range rules {
		if len(r.Paths) == 0 || project.MatchesAny(r.Paths, files) {
			out = append(out, r)
		}
	}
	return out
}

func rulesSection(rules []kspec.Rule) string {
	parts := make([]string, 0, len(rules)+1)
	parts = append(parts, "# Rules\n\nFollow the rules below in all your work.")
	budget := maxRulesBytes
	var omitted []string
	for _, r := range rules {
		if budget < len(r.Content) {
			omitted = append(omitted, r.Name)
			continue
		}
		budget -= len(r.Content)
		parts = append(parts, r.Content)
	}
	if len(omitted) > 0 {
		parts = append(parts, fmt.Sprintf("(rules omitted for size: %s)", strings.Join(omitted, ", ")))
	}
	return strings.Join(parts, "\n\n")
}

func (a *Agent) withSystemPrompt() []llm.Message {
	var out []llm.Message
	if a.systemPrompt == "" {
		out = a.messages
	} else if len(a.messages) > 0 && a.messages[0].Role == "system" {
		out = make([]llm.Message, 0, len(a.messages))
		out = append(out, llm.Message{Role: "system", Content: a.systemPrompt + "\n\n" + a.messages[0].Content})
		out = append(out, a.messages[1:]...)
	} else {
		out = make([]llm.Message, 0, len(a.messages)+1)
		out = append(out, llm.Message{Role: "system", Content: a.systemPrompt})
		out = append(out, a.messages...)
	}
	return dropOrphanedToolMessages(out)
}

func dropOrphanedToolMessages(msgs []llm.Message) []llm.Message {
	orphans := false
	for i, m := range msgs {
		if m.Role == "tool" && !respondsToToolCall(msgs[:i], m.ToolCallID) {
			orphans = true
			break
		}
	}
	if !orphans {
		return msgs
	}
	out := make([]llm.Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == "tool" && !respondsToToolCall(out, m.ToolCallID) {
			continue
		}
		out = append(out, m)
	}
	return out
}

func respondsToToolCall(prior []llm.Message, toolCallID string) bool {
	if toolCallID == "" {
		return false
	}
	j := len(prior) - 1
	for j >= 0 && prior[j].Role == "tool" {
		j--
	}
	if j < 0 || prior[j].Role != "assistant" {
		return false
	}
	for _, tc := range prior[j].ToolCalls {
		if tc.ID == toolCallID {
			return true
		}
	}
	return false
}
