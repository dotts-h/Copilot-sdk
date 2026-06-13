// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package web

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/config"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// day parses an RFC3339 timestamp for fixed-date test records.
func day(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// newSpendServer builds a session wired with an ephemeral spend ledger so the
// persistence path is exercised without touching disk.
func newSpendServer(t *testing.T) (*Server, *telemetry.SpendStore) {
	t.Helper()
	store, err := telemetry.LoadSpendStore("") // ephemeral
	if err != nil {
		t.Fatal(err)
	}
	hub := New(Options{
		Client: copilot.NewMockClient(),
		Forge:  &ctxforge.Forge{},
		Config: &config.Config{DefaultModel: "gpt-5"},
		Meter:  telemetry.NewMeter(telemetry.DefaultPriceBook()),
		Spend:  store,
		Logger: log.New(io.Discard, "", 0),
	})
	return hub.newSession("spend-test"), store
}

func TestUsagePersistsSpendRecord(t *testing.T) {
	s, store := newSpendServer(t)
	s.sessionID = "sess-xyz"
	s.handleEvent(copilot.Event{Type: copilot.EvUsage, Usage: copilot.UsageData{
		Model: "gpt-5", InputTokens: 1200, CachedTokens: 200, OutputTokens: 340, NanoAIU: 1.2e7,
	}})
	recs := store.Records()
	if len(recs) != 1 {
		t.Fatalf("expected 1 persisted record, got %d", len(recs))
	}
	r := recs[0]
	if r.SessionID != "sess-xyz" || r.Model != "gpt-5" || r.InputTokens != 1200 {
		t.Fatalf("record fields wrong: %+v", r)
	}
	if r.USD <= 0 {
		t.Fatalf("record should carry a priced USD cost, got %v", r.USD)
	}
	// NanoAIU (1.2e7 nano) folds into AIU at 1e-9.
	if r.AIU <= 0 {
		t.Fatalf("record should carry the reported AIU, got %v", r.AIU)
	}
}

func TestUsageTagsActiveAgent(t *testing.T) {
	s, store := newSpendServer(t)
	s.agentID = "builder" // the active agent persona
	s.handleEvent(copilot.Event{Type: copilot.EvUsage, Usage: copilot.UsageData{
		Model: "gpt-5", InputTokens: 1000, OutputTokens: 200,
	}})
	rec := store.Records()[0]
	if rec.AgentID != "builder" {
		t.Fatalf("chat turn should be tagged with the active agent, got %q", rec.AgentID)
	}
	// A plain chat turn carries no workflow attribution.
	if rec.WorkflowID != "" || rec.LaneIndex != 0 {
		t.Fatalf("chat turn must not carry workflow attribution: %+v", rec)
	}
}

func TestWorkflowUsageTagsWorkflowAndLane(t *testing.T) {
	s, store := newSpendServer(t)
	// A parallel run whose lanes have distinct backing session ids, so the EvUsage
	// routes to lane 1 by SessionID (run id "w", lane agent "b", index 1).
	run := twoStepRun(ctxforge.WorkflowParallel)
	s.mu.Lock()
	s.run = run
	s.busy = true
	run.start()
	run.lanes[0].sessionID = "s0"
	run.lanes[1].sessionID = "s1"
	s.mu.Unlock()

	s.handleEvent(copilot.Event{Type: copilot.EvUsage, SessionID: "s1", Usage: copilot.UsageData{
		Model: "gpt-5", InputTokens: 1000, OutputTokens: 200,
	}})
	rec := store.Records()[0]
	if rec.WorkflowID != "w" || rec.LaneIndex != 1 || rec.AgentID != "b" {
		t.Fatalf("workflow turn should be tagged with workflow id + lane index + lane agent: %+v", rec)
	}
}

func TestTelemetryPageShowsAttributionBreakdown(t *testing.T) {
	s, store := newSpendServer(t)
	s.forge.Agents = []ctxforge.Agent{{ID: "builder", Name: "Builder", Model: "gpt-5"}}
	s.forge.Workflows = []ctxforge.Workflow{{ID: "ship", Name: "Ship", Mode: ctxforge.WorkflowSequential}}
	for _, r := range []telemetry.SpendRecord{
		{At: day("2026-06-04T10:00:00Z"), Model: "gpt-5", USD: 0.20, AgentID: "builder"},
		{At: day("2026-06-05T10:00:00Z"), Model: "gpt-5", USD: 0.30, AgentID: "builder", WorkflowID: "ship", LaneIndex: 0},
		{At: day("2026-06-05T11:00:00Z"), Model: "gpt-5", USD: 0.10}, // plain chat, empty agent
	} {
		if err := store.Append(r); err != nil {
			t.Fatal(err)
		}
	}
	html := s.telemetryPartial(defaultSpendWindow)
	for _, want := range []string{"Cost by agent", "Builder", "chat (built-in)", "Cost by workflow", "Ship"} {
		if !strings.Contains(html, want) {
			t.Fatalf("telemetry page missing %q\n%s", want, html)
		}
	}
}

func TestTelemetryPageShowsLedgerVsRunsReconciliation(t *testing.T) {
	// The "Ledger vs runs" reconciliation (V15) joins the two persisted stores per
	// workflow: the ledger spend (WorkflowShares grain) beside what the workflow's
	// recorded runs metered (RunAggregates grain), and the delta. A non-trivial delta
	// is ambered. The workflow id resolves to its display name under forgeMu.
	s, store := newSpendServer(t)
	s.forge.Workflows = []ctxforge.Workflow{{ID: "ship", Name: "Ship", Mode: ctxforge.WorkflowSequential}}
	rs := withRunStore(s)
	// Ledger: the workflow metered 5.00 cr ($0.05) over two turns.
	for _, r := range []telemetry.SpendRecord{
		{At: day("2026-06-04T10:00:00Z"), Model: "gpt-5", USD: 0.03, WorkflowID: "ship", LaneIndex: 0},
		{At: day("2026-06-05T10:00:00Z"), Model: "gpt-5", USD: 0.02, WorkflowID: "ship", LaneIndex: 0},
	} {
		if err := store.Append(r); err != nil {
			t.Fatal(err)
		}
	}
	// Runs: a recorded run metered only 4.00 cr — diverges from the ledger (delta +1.00).
	if err := rs.Append(telemetry.RunRecord{
		ID: "r1", WorkflowID: "ship", Name: "Ship", Mode: "sequential", Outcome: "finished",
		Lanes: []telemetry.RunLane{{Index: 0, Status: "done", Credits: 4}},
	}); err != nil {
		t.Fatal(err)
	}
	html := s.telemetryPartial(defaultSpendWindow)
	for _, want := range []string{
		"Ledger vs runs",            // the section heading
		`class="grid recon"`,        // the comparison table
		`class="recon-row"`,         // a per-workflow row
		"Ship",                      // the workflow id resolved to its display name
		`class="recon-delta amber"`, // the non-trivial delta is ambered
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("telemetry page missing reconciliation %q\n%s", want, html)
		}
	}
}

