package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"kterminal/internal/agent"
	"kterminal/internal/catalog"
	"kterminal/internal/config"
	"kterminal/internal/jev"
	"kterminal/internal/kspec"
	"kterminal/internal/llm"
	"kterminal/internal/router"
	"kterminal/internal/session"
	"kterminal/internal/squad"
	"kterminal/internal/telemetry"
	"kterminal/internal/tools"
	"kterminal/internal/tui"
)

func main() {
	confirm := flag.Bool("confirm", false, "ask for confirmation before mutating tools (write/edit/bash)")
	continueFlag := flag.Bool("continue", false, "resume the most recent session (ignored when --session is set)")
	doctor := flag.Bool("doctor", false, "run setup checks and exit")
	sessionFlag := flag.String("session", "", "resume the session file at this path (takes precedence over --continue)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	cat, err := catalog.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	store := telemetry.Load(telemetryPath())
	kspecStore := kspec.Load()
	squadStore := squad.Load()

	if *doctor {
		runDoctor(cfg, cat, store, kspecStore)
		return
	}

	defer store.Close()

	sess, resumed, freshWarning, err := resolveSession(*continueFlag, *sessionFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "kterminal:", err)
		os.Exit(1)
	}
	if sess == nil {
		sess, err = session.NewWriter()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	}

	var llmClient *llm.Client
	if cfg.Ready() {
		llmClient = llm.New(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.SkipTLSVerify)
		llmClient.SetLimits(llm.Limits{
			RequestTimeout:   cfg.LLM.RequestTimeoutDuration,
			IdleTimeout:      cfg.LLM.IdleTimeoutDuration,
			FirstByteTimeout: cfg.LLM.FirstByteTimeoutDuration,
			MaxRetries:       cfg.LLM.MaxRetries,
		})
	}
	var jevRouter, fallback router.Router
	if cfg.Typesafe.APIKey != "" {
		jevRouter = router.NewJev(jev.New(cfg.Typesafe.APIKey))
	}
	fallback = &router.HeuristicRouter{Default: cat.DefaultModel}
	if jevRouter == nil {
		jevRouter = fallback
		fallback = nil
	}

	reg := tools.NewRegistry()
	reg.SetOnGoEdit(tools.GoVetHook)

	ag := agent.New(llmClient, jevRouter, fallback, cat, reg, sess, *confirm)
	ag.Telemetry = store
	ag.AttachKspec(kspecStore)
	ag.AttachSquad(squadStore)
	ag.SetSquadPins(cfg.Squad.Pins)
	ag.SetSquadLimits(squad.Limits{MaxConvocations: cfg.Squad.MaxConvocations, TokenBudget: cfg.Squad.TokenBudget})
	ag.SetSubagentTimeout(cfg.Agent.SubagentTimeoutDuration)
	ag.AttachTaskTool()
	ag.AttachSquadKickoffTool()
	ag.AttachAskUserTool()
	if len(resumed.Messages) > 0 {
		ag.SetMessages(resumed.Messages)
	}
	if err := ag.ActivateMode(cfg.Squad.DefaultMode); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if resumed.Mode != "" && resumed.Mode != cfg.Squad.DefaultMode {
		if err := ag.ActivateMode(resumed.Mode); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	}
	ag.RestoreMesa(resumed.Mesa)
	if resumed.Skill != "" {
		ag.RestoreSkill(resumed.Skill)
	}

	dark := lipgloss.HasDarkBackground()
	opts := []tui.Option{tui.WithKspec(kspecStore)}
	if len(resumed.Messages) > 0 {
		opts = append(opts, tui.WithResumed(resumed.Messages))
	}
	if freshWarning {
		opts = append(opts, tui.WithFreshWarning())
	}
	p := tea.NewProgram(tui.New(ag, cfg, cat, dark, opts...), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		sess.Close()
		store.Close()
		os.Exit(1)
	}
	sess.Close()
}

func resolveSession(continueFlag bool, sessionPath string) (*session.Writer, session.Snapshot, bool, error) {
	if sessionPath != "" {
		snap, err := session.Load(sessionPath)
		if err != nil {
			return nil, session.Snapshot{}, false, err
		}
		w, err := session.AppendWriter(sessionPath)
		if err != nil {
			return nil, session.Snapshot{}, false, err
		}
		return w, snap, false, nil
	}
	if continueFlag {
		path, snap, err := session.LoadLatest()
		if errors.Is(err, session.ErrNoSessions) {
			return nil, session.Snapshot{}, true, nil
		}
		if err != nil {
			return nil, session.Snapshot{}, false, err
		}
		w, err := session.AppendWriter(path)
		if err != nil {
			return nil, session.Snapshot{}, false, err
		}
		return w, snap, false, nil
	}
	return nil, session.Snapshot{}, false, nil
}

