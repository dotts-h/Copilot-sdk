// Command my-orchestra is a cost-aware coding TUI that fuses the GitHub Copilot
// CLI and Claude CLI experiences. It drives the GitHub Copilot SDK (through a
// Node sidecar), assembles context with the my-ctx forge, and meters AI-Credit
// spend live.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dotts-h/copilot-sdk/internal/config"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
	"github.com/dotts-h/copilot-sdk/internal/tui"
)

// version is overridden at build time via -ldflags.
var version = "dev"

func main() {
	var (
		showVersion bool
		configDir   string
		seed        bool
	)
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.StringVar(&configDir, "config-dir", defaultConfigDir(), "configuration directory")
	flag.BoolVar(&seed, "seed", false, "write a starter forge + config to the config dir and exit")
	flag.Parse()

	if showVersion {
		fmt.Printf("my-orchestra %s\n", version)
		return
	}

	if err := run(configDir, seed); err != nil {
		fmt.Fprintln(os.Stderr, "my-orchestra: "+err.Error())
		os.Exit(1)
	}
}

func run(configDir string, seed bool) error {
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

	// Spawn the Copilot SDK sidecar; if it cannot start, fall back to the mock
	// so the TUI is still usable for inspection.
	client, closeFn := dialClient(cfg)
	defer closeFn()

	model := tui.New(tui.Deps{Config: cfg, Forge: forge, Client: client, Meter: meter})
	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

// dialClient spawns the sidecar, returning a mock client if spawning fails.
func dialClient(cfg *config.Config) (copilot.Client, func()) {
	c, err := copilot.Spawn(copilot.SpawnConfig{
		Command:     cfg.SidecarCommand,
		Args:        cfg.SidecarArgs,
		GitHubToken: cfg.GitHubToken(),
		Stderr:      io.Discard,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "my-orchestra: sidecar unavailable ("+err.Error()+"); using offline mock")
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
