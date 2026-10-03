package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"kterminal/internal/agent"
	"kterminal/internal/catalog"
	"kterminal/internal/config"
	"kterminal/internal/jev"
	"kterminal/internal/llm"
	"kterminal/internal/router"
	"kterminal/internal/session"
	"kterminal/internal/tools"
	"kterminal/internal/tui"
)

func main() {
	confirm := flag.Bool("confirm", false, "ask for confirmation before mutating tools (write/edit/bash)")
	doctor := flag.Bool("doctor", false, "run setup checks and exit")
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

	if *doctor {
		runDoctor(cfg, cat)
		return
	}

	sess, err := session.NewWriter()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	var llmClient *llm.Client
	if cfg.Ready() {
		llmClient = llm.New(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.SkipTLSVerify)
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

	ag := agent.New(llmClient, jevRouter, fallback, cat, tools.NewRegistry(), sess, *confirm)

	dark := lipgloss.HasDarkBackground()
	p := tea.NewProgram(tui.New(ag, cfg, cat, dark), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		sess.Close()
		os.Exit(1)
	}
	sess.Close()
}

func runDoctor(cfg *config.Config, cat *catalog.Catalog) {
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

	if !ok {
		os.Exit(1)
	}
	fmt.Println("all checks passed")
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
