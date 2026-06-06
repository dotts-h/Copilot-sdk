// Package bootstrap assembles a ready-to-serve web.Hub from on-disk config and
// forge, wiring the price book, meter, the compiled default-agent SessionSpec,
// and the Copilot client (real SDK, or an offline/demo mock). Both the web
// entrypoint (cmd/my-orchestra) and the desktop shell (cmd/my-orchestra-desktop)
// share this so the two binaries serve byte-identical UIs over the same seam.
package bootstrap

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/config"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
	"github.com/dotts-h/copilot-sdk/internal/web"
)

// Build loads config + forge from configDir, builds the meter, compiles the
// default agent into a SessionSpec, dials the Copilot client, and returns a
// configured *web.Hub whose Handler() serves the htmx UI. The returned close
// func tears the client down and must be called on shutdown.
//
// In demo mode it drives a scripted MockClient (seeding a representative forge,
// models, and persisted sessions when none exist on disk) so the streaming UI
// works with no Copilot runtime — this backs the e2e suite and must stay
// deterministic.
func Build(configDir string, demo bool) (srv *web.Hub, close func(), err error) {
	cfg, err := config.Load(configDir)
	if err != nil {
		return nil, nil, fmt.Errorf("load config: %w", err)
	}
	forge, err := ctxforge.Load(cfg.ForgeDir)
	if err != nil {
		return nil, nil, fmt.Errorf("load forge: %w", err)
	}

	// Build the price book, applying any settings overrides.
	pb := telemetry.DefaultPriceBook()
	for model, r := range cfg.Telemetry.PriceOverrides {
		pb.Set(telemetry.ModelRate{
			Model: model, InputPerMTok: r[0], CachedInputPerMTok: r[1], OutputPerMTok: r[2],
		})
	}
	meter := telemetry.NewMeter(pb)

	// Persisted, append-only spend ledger (survives restart; feeds the Telemetry
	// trend view). Demo uses an ephemeral, pre-seeded store so the trend renders
	// deterministically offline without writing to a real config directory.
	var spend *telemetry.SpendStore
	if demo {
		spend, _ = telemetry.LoadSpendStore("")
		seedSpend(spend)
	} else {
		spend, err = telemetry.LoadSpendStore(configDir)
		if err != nil {
			return nil, nil, fmt.Errorf("load spend history: %w", err)
		}
	}

	// Persisted workflow-run history (sibling of the spend ledger; survives restart,
	// feeds the Runs page). Demo uses an ephemeral, pre-seeded store so the Runs page
	// renders deterministically offline (ADR-0022).
	var runs *telemetry.RunStore
	if demo {
		runs, _ = telemetry.LoadRunStore("")
		seedRuns(runs)
	} else {
		runs, err = telemetry.LoadRunStore(configDir)
		if err != nil {
			return nil, nil, fmt.Errorf("load run history: %w", err)
		}
	}

	// Compile the configured default agent (or just the enabled global
	// instructions/skills when none) so the very first session carries the same
	// persona an explicit agent selection would apply later. SeamSpec is the same
	// forge→seam translation the agent-restart and workflow-lane paths use.
	cspec, err := forge.Compile(cfg.DefaultAgent)
	if err != nil {
		log.Printf("compile default agent %q: %v", cfg.DefaultAgent, err)
	}
	spec := web.SeamSpec(cspec, cfg.DefaultModel, cfg.ReasoningEffort)

	var client copilot.Client
	var closeFn func()
	if demo {
		client, closeFn = demoClient(forge, &spec)
	} else {
		client, closeFn = dialClient(cfg)
	}

	srv = web.New(web.Options{
		Client: client,
		Forge:  forge,
		Config: cfg,
		Meter:  meter,
		Spend:  spend,
		Runs:   runs,
		Spec:   spec,
		Demo:   demo,
		Logger: log.New(os.Stderr, "web: ", log.LstdFlags),
	})
	return srv, closeFn, nil
}

