package web

import (
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// This file adds the run inspector's KEYED comparison view (O4, issue 0094): two runs
// of ONE workflow rendered side-by-side with per-field deltas — outcome, total credits,
// duration, and per-lane credits/status/pauses, plus the two runs' final assistant
// outputs when both event logs exist. It is the Braintrust experiment-comparison pattern
// at run grain ("did the rerun do better, and what did it cost?"), the local complement
// of the Runs-page rerun. The pure delta lives in telemetry (CompareRuns); this layer
// resolves agent/workflow labels, loads the optional logs, and renders. Explicitly NOT
// a structural trace-tree diff — the industry diffs keyed runs, not span trees. — ADR-0052.

// handleRunCompare serves GET /runs/compare?a={id}&b={id}: the side-by-side comparison
// of two persisted runs. Either id unknown 404s cleanly (like the detail page); the
// active window rides along (?window=) so the back button restores the Runs selection.
// Read-only — it reads the run history (s.runs) and the per-run event logs only.
func (s *Server) handleRunCompare(w http.ResponseWriter, r *http.Request) {
	a, okA := s.findRun(r.URL.Query().Get("a"))
	b, okB := s.findRun(r.URL.Query().Get("b"))
	if !okA || !okB || a.ID == b.ID {
		// Either id unknown, or a run compared with itself (an all-zero self-diff the
		// picker never offers) — 404 rather than render a meaningless comparison.
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(s.runComparePartial(a, b, clampWindow(r.URL.Query().Get("window")))))
}

// runComparePartial renders the comparison of runs a (baseline) and b. When the two
// are runs of different workflows the comparison is refused with a note — only a keyed
// (same-workflow) pair is meaningful. The run-level and per-lane deltas come from the
// pure telemetry.CompareRuns; labels resolve under forgeMu like runRow. The two runs'
// final assistant outputs render ONLY when BOTH event logs exist — a missing log
// degrades to the summary-only comparison rather than a one-sided output. — issue 0094.
func (s *Server) runComparePartial(a, b telemetry.RunRecord, window int) string {
	d := telemetry.CompareRuns(a, b)

	s.hub.forgeMu.Lock()
	workflow := s.workflowLabel(d.WorkflowID)
	aWorkflow := s.workflowLabel(a.WorkflowID)
	bWorkflow := s.workflowLabel(b.WorkflowID)
	var lanes []map[string]any
	if d.Comparable {
		lanes = make([]map[string]any, 0, len(d.Lanes))
		for _, ld := range d.Lanes {
			lanes = append(lanes, s.compareLaneRow(ld))
		}
	}
	s.hub.forgeMu.Unlock()

	creditsDelta, creditsUp := signedCredits(d.CreditsDelta)
	durationDelta, durationUp := signedDuration(d.DurationDelta)
	pausesDelta, pausesUp := signedInt(d.PausesDelta)
	data := map[string]any{
		"Window":     window,
		"Comparable": d.Comparable,
		"Workflow":   workflow,
		"AID":        a.ID, "BID": b.ID,
		"AWorkflow": aWorkflow, "BWorkflow": bWorkflow,
		// Each side's run-level snapshot.
		"AWhen": humanWhen(a.StartedAt), "BWhen": humanWhen(b.StartedAt),
		"AOutcome": d.A.Outcome, "BOutcome": d.B.Outcome,
		"ACredits": telemetry.FormatCredits(d.A.Credits), "BCredits": telemetry.FormatCredits(d.B.Credits),
		"ADuration": humanDuration(d.A.Duration), "BDuration": humanDuration(d.B.Duration),
		"APauses": d.A.Pauses, "BPauses": d.B.Pauses,
		// The B−A run-level deltas (signed, so the direction of change reads at a glance).
		// String and "up" flag come from one helper so they can never disagree.
		"CreditsDelta": creditsDelta, "CreditsUp": creditsUp,
		"DurationDelta": durationDelta, "DurationUp": durationUp,
		"PausesDelta": pausesDelta, "PausesUp": pausesUp,
		"Lanes": lanes,
	}

	// Final outputs: only when BOTH logs exist (issue 0094). A missing/absent log on
	// either side degrades the view to the summary comparison — never a one-sided output.
	if d.Comparable {
		aOut, aOK := s.finalRunOutput(a.ID)
		bOut, bOK := s.finalRunOutput(b.ID)
		if aOK && bOK {
			data["HasOutputs"] = true
			data["AOutput"] = aOut
			data["BOutput"] = bOut
		}
	}
	return frag("runComparePage", data)
}

// compareLaneRow projects one aligned LaneDelta into the template shape: the step (lane
// index + 1), each side's agent/status/credits (or a "—" gap when the run had no lane at
// that index), and the B−A credits/pauses deltas. Agent ids resolve via agentLabel —
// caller holds forgeMu.
func (s *Server) compareLaneRow(ld telemetry.LaneDelta) map[string]any {
	creditsDelta, creditsUp := signedCredits(ld.CreditsDelta)
	pausesDelta, _ := signedInt(ld.PausesDelta)
	return map[string]any{
		"Step":     ld.Index + 1,
		"APresent": ld.A.Present, "BPresent": ld.B.Present,
		"AAgent": s.laneAgent(ld.A), "BAgent": s.laneAgent(ld.B),
		"AStatus": ld.A.Status, "BStatus": ld.B.Status,
		"ACredits": telemetry.FormatCredits(ld.A.Credits), "BCredits": telemetry.FormatCredits(ld.B.Credits),
		"CreditsDelta": creditsDelta, "CreditsUp": creditsUp,
		"PausesDelta": pausesDelta, "HasPauseDelta": ld.PausesDelta != 0,
	}
}

// laneAgent resolves a lane side's agent label, or "—" for an absent side (the run had
// no lane at that index). Caller holds forgeMu.
func (s *Server) laneAgent(ls telemetry.LaneSide) string {
	if !ls.Present {
		return "—"
	}
	return s.agentLabel(ls.AgentID)
}

// finalRunOutput returns a run's final assistant output, reconstructed from its event
// log, and whether a log was available. ok is false when the event log is absent, empty,
// or unreadable — the caller uses it to gate the both-logs-present output rendering. The
// "final output" is the run's last committed assistant message (the EvMessage the run
// ended on), the figure a keyed comparison pits against the other run's.
func (s *Server) finalRunOutput(runID string) (string, bool) {
	if s.eventLogDir == "" {
		return "", false
	}
	log, err := telemetry.LoadRunEventLog(s.eventLogDir, runID)
	if err != nil || log.Count() == 0 {
		return "", false
	}
	return finalAssistantOutput(log.Records()), true
}

// finalAssistantOutput returns the text of the LAST committed assistant message in an
// event log — the run's final output. Streaming deltas and every non-message event are
// ignored; an empty result (a run that ended without an assistant message) is still a
// valid "output" (the caller already gated on the log existing). Pure: same events →
// same output.
func finalAssistantOutput(events []telemetry.RunEvent) string {
	var last string
	for _, e := range events {
		if e.Type == "EvMessage" {
			last = e.Text
		}
	}
	return last
}

// signedCredits renders a credit delta with an explicit sign for the compare view and
// reports whether it ROSE (the "cost went up" affordance): "+1.50 cr" + true when B cost
// more than A, a leading "-" + false when less, "0.00 cr" + false when equal. A sub-cent
// delta (float residue from summing lane credits) is treated as equal via reconcileEpsilon
// — the same threshold the V15 reconciliation uses — so the displayed magnitude and the
// "up" flag can never disagree (no "+0.00 cr" coloured as a rise).
func signedCredits(v float64) (string, bool) {
	if math.Abs(v) < reconcileEpsilon {
		return telemetry.FormatCredits(0), false
	}
	if v > 0 {
		return "+" + telemetry.FormatCredits(v), true
	}
	return telemetry.FormatCredits(v), false // FormatCredits already carries the "-"
}

// signedDuration renders a duration delta with an explicit sign and reports whether it
// rose: "+2m" + true when B took longer, "-30s" + false when shorter, "0s" + false when
// equal. A span that rounds to zero (humanDuration renders "" for ≤0 and "0s" for a
// sub-second tick) collapses to a clean "0s" with no sign — so a negligible difference
// never shows a signed "0s" coloured as a change.
func signedDuration(d time.Duration) (string, bool) {
	mag := humanDuration(absDuration(d))
	if mag == "" || mag == "0s" {
		return "0s", false
	}
	if d > 0 {
		return "+" + mag, true
	}
	return "-" + mag, false
}

// absDuration is |d| — the magnitude signedDuration formats before re-attaching the sign.
func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// signedInt renders an integer delta with an explicit "+" for positive values (a
// negative already carries its sign) and reports whether it rose — the pause-count delta
// in the compare view. Exact integers, so no epsilon is needed.
func signedInt(n int) (string, bool) {
	if n > 0 {
		return "+" + strconv.Itoa(n), true
	}
	return strconv.Itoa(n), false
}

// compareCandidates lists the OTHER persisted runs of rec's workflow — the runs rec can
// be compared against — newest first, excluding rec itself and any run of a different
// workflow (a cross-workflow comparison is refused, so it is never offered). Each
// candidate carries the id and a short label (outcome · when · cost) for the run-detail
// picker. Caller need not hold forgeMu (it reads only the run records). Capped so a long
// history can't balloon the picker.
func (s *Server) compareCandidates(rec telemetry.RunRecord) []map[string]any {
	if s.runs == nil || rec.ID == "" {
		return nil
	}
	const maxCandidates = 12
	recs := s.runs.Records()
	out := []map[string]any{}
	for i := len(recs) - 1; i >= 0; i-- { // newest first
		c := recs[i]
		if c.ID == "" || c.ID == rec.ID || c.WorkflowID != rec.WorkflowID {
			continue
		}
		out = append(out, map[string]any{
			"ID": c.ID, "Label": compareCandidateLabel(c),
		})
		if len(out) >= maxCandidates {
			break
		}
	}
	return out
}

// compareCandidateLabel names a compare candidate compactly: its outcome, when it ran,
// and its total metered cost — enough to tell two runs of one workflow apart in the
// picker even when they share an outcome (the cost/time distinguishes a costly rerun
// from a cheap one).
func compareCandidateLabel(r telemetry.RunRecord) string {
	label := r.Outcome
	if w := humanWhen(r.StartedAt); w != "" {
		label += " · " + w
	}
	return label + " · " + telemetry.FormatCredits(r.Credits())
}
