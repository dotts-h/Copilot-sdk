package main

import (
	"testing"

	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
)

// seedForge must populate an empty forge with representative skills, instructions,
// and agents. Regression: demo mode shipped no forge, so the Skills/Agents pages
// rendered empty and the browser suite had nothing to drive (e2e.yml went red).
func TestSeedForgePopulatesEmptyForge(t *testing.T) {
	f := &ctxforge.Forge{}
	seedForge(f)

	if len(f.Skills) == 0 {
		t.Error("seedForge should add skills")
	}
	if len(f.Agents) == 0 {
		t.Error("seedForge should add agents")
	}
	if len(f.Instructions) == 0 {
		t.Error("seedForge should add instructions")
	}
	if err := f.Validate(); err != nil {
		t.Errorf("seeded forge should be valid: %v", err)
	}
}

// seedForge backfills only empty kinds, so an existing forge is never clobbered.
func TestSeedForgePreservesExisting(t *testing.T) {
	f := &ctxforge.Forge{}
	if err := f.AddSkill(ctxforge.Skill{ID: "mine", Name: "Mine", Prompt: "do the thing", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	seedForge(f)

	if len(f.Skills) != 1 || f.Skills[0].ID != "mine" {
		t.Errorf("seedForge should not overwrite existing skills: %+v", f.Skills)
	}
}