// demoClient builds the deterministic scripted MockClient that backs demo mode
// and the e2e suite. It seeds a representative forge in memory when none is on
// disk (otherwise the Skills/Agents pages render empty), pins the spec to a
// listed model, and stages a couple of persisted sessions to resume/delete.
func demoClient(forge *ctxforge.Forge, spec *copilot.SessionSpec) (copilot.Client, func()) {
	if len(forge.Skills) == 0 && len(forge.Agents) == 0 {
		SeedForge(forge)
	}
	mock := copilot.NewMockClient()
	mock.Models = []copilot.ModelInfo{
		{ID: "gpt-5", Name: "GPT-5", SupportedReasoningEfforts: []string{"low", "medium", "high"}},
		{ID: "claude-sonnet-4-6", Name: "Claude Sonnet 4.6", SupportedReasoningEfforts: []string{"medium", "high"}},
		{ID: "claude-haiku-4-5", Name: "Claude Haiku 4.5"},
	}
	// Pin the demo to a listed model so the picker and reasoning-effort row are
	// self-consistent (and the statusline shows a real model).
	spec.Model, spec.ReasoningEffort = "gpt-5", "medium"
	// Seed a couple of persisted sessions (with history) so the Sessions page
	// has something to list, resume, and delete in demo / e2e.
	mock.Sessions = []copilot.SessionMeta{
		{ID: "demo-sess-1", Summary: "Refactor the auth flow", Modified: time.Now().Add(-30 * time.Minute)},
		{ID: "demo-sess-2", Summary: "Investigate the flaky CI run", Modified: time.Now().Add(-26 * time.Hour)},
	}
	mock.Histories = map[string][]copilot.Event{
		"demo-sess-1": {
			{Type: copilot.EvUserMessage, Text: "Help me refactor the auth flow"},
			{Type: copilot.EvReasoning, Text: "The login handler mixes parsing and policy; split them."},
			{Type: copilot.EvMessage, Text: "Done. I extracted `parseCredentials` from the handler and added a `Policy` seam.\n\n```go\nfunc parseCredentials(r *http.Request) (Creds, error) { /* … */ }\n```"},
		},
		"demo-sess-2": {
			{Type: copilot.EvUserMessage, Text: "Why is the e2e job flaky?"},
			{Type: copilot.EvMessage, Text: "The forge tests assumed an on-disk `forge.json`; in CI the demo seeds in memory now."},
		},
	}
	return mock, func() { _ = mock.Close() }
}

// seedSpend pre-loads the demo's ephemeral ledger with a few days of spend so
// the Telemetry trend view (spend over time, per-model share, the per-agent /
// per-workflow cost breakdown, CSV export) has something to show offline. The
// records carry agent ids — and a couple a workflow run owned (workflow id + lane
// index, ADR-0018) — so the orchestration-aware attribution view renders without
// a live run. Deterministic shape; dates are relative to now so the trend always
// lands in the recent window the page shows.
func seedSpend(store *telemetry.SpendStore) {
	now := time.Now()
	at := func(daysAgo int) time.Time { return now.AddDate(0, 0, -daysAgo) }
	for _, r := range []telemetry.SpendRecord{
		{At: at(4), SessionID: "demo-sess-2", Model: "gpt-5", InputTokens: 900, OutputTokens: 220, USD: 0.012, AgentID: "builder"},
		{At: at(3), SessionID: "demo-sess-2", Model: "claude-sonnet-4-6", InputTokens: 1500, OutputTokens: 400, USD: 0.030, AgentID: "sdet"},
		{At: at(2), SessionID: "demo-sess-1", Model: "gpt-5", InputTokens: 1200, CachedTokens: 200, OutputTokens: 340, USD: 0.018, AgentID: "builder"},
		// Two turns a workflow run owned: the Build & harden lanes (sequential),
		// attributed to the run and the lane within it.
		{At: at(1), SessionID: "demo-sess-1", Model: "gpt-5", InputTokens: 2200, CachedTokens: 600, OutputTokens: 520, USD: 0.026, AgentID: "builder", WorkflowID: "build-and-harden", LaneIndex: 0},
		{At: at(0), SessionID: "demo-sess-1", Model: "claude-sonnet-4-6", InputTokens: 1400, OutputTokens: 360, USD: 0.022, AgentID: "sdet", WorkflowID: "build-and-harden", LaneIndex: 1},
		{At: at(0), SessionID: "demo-sess-1", Model: "claude-haiku-4-5", InputTokens: 800, OutputTokens: 150, USD: 0.004, AgentID: "builder"},
	} {
		_ = store.Append(r)
	}
}

