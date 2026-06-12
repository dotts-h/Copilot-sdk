package telemetry

import (
	"fmt"
	"io"
	"sort"
	"time"
)

// This file is the spend digest (issue 0098, roadmap v16 P3): a period's cost story
// rolled into one summary, the *delivery* layer that pairs the P1 per-run cap
// (ADR-0053) and the P2 anomaly signal (anomaly.go) with the converged FinOps-for-AI
// control set — cap + anomaly + digest. It answers "what did this window cost, where
// did it go, and was anything off?" in one projection, so a slow burn surfaces on a
// glance instead of only when watched live. A pure reader over the two persisted
// stores (ledger + run history) — no IO, no schema change, no scheduling (the digest
// is on-demand over the existing window selector; a scheduled/recurring delivery is
// the v17 direction, deliberately out of scope).

// DigestOpts configures BuildSpendDigest. Since bounds the period (records/runs at or
// after it are included; the zero value means all-time). Anomaly is how cost anomalies
// in the period are flagged; TopN caps the per-workflow/agent share lines (0 = all).
type DigestOpts struct {
	Since   time.Time
	Anomaly AnomalyOpts
	TopN    int
}

// DigestShare is one labeled slice of the period's spend — a workflow or an agent and
// what it cost.
type DigestShare struct {
	Key     string // workflow id or agent id ("" = unattributed / the built-in chat agent)
	Credits float64
	USD     float64
	Turns   int
}

// SpendDigest is the period's cost summary: the totals, the biggest spenders by
// workflow and by agent, the run activity, and the cost anomalies that occurred in
// the period.
type SpendDigest struct {
	Since        time.Time
	TotalCredits float64
	TotalUSD     float64
	Turns        int
	Runs         int
	FailedRuns   int           // in-window runs whose outcome was not "finished" (a stopped/failed run)
	Workflows    []DigestShare // biggest first
	Agents       []DigestShare // biggest first; sub-agent spend excluded (no double count)
	Anomalies    []Anomaly     // in-period runs flagged out of band (baseline over full history)
}

// BuildSpendDigest rolls the ledger and run history into one period summary. Pure and
// deterministic. The window is opts.Since (zero = all-time): only records/runs at or
// after it are totaled. Per-workflow/agent shares rank by credits (biggest first),
// capped at opts.TopN; agent shares exclude sub-agent-tagged turns, mirroring
// AgentShares (a sub-agent's spend is the sub-agent's). Anomalies are detected over
// the FULL run history (a stable cohort baseline) but reported only for in-window runs.
func BuildSpendDigest(spend []SpendRecord, runs []RunRecord, opts DigestOpts) SpendDigest {
	d := SpendDigest{Since: opts.Since}

	byWorkflow := map[string]*DigestShare{}
	byAgent := map[string]*DigestShare{}
	for _, r := range spend {
		if !inWindow(r.At, opts.Since) {
			continue
		}
		d.Turns++
		d.TotalUSD += r.USD
		d.TotalCredits += r.Credits()
		addShare(byWorkflow, r.WorkflowID, r)
		// A sub-agent's spend is the sub-agent's, not the spawning agent's — exclude it
		// from the agent buckets so the per-agent rollup never double-counts (the
		// AgentShares discipline, ADR-0018).
		if r.SubagentID == "" {
			addShare(byAgent, r.AgentID, r)
		}
	}
	d.Workflows = topShares(byWorkflow, opts.TopN)
	d.Agents = topShares(byAgent, opts.TopN)

	inWindowRun := map[string]bool{}
	for _, r := range runs {
		if !inWindow(r.StartedAt, opts.Since) {
			continue
		}
		inWindowRun[r.ID] = true
		d.Runs++
		if r.Outcome != "finished" {
			d.FailedRuns++
		}
	}
	// Detect over full history so a short window still has a cohort to baseline against,
	// then keep only the anomalies whose run falls in the period.
	for _, a := range DetectAnomalies(runs, opts.Anomaly) {
		if inWindowRun[a.RunID] {
			d.Anomalies = append(d.Anomalies, a)
		}
	}
	return d
}

// WriteDigest renders a SpendDigest as a plain-markdown artifact — the written
// delivery of the period summary (the "and/or written artifact" of issue 0098), so a
// digest can be saved or pasted outside the app. Keys are raw workflow/agent ids (like
// the CSV exports); the HTML surface resolves display names. Returns the first write
// error.
func WriteDigest(w io.Writer, d SpendDigest) error {
	period := "all time"
	if !d.Since.IsZero() {
		period = "since " + d.Since.Format("2006-01-02")
	}
	var b []byte
	b = append(b, "# Spend digest\n\n"...)
	b = append(b, fmt.Sprintf("Period: %s\n", period)...)
	b = append(b, fmt.Sprintf("Total: %s (%s) over %d turn%s\n",
		FormatCredits(d.TotalCredits), FormatUSD(d.TotalUSD), d.Turns, plural(d.Turns))...)
	b = append(b, fmt.Sprintf("Runs: %d (%d stopped/failed)\n", d.Runs, d.FailedRuns)...)
	b = append(b, fmt.Sprintf("Anomalies: %d\n", len(d.Anomalies))...)

	b = appendShares(b, "Top workflows", d.Workflows)
	b = appendShares(b, "Top agents", d.Agents)
	if len(d.Anomalies) > 0 {
		b = append(b, "\n## Cost anomalies\n"...)
		for _, a := range d.Anomalies {
			b = append(b, fmt.Sprintf("- %s (%s): %s — %.1f× the %s baseline\n",
				a.RunID, labelOrDash(a.WorkflowID), FormatCredits(a.Credits), a.Factor, FormatCredits(a.Baseline))...)
		}
	}
	_, err := w.Write(b)
	return err
}

func appendShares(b []byte, heading string, shares []DigestShare) []byte {
	if len(shares) == 0 {
		return b
	}
	b = append(b, "\n## "+heading+"\n"...)
	for _, s := range shares {
		b = append(b, fmt.Sprintf("- %s: %s (%s, %d turn%s)\n",
			labelOrDash(s.Key), FormatCredits(s.Credits), FormatUSD(s.USD), s.Turns, plural(s.Turns))...)
	}
	return b
}

func labelOrDash(key string) string {
	if key == "" {
		return "—"
	}
	return key
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// inWindow reports whether t is at or after since; a zero since means all-time.
func inWindow(t, since time.Time) bool {
	return since.IsZero() || !t.Before(since)
}

// addShare folds one record into its key's running share, creating the bucket on first
// sight.
func addShare(buckets map[string]*DigestShare, key string, r SpendRecord) {
	s := buckets[key]
	if s == nil {
		s = &DigestShare{Key: key}
		buckets[key] = s
	}
	s.Credits += r.Credits()
	s.USD += r.USD
	s.Turns++
}

// topShares flattens the buckets, sorts them biggest-credits-first (ties broken by key
// for determinism), and caps to top (0 = all).
func topShares(buckets map[string]*DigestShare, top int) []DigestShare {
	out := make([]DigestShare, 0, len(buckets))
	for _, s := range buckets {
		out = append(out, *s)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Credits != out[j].Credits {
			return out[i].Credits > out[j].Credits
		}
		return out[i].Key < out[j].Key
	})
	if top > 0 && len(out) > top {
		out = out[:top]
	}
	return out
}