func TestTelemetryPageShowsPerLaneReconciliation(t *testing.T) {
	// The per-lane "Ledger vs runs by lane" reconciliation (V16) is the per-workflow
	// "Ledger vs runs" table one grain finer: each (workflow, lane)'s ledger spend
	// (lane-tagged SpendRecords) beside what its recorded run lane metered, and the delta —
	// so a divergence is locatable at the exact step, not just the workflow total. The lane
	// is named "<workflow> · step <n>" (n = lane index + 1), resolving the workflow id to
	// its display name under forgeMu; a non-trivial delta is ambered.
	s, store := newSpendServer(t)
	s.forge.Workflows = []ctxforge.Workflow{{ID: "ship", Name: "Ship", Mode: ctxforge.WorkflowSequential}}
	rs := withRunStore(s)
	// Ledger: lane 0 of ship metered 5.00 cr ($0.05).
	if err := store.Append(telemetry.SpendRecord{
		At: day("2026-06-04T10:00:00Z"), Model: "gpt-5", USD: 0.05, WorkflowID: "ship", LaneIndex: 0,
	}); err != nil {
		t.Fatal(err)
	}
	// Runs: that lane's recorded run metered only 4.00 cr — diverges (delta +1.00).
	if err := rs.Append(telemetry.RunRecord{
		ID: "r1", WorkflowID: "ship", Name: "Ship", Mode: "sequential", Outcome: "finished",
		Lanes: []telemetry.RunLane{{Index: 0, AgentID: "builder", Status: "done", Credits: 4}},
	}); err != nil {
		t.Fatal(err)
	}
	html := s.telemetryPartial(defaultSpendWindow)
	for _, want := range []string{
		"Ledger vs runs by lane",           // the per-lane section heading
		`class="grid lane-recon"`,          // the per-lane comparison table
		`class="recon-row lane-recon-row"`, // a per-(workflow, lane) row
		"Ship · step 1",                    // the lane label (workflow name + 1-based step)
		`class="recon-delta amber"`,        // the non-trivial delta is ambered
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("telemetry page missing per-lane reconciliation %q\n%s", want, html)
		}
	}
}