func telemetryPath() string {
	if base := os.Getenv("XDG_DATA_HOME"); base != "" {
		return filepath.Join(base, "kterminal", "telemetry.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".kterminal", "telemetry.json")
	}
	return filepath.Join(home, ".local", "share", "kterminal", "telemetry.json")
}

func runDoctor(cfg *config.Config, cat *catalog.Catalog, store *telemetry.Store, kspecStore *kspec.Store) {
	ok := true
	fmt.Println("kterminal doctor")
	fmt.Println()

	fmt.Printf("config file: %s\n", config.Path())
	if config.Exists() {
		fmt.Printf("  exists: yes\n")
	} else {
		fmt.Printf("  exists: no (run kterminal and use /config)\n")
	}
	fmt.Printf("  llm base url: %s\n", display(cfg.LLM.BaseURL))
	fmt.Printf("  llm api key: %s\n", display(cfg.LLM.APIKey))
	fmt.Printf("  skip tls verify: %v\n", cfg.LLM.SkipTLSVerify)
	fmt.Print(doctorLimitsLines(cfg))
	fmt.Printf("  typesafe api key: %s\n", display(cfg.Typesafe.APIKey))
	fmt.Println()

	fmt.Printf("catalog: %d models, default %s\n", len(cat.Models), cat.DefaultModel)
	for _, m := range cat.Models {
		fmt.Printf("  %s — $%.2f/M in, $%.2f/M out, ~%.0f tok/s, %dk ctx\n",
			m.Name, m.InputPricePerM, m.OutputPricePerM, m.TPSEstimate, m.ContextWindow/1000)
	}
	fmt.Println()

	if !cfg.Ready() {
		fmt.Println("gateway: NOT CONFIGURED — set llm base_url and api_key")
		ok = false
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		client := llm.New(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.SkipTLSVerify)
		client.SetLimits(llm.Limits{
			RequestTimeout:   cfg.LLM.RequestTimeoutDuration,
			IdleTimeout:      cfg.LLM.IdleTimeoutDuration,
			FirstByteTimeout: cfg.LLM.FirstByteTimeoutDuration,
			MaxRetries:       cfg.LLM.MaxRetries,
		})
		remote, err := client.ListModels(ctx)
		cancel()
		if err != nil {
			fmt.Printf("gateway %s: UNREACHABLE — %v\n", cfg.LLM.BaseURL, err)
			ok = false
		} else {
			avail := cat.Available(remote)
			fmt.Printf("gateway %s: reachable, %d models exposed\n", cfg.LLM.BaseURL, len(remote))
			if len(avail) == 0 {
				fmt.Println("  WARNING: no gateway model matches the catalog names")
				ok = false
			} else {
				for _, m := range avail {
					fmt.Printf("  routable: %s\n", m.Name)
				}
			}
		}
	}
	fmt.Println()

	if cfg.Typesafe.APIKey == "" {
		fmt.Println("jev: NOT CONFIGURED — routing falls back to heuristics")
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		client := jev.New(cfg.Typesafe.APIKey)
		_, conf, _, err := client.Decide(ctx, "Health check for a terminal coding assistant.", "Which team owns this?", map[string]string{
			"dev":   "Development topics",
			"sales": "Pricing questions",
		})
		cancel()
		if err != nil {
			fmt.Printf("jev: UNREACHABLE — %v\n", err)
			ok = false
		} else {
			fmt.Printf("jev: reachable (answered with confidence %.2f)\n", conf)
		}
	}

	fmt.Println()
	fmt.Print(doctorKspecSection(kspecStore))

	fmt.Println()
	fmt.Print(doctorTelemetryTable(store, cat))

	if !ok {
		os.Exit(1)
	}
	fmt.Println("all checks passed")
}

func doctorLimitsLines(cfg *config.Config) string {
	var b strings.Builder
	fmt.Fprintf(&b, "  llm timeouts: first byte %s, idle %s, total %s, retries %d\n",
		cfg.LLM.FirstByteTimeoutDuration, cfg.LLM.IdleTimeoutDuration, cfg.LLM.RequestTimeoutDuration, cfg.LLM.MaxRetries)
	fmt.Fprintf(&b, "  agent: subagent stall %s\n", cfg.Agent.SubagentTimeoutDuration)
	return b.String()
}

func doctorKspecSection(store *kspec.Store) string {
	var b strings.Builder
	source := store.Source()
	if v := store.Version(); v != "" {
		fmt.Fprintf(&b, "kspec: v%s (%s)\n", v, source)
	} else {
		fmt.Fprintf(&b, "kspec: version unknown (%s)\n", source)
	}
	for _, inv := range store.Invalid() {
		fmt.Fprintf(&b, "  WARNING: invalid skill %s: %v\n", inv.Dir, inv.Err)
	}
	return b.String()
}

func doctorTelemetryTable(store *telemetry.Store, cat *catalog.Catalog) string {
	var b strings.Builder
	fmt.Fprintln(&b, "telemetry: model | measured (mean tok/s, samples) | estimate (tok/s)")
	for _, m := range cat.Models {
		mean, samples := store.GetMean(m.Name)
		measured := "no samples"
		if samples > 0 {
			measured = fmt.Sprintf("%.1f tok/s, %d samples", mean, samples)
		}
		fmt.Fprintf(&b, "  %s | %s | %.0f tok/s\n", m.Name, measured, m.TPSEstimate)
	}
	return b.String()
}

func display(s string) string {
	if s == "" {
		return "(not set)"
	}
	if len(s) <= 8 {
		return s
	}
	return s[:4] + "…" + s[len(s)-4:]
}
