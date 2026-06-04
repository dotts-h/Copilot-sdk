// Command my-orchestra is a cost-aware coding web app that fuses the GitHub
// Copilot CLI and Claude CLI experiences. It drives the GitHub Copilot SDK
// (through a Node sidecar), assembles context with the my-ctx forge, meters
// AI-Credit spend live, and serves a server-rendered htmx UI over SSE.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/dotts-h/copilot-sdk/internal/bootstrap"
	"github.com/dotts-h/copilot-sdk/internal/config"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
)

// version is overridden at build time via -ldflags.
var version = "dev"

func main() {
	var (
		showVersion bool
		configDir   string
		seed        bool
		addr        string
		demo        bool
	)
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.StringVar(&configDir, "config-dir", bootstrap.DefaultConfigDir(), "configuration directory")
	flag.BoolVar(&seed, "seed", false, "write a starter forge + config to the config dir and exit")
	flag.StringVar(&addr, "addr", "127.0.0.1:8765", "address for the web UI")
	flag.BoolVar(&demo, "demo", false, "drive the web UI with a scripted mock (no Copilot runtime)")
	flag.Parse()

	if showVersion {
		fmt.Printf("my-orchestra %s\n", version)
		return
	}

	if err := run(configDir, addr, seed, demo); err != nil {
		fmt.Fprintln(os.Stderr, "my-orchestra: "+err.Error())
		os.Exit(1)
	}
}

// run builds the configured server via bootstrap and serves the htmx web UI.
// The -seed path writes a starter forge + config and exits without serving.
func run(configDir, addr string, seed, demo bool) error {
	if seed {
		return seedStarter(configDir)
	}

	srv, closeFn, err := bootstrap.Build(configDir, demo)
	if err != nil {
		return err
	}
	defer closeFn()

	httpSrv := &http.Server{Addr: addr, Handler: srv.Handler()}
	fmt.Printf("my-orchestra web UI on http://%s\n", addr)
	return httpSrv.ListenAndServe()
}

// seedStarter writes a representative forge and config so first runs have
// something to explore.
func seedStarter(configDir string) error {
	cfg, err := config.Load(configDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	forge, err := ctxforge.Load(cfg.ForgeDir)
	if err != nil {
		return fmt.Errorf("load forge: %w", err)
	}
	bootstrap.SeedForge(forge)
	if err := forge.Save(); err != nil {
		return err
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Printf("seeded forge + config in %s\n", cfg.Dir())
	return nil
}
