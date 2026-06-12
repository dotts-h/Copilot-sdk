package telemetry

import (
	"strings"
	"testing"
	"time"
)

func atDays(base time.Time, d int) time.Time { return base.AddDate(0, 0, d) }

// spend builds a one-turn SpendRecord at time t for the given workflow/agent.
func spendAt(t time.Time, usd float64, workflow, agent, subagent string) SpendRecord {
	return SpendRecord{At: t, USD: usd, WorkflowID: workflow, AgentID: agent, SubagentID: subagent}
}

// BuildSpendDigest totals only the records in the window, and the per-workflow /
// per-agent shares sum the same windowed spend.
func TestBuildSpendDigestTotalsAndShares(t *testing.T) {
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	since := atDays(now, -14)
	spend := []SpendRecord{
		spendAt(atDays(now, -1), 1.00, "wfA", "ag1", ""),
		spendAt(atDays(now, -2), 2.00, "wfA", "ag1", ""),
		spendAt(atDays(now, -3), 5.00, "wfB", "ag2", ""),
		spendAt(atDays(now, -40), 99.0, "wfB", "ag2", ""), // outside the window — excluded
	}
	d := BuildSpendDigest(spend, nil, DigestOpts{Since: since})

	if d.Turns != 3 {
		t.Errorf("only in-window turns count, got %d", d.Turns)
	}
	if d.TotalUSD != 8.00 {
		t.Errorf("total USD should sum the window (1+2+5), got %v", d.TotalUSD)
	}
	if len(d.Workflows) != 2 || d.Workflows[0].Key != "wfB" {
		t.Fatalf("workflows ranked by credits desc (wfB=5 first), got %+v", d.Workflows)
	}
	if d.Workflows[1].Key != "wfA" || d.Workflows[1].USD != 3.00 {
		t.Errorf("wfA should total its two turns (3 USD), got %+v", d.Workflows[1])
	}
}

// A sub-agent-tagged record is excluded from the per-agent shares (its spend is the
// sub-agent's, not the spawning agent's — no double count), mirroring AgentShares.
func TestBuildSpendDigestExcludesSubagentFromAgents(t *testing.T) {
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	spend := []SpendRecord{
		spendAt(atDays(now, -1), 3.00, "wf", "ag1", ""),
		spendAt(atDays(now, -1), 9.00, "wf", "ag1", "sub-7"), // sub-agent turn
	}
	d := BuildSpendDigest(spend, nil, DigestOpts{}) // Since zero = all-time

	if len(d.Agents) != 1 || d.Agents[0].Key != "ag1" {
		t.Fatalf("one agent bucket, got %+v", d.Agents)
	}
	if d.Agents[0].USD != 3.00 {
		t.Errorf("the agent bucket excludes the sub-agent turn (3, not 12), got %v", d.Agents[0].USD)
	}
}

// TopN caps the number of share lines (the worst spenders), so the digest stays a
// summary rather than the full breakdown.
func TestBuildSpendDigestTopN(t *testing.T) {
	now := time.Now()
	spend := []SpendRecord{
		spendAt(now, 1, "a", "", ""), spendAt(now, 2, "b", "", ""),
		spendAt(now, 3, "c", "", ""), spendAt(now, 4, "d", "", ""),
	}
	d := BuildSpendDigest(spend, nil, DigestOpts{TopN: 2})
	if len(d.Workflows) != 2 {
		t.Fatalf("TopN should cap to 2 workflows, got %d", len(d.Workflows))
	}
	if d.Workflows[0].Key != "d" || d.Workflows[1].Key != "c" {
		t.Errorf("the two biggest spenders survive (d=4, c=3), got %+v", d.Workflows)
	}
}

// Run activity counts the in-window runs and the stopped/failed ones; anomalies are
// detected over full history (a stable baseline) but reported only for in-window runs.
func TestBuildSpendDigestRunsAndAnomalies(t *testing.T) {
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	since := atDays(now, -14)
	mk := func(id string, started time.Time, outcome string, credits float64) RunRecord {
		return RunRecord{ID: id, WorkflowID: "wf", Name: "wf", StartedAt: started, Outcome: outcome,
			Lanes: []RunLane{{Index: 0, Credits: credits}}}
	}
	runs := []RunRecord{
		mk("h1", atDays(now, -3), "finished", 1),
		mk("h2", atDays(now, -2), "finished", 1),
		mk("h3", atDays(now, -1), "finished", 1),
		mk("bad", atDays(now, -1), "failed", 1),
		mk("runaway", atDays(now, -1), "finished", 12), // in-window anomaly
		mk("old", atDays(now, -40), "finished", 99),    // outside window — not reported, but in baseline
	}
	d := BuildSpendDigest(nil, runs, DigestOpts{Since: since, Anomaly: DefaultAnomalyOpts()})

	if d.Runs != 5 {
		t.Errorf("in-window runs (excludes the -40d one), got %d", d.Runs)
	}
	if d.FailedRuns != 1 {
		t.Errorf("one stopped/failed run, got %d", d.FailedRuns)
	}
	if len(d.Anomalies) != 1 || d.Anomalies[0].RunID != "runaway" {
		t.Fatalf("the in-window runaway is the one reported anomaly, got %+v", d.Anomalies)
	}
}

// WriteDigest renders the summary, the shares, and the anomalies as plain markdown.
func TestWriteDigest(t *testing.T) {
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	since := atDays(now, -14)
	spend := []SpendRecord{spendAt(atDays(now, -1), 3.00, "wf", "ag1", "")}
	runs := []RunRecord{
		{ID: "h1", WorkflowID: "wf", StartedAt: atDays(now, -3), Outcome: "finished", Lanes: []RunLane{{Credits: 1}}},
		{ID: "h2", WorkflowID: "wf", StartedAt: atDays(now, -2), Outcome: "finished", Lanes: []RunLane{{Credits: 1}}},
		{ID: "h3", WorkflowID: "wf", StartedAt: atDays(now, -1), Outcome: "finished", Lanes: []RunLane{{Credits: 1}}},
		{ID: "runaway", WorkflowID: "wf", StartedAt: atDays(now, -1), Outcome: "finished", Lanes: []RunLane{{Credits: 9}}},
	}
	d := BuildSpendDigest(spend, runs, DigestOpts{Since: since, Anomaly: DefaultAnomalyOpts()})

	var sb strings.Builder
	if err := WriteDigest(&sb, d); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	for _, want := range []string{"# Spend digest", "since 2026-05-29", "Top workflows", "Cost anomalies", "runaway"} {
		if !strings.Contains(out, want) {
			t.Errorf("digest artifact missing %q:\n%s", want, out)
		}
	}
}

// An all-time digest (zero Since) includes every record.
func TestBuildSpendDigestAllTime(t *testing.T) {
	old := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	d := BuildSpendDigest([]SpendRecord{spendAt(old, 5, "wf", "ag", "")}, nil, DigestOpts{})
	if d.Turns != 1 || d.TotalUSD != 5 {
		t.Errorf("a zero Since includes all history, got turns=%d usd=%v", d.Turns, d.TotalUSD)
	}
}