// seedRuns pre-loads the demo's ephemeral run history with a couple of completed
// workflow runs so the Runs page renders offline — including a branched run with a
// skipped lane (the per-lane outcome a spend record can't express, ADR-0022).
// Deterministic shape; timestamps are relative to now so they read as recent.
func seedRuns(store *telemetry.RunStore) {
	now := time.Now()
	for _, r := range []telemetry.RunRecord{
		{
			ID: "run-demo-1", WorkflowID: "build-and-harden", Name: "Build & harden",
			Mode: "sequential", StartedAt: now.Add(-26 * time.Hour),
			FinishedAt: now.Add(-26 * time.Hour).Add(2 * time.Minute), Outcome: "finished",
			Lanes: []telemetry.RunLane{
				{Index: 0, AgentID: "builder", Status: "done", Credits: 2.6},
				{Index: 1, AgentID: "sdet", Status: "done", Credits: 2.2},
			},
		},
		{
			ID: "run-demo-2", WorkflowID: "review-and-fix", Name: "Review & fix",
			Mode: "sequential", StartedAt: now.Add(-3 * time.Hour),
			FinishedAt: now.Add(-3 * time.Hour).Add(90 * time.Second), Outcome: "finished",
			Lanes: []telemetry.RunLane{
				{Index: 0, AgentID: "sdet", Status: "done", Credits: 1.8},
				{Index: 1, AgentID: "builder", Status: "skipped"}, // the review found no issues, so the fix branch skipped
			},
		},
	} {
		_ = store.Append(r)
	}
}

// dialClient starts the SDK-backed client, returning a mock if the runtime is
// unavailable so the app still launches.
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

// ServeLocal binds an ephemeral loopback port and serves h on it in a
// background goroutine, returning the chosen port and a stop func. The desktop
// shell points a webview window at the returned port; keeping this in bootstrap
// (free of the Wails/CGO dependency) lets it stay unit-testable without a
// native window or display.
func ServeLocal(h http.Handler) (port int, stop func(), err error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, nil, err
	}
	hs := &http.Server{Handler: h}
	go func() { _ = hs.Serve(ln) }()
	return ln.Addr().(*net.TCPAddr).Port, func() { _ = hs.Close() }, nil
}

// DefaultConfigDir is the per-user config directory, overridable via the
// MY_ORCHESTRA_HOME env var. Shared by every entrypoint.
func DefaultConfigDir() string {
	if dir := os.Getenv("MY_ORCHESTRA_HOME"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".my-orchestra"
	}
	return filepath.Join(home, ".my-orchestra")
}

