package bootstrap

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
)

// ServeLocal serves the handler on an ephemeral loopback port; after stop() the
// listener is closed. This is the desktop shell's window-free seam.
func TestServeLocal(t *testing.T) {
	port, stop, err := ServeLocal(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	if err != nil {
		t.Fatalf("ServeLocal: %v", err)
	}
	if port <= 0 {
		t.Fatalf("ServeLocal returned bad port %d", port)
	}

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}

	stop()
	if _, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port)); err == nil {
		t.Error("expected GET to fail after stop()")
	}
}

// Build in demo mode must return a server whose Handler serves the app shell,
// with no Copilot runtime present. This guards the assembly the desktop shell
// and the web entrypoint both depend on.
func TestBuildDemoServesIndex(t *testing.T) {
	srv, closeFn, err := Build(t.TempDir(), true)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if srv == nil {
		t.Fatal("Build returned nil server")
	}
	t.Cleanup(closeFn) // must be safe to call

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", rec.Code)
	}
	if rec.Body.Len() == 0 {
		t.Error("GET / returned empty body")
	}
}

// SeedForge must populate an empty forge with representative skills, instructions,
// and agents. Regression: demo mode shipped no forge, so the Skills/Agents pages
// rendered empty and the browser suite had nothing to drive (e2e.yml went red).
func TestSeedForgePopulatesEmptyForge(t *testing.T) {
	f := &ctxforge.Forge{}
	SeedForge(f)

	if len(f.Skills) == 0 {
		t.Error("SeedForge should add skills")
	}
	if len(f.Agents) == 0 {
		t.Error("SeedForge should add agents")
	}
	if len(f.Instructions) == 0 {
		t.Error("SeedForge should add instructions")
	}
	if err := f.Validate(); err != nil {
		t.Errorf("seeded forge should be valid: %v", err)
	}
}

// SeedForge backfills only empty kinds, so an existing forge is never clobbered.
func TestSeedForgePreservesExisting(t *testing.T) {
	f := &ctxforge.Forge{}
	if err := f.AddSkill(ctxforge.Skill{ID: "mine", Name: "Mine", Prompt: "do the thing", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	SeedForge(f)

	if len(f.Skills) != 1 || f.Skills[0].ID != "mine" {
		t.Errorf("SeedForge should not overwrite existing skills: %+v", f.Skills)
	}
	// Backfilling agents onto a forge that already had skills must stay valid: the
	// seeded builder agent must not pin a skill (tdd) that was never seeded, or
	// -seed's forge.Save() would fail Validate on a dangling reference.
	if err := f.Validate(); err != nil {
		t.Errorf("partial-seed forge should remain valid: %v", err)
	}
}