func TestTelemetryPageShowsEstimateVsReportedDrift(t *testing.T) {
	// The "Estimate vs reported" drift table (issue 0060) joins the price-book
	// estimate to GitHub's reported cost per model over the REPORTED turns, the cost
	// cousin of the ledger⋈runs reconciliation: a delta past epsilon is ambered, and
	// unreported turns are counted as est-only coverage, never compared.
	s, store := newSpendServer(t)
	for _, r := range []telemetry.SpendRecord{
		// Reported turn: estimate 5.00 cr ($0.05) vs reported 4.00 AIU — drifts by +1.00.
		{At: day("2026-06-04T10:00:00Z"), Model: "gpt-5", USD: 0.05, AIU: 4},
		// Unreported turn: counted in the coverage cell, excluded from the comparison.
		{At: day("2026-06-05T10:00:00Z"), Model: "gpt-5", USD: 0.10},
	} {
		if err := store.Append(r); err != nil {
			t.Fatal(err)
		}
	}
	html := s.telemetryPartial(defaultSpendWindow)
	for _, want := range []string{
		"Estimate vs reported", // the section heading
		`class="grid drift"`,   // the comparison table
		// The full row pins the estimate to the REPORTED turn only (5.00 cr, not the
		// 15.00 cr both turns sum to — which the Total-cost row legitimately shows
		// elsewhere on the page) beside the reported figure.
		`class="recon-row drift-row"><td class="drift-model">gpt-5</td><td class="recon-ledger">5.00 cr</td><td class="recon-runs">4.00 cr</td>`,
		`class="recon-delta amber"`, // the non-trivial delta is ambered
		"1 reported · 1 est-only",   // the coverage cell
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("telemetry page missing drift %q\n%s", want, html)
		}
	}
}

func TestTelemetryPageHidesDriftWithoutReportedTurns(t *testing.T) {
	// With no reported turn there is nothing to compare — the drift section stays
	// hidden rather than showing an empty (or estimate-vs-zero) table.
	s, store := newSpendServer(t)
	if err := store.Append(telemetry.SpendRecord{
		At: day("2026-06-04T10:00:00Z"), Model: "gpt-5", USD: 0.05,
	}); err != nil {
		t.Fatal(err)
	}
	html := s.telemetryPartial(defaultSpendWindow)
	if strings.Contains(html, "Estimate vs reported") {
		t.Fatalf("drift section should be hidden when no turn was reported\n%s", html)
	}
}

