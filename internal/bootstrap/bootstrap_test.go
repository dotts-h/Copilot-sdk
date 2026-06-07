package bootstrap

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
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

// seedSpend must populate the demo ledger with a deterministic, multi-day,
// multi-model history so the Telemetry trend view renders something offline.
func TestSeedSpendPopulatesDeterministicHistory(t *testing.T) {
	store, err := telemetry.LoadSpendStore("") // ephemeral
	if err != nil {
		t.Fatal(err)
	}
	seedSpend(store)
	recs := store.Records()
	if len(recs) < 3 {
		t.Fatalf("seedSpend should add several records, got %d", len(recs))
	}
	if days := telemetry.DailyTotals(recs); len(days) < 2 {
		t.Fatalf("seed should span multiple days, got %d", len(days))
	}
	if shares := telemetry.ModelShares(recs); len(shares) < 2 {
		t.Fatalf("seed should span multiple models, got %d", len(shares))
	}
}

// A demo-mode Build wires the seeded ledger through to the Telemetry page, so
// the trend view and CSV export are populated with no Copilot runtime.
func TestBuildDemoTelemetryShowsTrend(t *testing.T) {
	srv, closeFn, err := Build(t.TempDir(), true)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(closeFn)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/page/telemetry", nil))
	if !strings.Contains(rec.Body.String(), "Spend over time") {
		t.Fatalf("demo telemetry page missing the trend view:\n%s", rec.Body.String())
	}
	// The cost⋈run reconciliation (V15) joins the demo's seeded ledger and run
	// history per workflow: build-and-harden agrees across both stores while
	// review-and-fix metered a run with no matching ledger spend, so its delta is
	// non-trivial and ambered — the divergence the section exists to surface. The
	// per-lane reconciliation (V16) renders the same comparison one grain finer
	// (build-and-harden's lanes are seeded lane-tagged on both sides).
	for _, want := range []string{
		"Ledger vs runs", `class="grid recon"`, `recon-delta amber`,
		"Ledger vs runs by lane", `class="grid lane-recon"`,
	} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("demo telemetry page missing reconciliation %q:\n%s", want, rec.Body.String())
		}
	}

	csv := httptest.NewRecorder()
	srv.Handler().ServeHTTP(csv, httptest.NewRequest(http.MethodGet, "/telemetry/export.csv", nil))
	if ct := csv.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("export Content-Type = %q, want text/csv", ct)
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

// SeedForge must seed a curated set of well-known stdio MCP servers, all
// DISABLED by default (item 2.2 / ADR-0010): seeding config is cheap, but
// enabling a stdio server whose binary is absent would surprise-fail at session
// start, so the user opts in after the page preflight confirms availability.
func TestSeedForgeSeedsMCPServersDisabled(t *testing.T) {
	f := &ctxforge.Forge{}
	SeedForge(f)

	if len(f.MCPServers) == 0 {
		t.Fatal("SeedForge should seed curated MCP servers")
	}
	for _, m := range f.MCPServers {
		if m.Enabled {
			t.Errorf("curated MCP server %q must be seeded disabled by default", m.ID)
		}
		if m.Command == "" {
			t.Errorf("curated MCP server %q must carry a command", m.ID)
		}
		// Curated defaults must be key-free (no secrets UI yet — ADR-0010).
		if len(m.Env) != 0 {
			t.Errorf("curated MCP server %q must not require env/secrets, got %+v", m.ID, m.Env)
		}
	}
	if err := f.Validate(); err != nil {
		t.Errorf("seeded forge with MCP servers should be valid: %v", err)
	}
}

// SeedForge backfills MCP servers independently: a forge that already has some is
// left untouched.
func TestSeedForgePreservesExistingMCPServers(t *testing.T) {
	f := &ctxforge.Forge{}
	if err := f.AddMCPServer(ctxforge.MCPServer{ID: "mine", Name: "Mine", Command: "my-cmd"}); err != nil {
		t.Fatal(err)
	}
	SeedForge(f)
	if len(f.MCPServers) != 1 || f.MCPServers[0].ID != "mine" {
		t.Errorf("SeedForge should not overwrite existing MCP servers: %+v", f.MCPServers)
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
