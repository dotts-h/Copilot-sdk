// Command my-orchestra is a cost-aware coding TUI that fuses the GitHub Copilot
// CLI and Claude CLI experiences. It drives the GitHub Copilot SDK (through a
// Node sidecar), assembles context with the my-ctx forge, and meters AI-Credit
// spend live.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dotts-h/copilot-sdk/internal/config"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
	"github.com/dotts-h/copilot-sdk/internal/tui"
	"github.com/dotts-h/copilot-sdk/internal/web"
)

// version is overridden at build time via -ldflags.
var version = "dev"

func main() {
	var (
		showVersion bool
		configDir   string
		seed        bool
		resume      bool
		webUI       bool
		webAddr     string
		webDemo     bool
	)
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.StringVar(&configDir, "config-dir", defaultConfigDir(), "configuration directory")
	flag.BoolVar(&seed, "seed", false, "write a starter forge + config to the config dir and exit")
	flag.BoolVar(&resume, "resume", false, "resume the most recent session on launch")
	flag.BoolVar(&webUI, "web", false, "serve the htmx web UI instead of the TUI")
	flag.StringVar(&webAddr, "web-addr", "127.0.0.1:8765", "address for the web UI")
	flag.BoolVar(&webDemo, "web-demo", false, "drive the web UI with a scripted mock (no Copilot runtime)")
	flag.Parse()

	if showVersion {
		fmt.Printf("my-orchestra %s\n", version)
		return
	}

	if webUI {
		if err := runWeb(configDir, webAddr, webDemo); err != nil {
			fmt.Fprintln(os.Stderr, "my-orchestra: "+err.Error())
			os.Exit(1)
		}
		return
	}

	if err := run(configDir, seed, resume); err != nil {
		fmt.Fprintln(os.Stderr, "my-orchestra: "+err.Error())
		os.Exit(1)
	}
}

func run(configDir string, seed, resume bool) error {
	cfg, err := config.Load(configDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	forge, err := ctxforge.Load(cfg.ForgeDir)
	if err != nil {
		return fmt.Errorf("load forge: %w", err)
	}

	if seed {
		return seedStarter(cfg, forge)
	}

	// Build the price book, applying any settings overrides.
	pb := telemetry.DefaultPriceBook()
	for model, r := range cfg.Telemetry.PriceOverrides {
		pb.Set(telemetry.ModelRate{
			Model: model, InputPerMTok: r[0], CachedInputPerMTok: r[1], OutputPerMTok: r[2],
		})
	}
	meter := telemetry.NewMeter(pb)

	// Connect to the Copilot runtime via the official Go SDK, authenticating
	// with the logged-in `copilot` CLI session; if it cannot start (no `copilot`
	// CLI on PATH, or not logged in), fall back to the offline mock so the TUI
	// remains usable for inspection.
	client, closeFn := dialClient(cfg)
	defer closeFn()

	model := tui.New(tui.Deps{Config: cfg, Forge: forge, Client: client, Meter: meter, Resume: resume})
	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

// runWeb serves the htmx web UI (WEB_UI_PLAN.md). In demo mode it drives a
// scripted MockClient so the streaming skeleton works with no Copilot runtime.
func runWeb(configDir, addr string, demo bool) error {
	cfg, err := config.Load(configDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	forge, err := ctxforge.Load(cfg.ForgeDir)
	if err != nil {
		return fmt.Errorf("load forge: %w", err)
	}

	pb := telemetry.DefaultPriceBook()
	for model, r := range cfg.Telemetry.PriceOverrides {
		pb.Set(telemetry.ModelRate{
			Model: model, InputPerMTok: r[0], CachedInputPerMTok: r[1], OutputPerMTok: r[2],
		})
	}
	meter := telemetry.NewMeter(pb)

	var client copilot.Client
	var closeFn func()
	if demo {
		mock := copilot.NewMockClient()
		client, closeFn = mock, func() { _ = mock.Close() }
	} else {
		client, closeFn = dialClient(cfg)
	}
	defer closeFn()

	srv := web.New(web.Options{
		Client: client,
		Forge:  forge,
		Config: cfg,
		Meter:  meter,
		Spec:   copilot.SessionSpec{Model: cfg.DefaultModel, Streaming: true},
		Demo:   demo,
		Logger: log.New(os.Stderr, "web: ", log.LstdFlags),
	})

	httpSrv := &http.Server{Addr: addr, Handler: srv.Handler()}
	fmt.Printf("my-orchestra web UI on http://%s\n", addr)
	return httpSrv.ListenAndServe()
}

// dialClient starts the SDK-backed client, returning a mock if the runtime is
// unavailable so the TUI still launches.
func dialClient(cfg *config.Config) (copilot.Client, func()) {
	// Prefer the already-logged-in `copilot` CLI session; only fall back to an
	// explicit token when one is configured via GitHubTokenEnv.
	token, useLoggedInUser := copilot.ResolveAuth(cfg.GitHubToken())
	c, err := copilot.NewSDKClient(context.Background(), copilot.Options{
		GitHubToken:     token,
		UseLoggedInUser: useLoggedInUser,
		OTLPEndpoint:    cfg.Telemetry.OTLPEndpoint,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "my-orchestra: copilot runtime unavailable ("+err.Error()+"); using offline mock")
		mock := copilot.NewMockClient()
		return mock, func() { _ = mock.Close() }
	}
	return c, func() { _ = c.Close() }
}

func defaultConfigDir() string {
	if dir := os.Getenv("MY_ORCHESTRA_HOME"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".my-orchestra"
	}
	return filepath.Join(home, ".my-orchestra")
}

// seedStarter writes a representative forge and config so first runs have
// something to explore.
func seedStarter(cfg *config.Config, forge *ctxforge.Forge) error {
	if len(forge.Skills) == 0 {
		_ = forge.AddSkill(ctxforge.Skill{
			ID: "tdd", Name: "Test-Driven Development", Command: "tdd",
			Description: "Write a failing test before any implementation.",
			Prompt:      "Always write a failing test first, then the minimum code to pass it, then refactor.",
			Enabled:     true,
		})
		_ = forge.AddSkill(ctxforge.Skill{
			ID: "cost-aware", Name: "Cost-aware engineering",
			Description: "Prefer cheaper models and minimal tokens.",
			Prompt:      "Be token-frugal: prefer concise diffs, avoid re-reading unchanged files, and pick the cheapest capable model.",
			Enabled:     true,
		})
	}
	if len(forge.Instructions) == 0 {
		forge.Instructions = append(forge.Instructions, ctxforge.Instruction{
			ID: "no-secrets", Title: "Never leak secrets", Priority: 1, Enabled: true,
			Body: "Never print or commit secrets, tokens, or credentials.",
		})
	}
	if len(forge.Agents) == 0 {
		forge.Agents = append(forge.Agents,
			ctxforge.Agent{ID: "builder", Name: "Builder", Description: "Implements features test-first",
				Model: "gpt-5", ReasoningEffort: "high", Skills: []string{"tdd"}},
			ctxforge.Agent{ID: "sdet", Name: "SDET", Description: "Hardens code with adversarial tests",
				Model: "claude-sonnet-4.6", ReasoningEffort: "high"},
		)
	}
	if err := forge.Save(); err != nil {
		return err
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Printf("seeded forge + config in %s\n", cfg.Dir())
	return nil
}