// SeedForge populates a representative set of skills, instructions, and agents
// in memory (no disk write). It backfills only the empty kinds, so an existing
// forge is never clobbered. Shared by the on-disk seeder (-seed) and demo mode.
func SeedForge(forge *ctxforge.Forge) {
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
		// Only pin the tdd skill if it actually exists — when the forge already had
		// skills (so we didn't seed tdd above), pinning it would dangle the
		// reference and fail forge.Validate() on the next Save (e.g. under -seed).
		var builderSkills []string
		if forge.Skill("tdd") != nil {
			builderSkills = []string{"tdd"}
		}
		forge.Agents = append(forge.Agents,
			ctxforge.Agent{ID: "builder", Name: "Builder", Description: "Implements features test-first",
				Model: "gpt-5", ReasoningEffort: "high", Skills: builderSkills},
			ctxforge.Agent{ID: "sdet", Name: "SDET", Description: "Hardens code with adversarial tests",
				Model: "claude-sonnet-4.6", ReasoningEffort: "high"},
		)
	}
	if len(forge.Workflows) == 0 && forge.Agent("builder") != nil && forge.Agent("sdet") != nil {
		// A representative sequential handoff: the Builder implements, then the SDET
		// hardens its output. Only seeded when both agents exist, so referential
		// integrity holds under any partial state (mirrors the tdd-skill guard above).
		_ = forge.AddWorkflow(ctxforge.Workflow{
			ID: "build-and-harden", Name: "Build & harden", Mode: ctxforge.WorkflowSequential,
			Description: "Builder implements a change, then SDET hardens it with tests.",
			Steps: []ctxforge.WorkflowStep{
				{AgentID: "builder", Prompt: "Implement the requested change."},
				{AgentID: "sdet", Prompt: "Review and harden the previous step with adversarial tests."},
			},
		})
		// A representative PARALLEL fan-out: Builder and SDET review the same change
		// at once. With the mock now handing out distinct session ids and the demo
		// lane tagging its events with them (B1 / issue 0015), this drives two
		// concurrent lanes offline — the parallel path the demo/e2e couldn't reach.
		_ = forge.AddWorkflow(ctxforge.Workflow{
			ID: "parallel-review", Name: "Parallel review", Mode: ctxforge.WorkflowParallel,
			Description: "Builder and SDET review the same change at once (fan-out).",
			Steps: []ctxforge.WorkflowStep{
				{AgentID: "builder", Prompt: "Review this change for correctness and design."},
				{AgentID: "sdet", Prompt: "Review this change for test coverage and edge cases."},
			},
		})
		// A representative BRANCHING workflow (B2 / ADR-0021): the SDET reviews and
		// flags issues, then the Builder's fix step runs only IF the review output
		// mentions "issues", while a celebrate step runs only if it says "perfect".
		// The demo lane echoes the prompt's first line, so the review output contains
		// "issues" — the fix step RUNS and the celebrate step SKIPS, driving a real
		// branch (a skipped lane) offline for the demo/e2e.
		_ = forge.AddWorkflow(ctxforge.Workflow{
			ID: "review-then-fix", Name: "Review, then fix", Mode: ctxforge.WorkflowSequential,
			Description: "SDET reviews; the Builder fixes only if the review flags issues.",
			Steps: []ctxforge.WorkflowStep{
				{AgentID: "sdet", Prompt: "Review this change and flag any issues you find."},
				{AgentID: "builder", Prompt: "Apply fixes for the flagged issues.",
					When: &ctxforge.StepCondition{Step: 1, Condition: ctxforge.CondOutputContains, Value: "issues"}},
				{AgentID: "builder", Prompt: "Nothing to fix — record that the change is perfect.",
					When: &ctxforge.StepCondition{Step: 1, Condition: ctxforge.CondOutputContains, Value: "perfect"}},
			},
		})
	}
	if len(forge.Snippets) == 0 {
		// A couple of representative library prompts so the composer autocomplete
		// and the Snippets page have something to drive out of the box (item 3.4).
		_ = forge.AddSnippet(ctxforge.Snippet{
			ID: "review-pr", Name: "Review this PR",
			Body: "Review the current diff for correctness and style, then summarise the risks.",
		})
		_ = forge.AddSnippet(ctxforge.Snippet{
			ID: "explain", Name: "Explain this code",
			Body: "Explain what the selected code does, step by step, and note any edge cases.",
		})
		// A multi-line body so the composer (a <textarea> since C2) and its e2e can
		// prove that inserting a snippet keeps its line breaks (TECH_DEBT #15).
		_ = forge.AddSnippet(ctxforge.Snippet{
			ID: "checklist", Name: "Review checklist",
			Body: "Review checklist:\n1. Correctness and edge cases\n2. Tests and coverage\n3. Naming and docs",
		})
	}
	if len(forge.MCPServers) == 0 {
		// Curated, well-known stdio MCP servers, seeded DISABLED by default (ADR
		// 0010). All are key-free (no secrets UI yet) and external processes, so the
		// user enables one only after the MCP page's PATH preflight confirms its
		// command is installed — auto-enabling would surprise-fail at session start
		// and clashes with the offline single-binary value.
		for _, m := range curatedMCPServers() {
			_ = forge.AddMCPServer(m)
		}
	}
}

// curatedMCPServers returns the baked-in set of well-known, key-free stdio MCP
// servers. They are seeded disabled; the command (npx/uvx) must be present on the
// host before the server can be enabled — the MCP page flags the unavailable ones.
func curatedMCPServers() []ctxforge.MCPServer {
	return []ctxforge.MCPServer{
		{ID: "filesystem", Name: "Filesystem", Command: "npx",
			Args: []string{"-y", "@modelcontextprotocol/server-filesystem", "."}},
		{ID: "git", Name: "Git", Command: "uvx", Args: []string{"mcp-server-git"}},
		{ID: "fetch", Name: "Fetch", Command: "uvx", Args: []string{"mcp-server-fetch"}},
		{ID: "memory", Name: "Memory", Command: "npx",
			Args: []string{"-y", "@modelcontextprotocol/server-memory"}},
		{ID: "sequential-thinking", Name: "Sequential Thinking", Command: "npx",
			Args: []string{"-y", "@modelcontextprotocol/server-sequential-thinking"}},
		{ID: "time", Name: "Time", Command: "uvx", Args: []string{"mcp-server-time"}},
	}
}
