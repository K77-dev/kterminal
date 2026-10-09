package agent

import (
	"errors"
	"fmt"
	"strings"

	"kterminal/internal/kspec"
	"kterminal/internal/llm"
	"kterminal/internal/session"
)

const baseSystemPrompt = `You are kterminal, a terminal-based coding agent.

Identity:
- You help with software engineering: reading and writing code, fixing bugs, running commands and tests, and navigating codebases.
- You operate in the current working directory; tool paths are absolute or relative to it.
- Be direct and concise. Talk to the user in the user's language; write code, identifiers and error messages in English.

Tools:
- read, glob, grep: inspect files and search the codebase.
- write, edit: create and modify files; edit replaces an exact existing string.
- bash: run shell commands (builds, tests, git).
- task: delegate a focused subtask to an isolated subagent and get its final answer.
- ask_user: ask the user structured questions with keyboard-navigable options.
- kspec_bootstrap: write the kspec structure (.agents/, spec/tasks/) into the current project.

Conventions:
- Make minimal, focused changes and follow the conventions of the codebase you are editing.
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

func (a *Agent) rebuildSystemPrompt() {
	if a.mode == "squad" && a.squadStore != nil {
		a.systemPrompt = a.squadStore.MaestroPrompt()
		return
	}
	a.systemPrompt = a.buildSystemPrompt()
}

func (a *Agent) buildSystemPrompt() string {
	if a.kspecStore == nil {
		return ""
	}
	sections := []string{baseSystemPrompt}
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

func (a *Agent) activeRules() []kspec.Rule {
	if a.kspecStore == nil {
		return nil
	}
	if a.kspecStore.HasProjectRules() {
		return a.kspecStore.Rules()
	}
	if a.activeSkill != "" && a.kspecStore.Source() == kspec.SourceEmbedded {
		return a.kspecStore.Rules()
	}
	return nil
}

func rulesSection(rules []kspec.Rule) string {
	parts := make([]string, 0, len(rules)+1)
	parts = append(parts, "# Rules\n\nFollow the rules below in all your work.")
	for _, r := range rules {
		parts = append(parts, r.Content)
	}
	return strings.Join(parts, "\n\n")
}

func (a *Agent) withSystemPrompt() []llm.Message {
	if a.systemPrompt == "" {
		return a.messages
	}
	if len(a.messages) > 0 && a.messages[0].Role == "system" {
		out := make([]llm.Message, 0, len(a.messages))
		out = append(out, llm.Message{Role: "system", Content: a.systemPrompt + "\n\n" + a.messages[0].Content})
		return append(out, a.messages[1:]...)
	}
	out := make([]llm.Message, 0, len(a.messages)+1)
	out = append(out, llm.Message{Role: "system", Content: a.systemPrompt})
	return append(out, a.messages...)
}
