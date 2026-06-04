package main

import (
	"testing"

	"github.com/dotts-h/copilot-sdk/internal/config"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
)

// seedStarter writes a valid starter forge + config to a fresh config dir and
// exits without serving. (The SeedForge backfill rules themselves are guarded
// in internal/bootstrap.)
func TestSeedStarterWritesValidForge(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MY_ORCHESTRA_HOME", dir)

	if err := seedStarter(dir); err != nil {
		t.Fatalf("seedStarter: %v", err)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	forge, err := ctxforge.Load(cfg.ForgeDir)
	if err != nil {
		t.Fatalf("reload forge: %v", err)
	}
	if len(forge.Skills) == 0 || len(forge.Agents) == 0 {
		t.Errorf("seeded forge missing content: %d skills, %d agents", len(forge.Skills), len(forge.Agents))
	}
	if err := forge.Validate(); err != nil {
		t.Errorf("persisted forge should be valid: %v", err)
	}
}