func TestReconcileExportReturnsCSV(t *testing.T) {
	// The reconciliation export (V17) mirrors the spend and runs exports: a CSV
	// attachment over the cross-store join, so the ledger-vs-runs divergence can be
	// analysed outside the app. One file carries both grains — the per-workflow rows
	// then the per-(workflow, lane) rows.
	s, store := newSpendServer(t)
	rs := withRunStore(s)
	if err := store.Append(telemetry.SpendRecord{
		At: day("2026-06-05T10:00:00Z"), Model: "gpt-5", USD: 0.05, WorkflowID: "ship", LaneIndex: 0,
	}); err != nil {
		t.Fatal(err)
	}
	if err := rs.Append(telemetry.RunRecord{
		ID: "r1", WorkflowID: "ship", Name: "Ship", Mode: "sequential", Outcome: "finished",
		Lanes: []telemetry.RunLane{{Index: 0, Status: "done", Credits: 4}},
	}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/telemetry/reconcile.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("Content-Type = %q, want text/csv", ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Fatalf("Content-Disposition = %q, want attachment", cd)
	}
	body, _ := io.ReadAll(resp.Body)
	out := string(body)
	if !strings.Contains(out, "grain,workflow,lane,ledgerCredits,runCredits,delta") || !strings.Contains(out, "ship") {
		t.Fatalf("CSV body missing header or data:\n%s", out)
	}
}

func TestReconcileExportHeaderOnlyWithoutStores(t *testing.T) {
	// No stores wired (or nothing to reconcile): the export is the header alone, never a
	// 500 — mirrors the spend and runs exports' empty case.
	s, _ := newSpendServer(t) // ephemeral ledger left empty, s.runs == nil
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/telemetry/reconcile.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.HasPrefix(string(body), "grain,workflow,lane,ledgerCredits,runCredits,delta") {
		t.Fatalf("want header-only CSV, got:\n%s", body)
	}
}

func TestTelemetryPageHasReconcileExportLink(t *testing.T) {
	// The Telemetry page surfaces the reconciliation export affordance only when there is
	// a reconciliation to show (a run store wired with reconcilable spend) — a DISJOINT
	// marker class (reconcile-export) so it can't collide with the spend export's selector.
	s, store := newSpendServer(t)
	rs := withRunStore(s)
	if err := store.Append(telemetry.SpendRecord{
		At: day("2026-06-05T10:00:00Z"), Model: "gpt-5", USD: 0.05, WorkflowID: "ship", LaneIndex: 0,
	}); err != nil {
		t.Fatal(err)
	}
	if err := rs.Append(telemetry.RunRecord{
		ID: "r1", WorkflowID: "ship", Name: "Ship", Mode: "sequential", Outcome: "finished",
		Lanes: []telemetry.RunLane{{Index: 0, Status: "done", Credits: 4}},
	}); err != nil {
		t.Fatal(err)
	}
	html := s.telemetryPartial(defaultSpendWindow)
	if !strings.Contains(html, `href="/telemetry/reconcile.csv"`) {
		t.Fatalf("Telemetry page should carry the reconciliation export link:\n%s", html)
	}
}

func TestTelemetryReconcileExportLinkHiddenWithoutReconciliation(t *testing.T) {
	// No run store → no reconciliation → the export link is absent (the same gate as the
	// reconciliation tables themselves), so a chat-only build shows no dangling affordance.
	s, store := newSpendServer(t)
	if err := store.Append(telemetry.SpendRecord{
		At: day("2026-06-05T10:00:00Z"), Model: "gpt-5", USD: 0.05, WorkflowID: "ship", LaneIndex: 0,
	}); err != nil {
		t.Fatal(err)
	}
	html := s.telemetryPartial(defaultSpendWindow)
	if strings.Contains(html, "/telemetry/reconcile.csv") {
		t.Fatalf("reconciliation export link should be absent without a reconciliation:\n%s", html)
	}
}

func TestTelemetryReconciliationHiddenWithoutRunStore(t *testing.T) {
	// Reconciliation needs BOTH stores — with no run store wired there is nothing to
	// reconcile against, so the section is absent (the page keeps its prior shape).
	s, store := newSpendServer(t)
	s.forge.Workflows = []ctxforge.Workflow{{ID: "ship", Name: "Ship", Mode: ctxforge.WorkflowSequential}}
	if err := store.Append(telemetry.SpendRecord{At: day("2026-06-04T10:00:00Z"), Model: "gpt-5", USD: 0.05, WorkflowID: "ship"}); err != nil {
		t.Fatal(err)
	}
	html := s.telemetryPartial(defaultSpendWindow)
	if strings.Contains(html, "Ledger vs runs") {
		t.Fatalf("reconciliation should be absent with no run store:\n%s", html)
	}
}

func TestTelemetryReconciliationRendersWithoutSpendHistory(t *testing.T) {
	// The reconciliation must render even when the ledger has NO history yet (so the
	// "Spend over time" trend is absent) but the run store holds runs — a run-only
	// workflow is the sharpest divergence (cost recorded by a run but never reaching
	// the ledger under that id), exactly what the section exists to surface. Regression:
	// the section once lived inside the HasHistory branch and was dropped in this case.
	s, _ := newSpendServer(t) // ephemeral ledger, left empty → HasHistory false
	s.forge.Workflows = []ctxforge.Workflow{{ID: "ship", Name: "Ship", Mode: ctxforge.WorkflowSequential}}
	rs := withRunStore(s)
	if err := rs.Append(telemetry.RunRecord{
		ID: "r1", WorkflowID: "ship", Name: "Ship", Mode: "sequential", Outcome: "finished",
		Lanes: []telemetry.RunLane{{Index: 0, Status: "done", Credits: 3}},
	}); err != nil {
		t.Fatal(err)
	}
	html := s.telemetryPartial(defaultSpendWindow)
	if strings.Contains(html, "Spend over time") {
		t.Fatalf("an empty ledger should show no spend trend:\n%s", html)
	}
	// ...yet the run-only reconciliation row (ledger 0, runs 3.00, delta −3.00) renders.
	for _, want := range []string{"Ledger vs runs", `class="recon-row"`, "Ship", `class="recon-delta amber"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("run-only reconciliation should render without spend history, missing %q\n%s", want, html)
		}
	}
}

func TestNewSessionSeedsActiveAgentFromConfig(t *testing.T) {
	// A new session inherits the launch-time active agent, so its first chat turn
	// attributes spend correctly even before any /agent switch.
	hub := New(Options{
		Client: copilot.NewMockClient(),
		Forge:  &ctxforge.Forge{},
		Config: &config.Config{DefaultModel: "gpt-5", DefaultAgent: "builder"},
		Meter:  telemetry.NewMeter(telemetry.DefaultPriceBook()),
		Logger: log.New(io.Discard, "", 0),
	})
	if got := hub.newSession("seed").agentID; got != "builder" {
		t.Fatalf("session should seed the active agent from config, got %q", got)
	}
}

func TestUsageWithoutLedgerDoesNotPanic(t *testing.T) {
	s, _ := newTestServer() // no Spend wired (s.spend == nil)
	s.handleEvent(copilot.Event{Type: copilot.EvUsage, Usage: copilot.UsageData{
		Model: "gpt-5", InputTokens: 100, OutputTokens: 50,
	}})
	// Reaching here without a panic is the assertion.
}

func TestTelemetryPageShowsTrendFromLedger(t *testing.T) {
	s, store := newSpendServer(t)
	// Two days, two models, so both trend lists have content.
	for _, r := range []telemetry.SpendRecord{
		{At: day("2026-06-04T10:00:00Z"), Model: "gpt-5", USD: 0.20},
		{At: day("2026-06-05T10:00:00Z"), Model: "claude-sonnet-4-6", USD: 0.30},
	} {
		if err := store.Append(r); err != nil {
			t.Fatal(err)
		}
	}
	html := s.telemetryPartial(defaultSpendWindow)
	for _, want := range []string{"Spend history", "Spend over time", "Per-model share", "2026-06-04", "2026-06-05", "Export CSV"} {
		if !strings.Contains(html, want) {
			t.Fatalf("telemetry page missing %q\n%s", want, html)
		}
	}
}

func TestTelemetryPerModelTableIsPopulatedFromLedger(t *testing.T) {
	// THE missing-coverage test (issue 0058). The per-model token table read the
	// live in-process meter, which is empty until turns replay through it — so it
	// showed 0/0/0 next to real spend (the demo seeds the LEDGER but never replays
	// the meter; epic 0050 finding 2). The ledger already carries the token counts,
	// so this seeds ONLY the ledger (the meter stays untouched and empty) and asserts
	// the rendered table is populated from history, not 0/0/0.
	s, store := newSpendServer(t)
	for _, r := range []telemetry.SpendRecord{
		{At: day("2026-06-04T10:00:00Z"), Model: "gpt-5", InputTokens: 1000, CachedTokens: 200, OutputTokens: 300, USD: 0.50},
		{At: day("2026-06-05T10:00:00Z"), Model: "gpt-5", InputTokens: 500, CachedTokens: 100, OutputTokens: 150, USD: 0.25},
		{At: day("2026-06-05T11:00:00Z"), Model: "claude-sonnet-4-6", InputTokens: 800, CachedTokens: 0, OutputTokens: 400, USD: 0.10},
	} {
		if err := store.Append(r); err != nil {
			t.Fatal(err)
		}
	}
	// Guard: the live meter is empty (the bug's root cause) — nothing was replayed.
	if in, cached, out := s.meter.TotalTokens(); in != 0 || cached != 0 || out != 0 {
		t.Fatalf("precondition: live meter must be empty, got in=%d cached=%d out=%d", in, cached, out)
	}

	html := s.telemetryPartial(defaultSpendWindow)

	// The "all-time" per-model breakdown (from the ledger) names each model and its
	// summed token counts — the figures that read 0 before this fix.
	for _, want := range []string{
		"all-time",          // the ledger source is labelled (vs the live "this session")
		"gpt-5",             // the model row
		"claude-sonnet-4-6", // the second model row
		">1500<",            // gpt-5 input tokens summed across its two turns (1000+500)
		">300<",             // gpt-5 cached (200+100)
		">450<",             // gpt-5 output (300+150)
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("per-model breakdown should be populated from the ledger, missing %q\n%s", want, html)
		}
	}
	// The empty-table placeholder must be gone now that history populates the rows.
	if strings.Contains(html, "no usage yet — send a prompt on the Chat page") {
		t.Fatalf("per-model table still shows the empty-meter placeholder despite seeded ledger:\n%s", html)
	}
}

func TestUsagePricesAndPersistsCacheWriteAndReasoning(t *testing.T) {
	// ADR-0034: cache-write is a priced, additive category; reasoning is a subset of
	// output (already priced) tracked for display. recordUsage must price cache-write
	// into USD and persist both counts so the all-time breakdown can surface them.
	s, store := newSpendServer(t)
	s.handleEvent(copilot.Event{Type: copilot.EvUsage, Usage: copilot.UsageData{
		Model: "gpt-5", InputTokens: 1_000_000, OutputTokens: 1_000_000,
		CacheWriteTokens: 1_000_000, ReasoningTokens: 400_000,
	}})
	rec := store.Records()[0]
	if rec.CacheWriteTokens != 1_000_000 || rec.ReasoningTokens != 400_000 {
		t.Fatalf("record must persist cache-write + reasoning counts: %+v", rec)
	}
	// USD includes the cache-write charge: gpt-5 in $1.25 + out $10 + cache-write
	// $1.5625 (1.25× input) = $12.8125. Reasoning adds nothing (subset of output).
	if got := rec.USD; got < 12.81 || got > 12.82 {
		t.Fatalf("recorded USD should include cache-write (≈$12.8125), got %v", got)
	}
}

func TestTelemetryPerModelTableShowsCacheWriteAndReasoning(t *testing.T) {
	// The all-time per-model breakdown (from the ledger) gains cache-write + reasoning
	// columns (ADR-0034), summed from history.
	s, store := newSpendServer(t)
	for _, r := range []telemetry.SpendRecord{
		{At: day("2026-06-04T10:00:00Z"), Model: "gpt-5", InputTokens: 1000, OutputTokens: 300, CacheWriteTokens: 500, ReasoningTokens: 120, USD: 0.50},
		{At: day("2026-06-05T10:00:00Z"), Model: "gpt-5", InputTokens: 500, OutputTokens: 150, CacheWriteTokens: 200, ReasoningTokens: 80, USD: 0.25},
	} {
		if err := store.Append(r); err != nil {
			t.Fatal(err)
		}
	}
	html := s.telemetryPartial(defaultSpendWindow)
	for _, want := range []string{
		"cache-write", // the new column header
		"reasoning",   // the new column header
		">700<",       // cache-write summed across the two turns (500+200)
		">200<",       // reasoning summed (120+80)
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("per-model breakdown missing %q\n%s", want, html)
		}
	}
}

func TestSettingsSaveCacheWriteOverrideRepricesLive(t *testing.T) {
	// A 4-element override pins the cache-write rate (ADR-0034); it round-trips and
	// reprices the live meter — Price uses the pinned rate, not the 1.25× default.
	s, dir := newSettingsServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL+"/settings", url.Values{
		"defaultModel":   {"gpt-5"},
		"price.0.model":  {"gpt-5"},
		"price.0.in":     {"2"},
		"price.0.cached": {"0.2"},
		"price.0.out":    {"20"},
		"price.0.cw":     {"5"}, // explicit cache-write, NOT 1.25×2=2.5
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if got := s.config.Telemetry.PriceOverrides["gpt-5"]; !slices.Equal(got, []float64{2, 0.2, 20, 5}) {
		t.Errorf("4-element cache-write override not persisted: %v", got)
	}
	reloaded, _ := config.Load(dir)
	if got := reloaded.Telemetry.PriceOverrides["gpt-5"]; !slices.Equal(got, []float64{2, 0.2, 20, 5}) {
		t.Errorf("override not persisted to disk: %v", got)
	}
	// The live meter prices cache-write at the pinned $5/Mt, on both shared meters.
	if got := telemetry.Price(s.meter.PriceBook(), telemetry.Usage{Model: "gpt-5", CacheWriteTokens: 1_000_000}).CacheWriteUSD; got != 5 {
		t.Errorf("account meter cache-write not repriced: got $%v/Mt, want $5", got)
	}
}

func TestTelemetryTrendWindowsAndScalesToVisibleMax(t *testing.T) {
	s, store := newSpendServer(t)
	// 20 days of history: the all-time biggest spender is day 0 (off-screen,
	// outside the most-recent-14 window); inside the window the biggest is the
	// last day. Bars must scale to the in-window max so it fills to 100%.
	base := day("2026-05-01T10:00:00Z")
	if err := store.Append(telemetry.SpendRecord{At: base, Model: "gpt-5", USD: 99.0}); err != nil {
		t.Fatal(err)
	}
	for i := 1; i < 20; i++ {
		usd := 0.10 * float64(i) // grows toward the recent end
		if err := store.Append(telemetry.SpendRecord{At: base.AddDate(0, 0, i), Model: "gpt-5", USD: usd}); err != nil {
			t.Fatal(err)
		}
	}
	days, _, has := s.spendTrend(defaultSpendWindow)
	if !has {
		t.Fatal("expected history")
	}
	if len(days) != 14 {
		t.Fatalf("trend should window to 14 days, got %d", len(days))
	}
	// The off-screen $99 day must not appear; the busiest in-window day fills.
	for _, d := range days {
		if d["Day"] == "2026-05-01" {
			t.Fatal("the off-screen all-time-max day leaked into the window")
		}
	}
	if w := days[len(days)-1]["Width"]; w != "100.0%" {
		t.Fatalf("busiest in-window day should fill the bar, got width %v", w)
	}
}

func TestClampWindow(t *testing.T) {
	cases := map[string]int{
		"14": 14, "30": 30, "90": 90, // the allowed set
		"":        defaultSpendWindow, // no param → default
		"garbage": defaultSpendWindow, // unparseable → default
		"7":       defaultSpendWindow, // below the set → default
		"1000":    defaultSpendWindow, // above the set → default
		"-30":     defaultSpendWindow, // negative → default
		"30.5":    defaultSpendWindow, // non-integer → default
	}
	for raw, want := range cases {
		if got := clampWindow(raw); got != want {
			t.Errorf("clampWindow(%q) = %d, want %d", raw, got, want)
		}
	}
	if defaultSpendWindow != 14 {
		t.Fatalf("default window should be 14 (the historical behavior), got %d", defaultSpendWindow)
	}
}

func TestSpendTrendWidensWithWindow(t *testing.T) {
	s, store := newSpendServer(t)
	// 40 days of history: 14 < 30 < the full 40 < 90, so each wider window shows
	// strictly more (older) rows, and 90 is clamped by available history to 40.
	base := day("2026-04-01T10:00:00Z")
	for i := 0; i < 40; i++ {
		if err := store.Append(telemetry.SpendRecord{At: base.AddDate(0, 0, i), Model: "gpt-5", USD: 0.10 * float64(i+1)}); err != nil {
			t.Fatal(err)
		}
	}
	d14, _, _ := s.spendTrend(14)
	d30, _, _ := s.spendTrend(30)
	d90, _, _ := s.spendTrend(90)
	if len(d14) != 14 || len(d30) != 30 || len(d90) != 40 {
		t.Fatalf("window widths wrong: 14→%d 30→%d 90→%d (want 14/30/40)", len(d14), len(d30), len(d90))
	}
	// The widest window reaches the oldest day; the 14-day window does not.
	if d90[0]["Day"] != "2026-04-01" {
		t.Fatalf("90-day window should reach the oldest day, got %v", d90[0]["Day"])
	}
	if d14[0]["Day"] == "2026-04-01" {
		t.Fatal("the 14-day window should not reach the oldest day")
	}
	// The most-recent day is the same regardless of window (most-recent-last).
	if d14[len(d14)-1]["Day"] != d90[len(d90)-1]["Day"] {
		t.Fatal("the most-recent day should be identical across windows")
	}
}

func TestTelemetryTrendScalesToVisibleMaxPerWindow(t *testing.T) {
	// The REGRESSIONS #14 invariant must hold for EVERY window: an off-window
	// all-time peak must never shrink the visible bars — the busiest IN-WINDOW
	// day fills 100%. Seed a peak one day older than each window's left edge.
	for _, win := range spendWindows {
		win := win
		t.Run(fmt.Sprintf("window-%d", win), func(t *testing.T) {
			s, store := newSpendServer(t)
			base := day("2026-01-01T10:00:00Z")
			// A towering peak just outside the window (win+1 days back from the last).
			peakDay := base
			if err := store.Append(telemetry.SpendRecord{At: peakDay, Model: "gpt-5", USD: 999.0}); err != nil {
				t.Fatal(err)
			}
			// win days of in-window history, growing toward the recent end so the
			// last day is the in-window max.
			for i := 1; i <= win; i++ {
				if err := store.Append(telemetry.SpendRecord{At: base.AddDate(0, 0, i), Model: "gpt-5", USD: 0.10 * float64(i)}); err != nil {
					t.Fatal(err)
				}
			}
			days, _, has := s.spendTrend(win)
			if !has {
				t.Fatal("expected history")
			}
			if len(days) != win {
				t.Fatalf("expected %d in-window days, got %d", win, len(days))
			}
			for _, d := range days {
				if d["Day"] == "2026-01-01" {
					t.Fatal("the off-window all-time-max day leaked into the window")
				}
			}
			if w := days[len(days)-1]["Width"]; w != "100.0%" {
				t.Fatalf("busiest in-window day should fill the bar, got width %v", w)
			}
		})
	}
}

func TestTelemetryPageRendersWindowSelector(t *testing.T) {
	s, store := newSpendServer(t)
	for _, r := range []telemetry.SpendRecord{
		{At: day("2026-06-04T10:00:00Z"), Model: "gpt-5", USD: 0.20},
		{At: day("2026-06-05T10:00:00Z"), Model: "gpt-5", USD: 0.30},
	} {
		if err := store.Append(r); err != nil {
			t.Fatal(err)
		}
	}
	html := s.telemetryPartial(30)
	// All three windows are offered, each re-fetching the page with its param.
	for _, w := range []string{"window=14", "window=30", "window=90"} {
		if !strings.Contains(html, w) {
			t.Fatalf("window selector missing %q\n%s", w, html)
		}
	}
	// The chosen window is the active button; the others are not.
	if !strings.Contains(html, `class="window on" hx-get="/page/telemetry?window=30"`) {
		t.Fatalf("the 30-day window should be marked active\n%s", html)
	}
	if strings.Contains(html, `class="window on" hx-get="/page/telemetry?window=14"`) ||
		strings.Contains(html, `class="window on" hx-get="/page/telemetry?window=90"`) {
		t.Fatalf("only the chosen window should be active\n%s", html)
	}
}

func TestTelemetryPageEmptyHistoryNote(t *testing.T) {
	s, _ := newSpendServer(t) // ledger wired but empty
	html := s.telemetryPartial(defaultSpendWindow)
	if !strings.Contains(html, "no persisted history yet") {
		t.Fatalf("empty ledger should show the no-history note:\n%s", html)
	}
}

func TestSpendExportReturnsCSV(t *testing.T) {
	s, store := newSpendServer(t)
	if err := store.Append(telemetry.SpendRecord{At: day("2026-06-05T10:00:00Z"), Model: "gpt-5", InputTokens: 1200, USD: 0.5}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/telemetry/export.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("Content-Type = %q, want text/csv", ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Fatalf("Content-Disposition = %q, want attachment", cd)
	}
	body, _ := io.ReadAll(resp.Body)
	out := string(body)
	if !strings.Contains(out, "at,session,model") || !strings.Contains(out, "gpt-5") {
		t.Fatalf("CSV body missing header or data:\n%s", out)
	}
}
