package web

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// This file renders the Runs page: the persisted workflow-run history (ADR-0022). A
// run is the product's unit of orchestration, recorded once on completion by
// workflow.go (recordRun). The page is a read-only view over telemetry.RunStore,
// resolving agent ids to display names like the Telemetry cost breakdowns.

// runsPartial renders the Runs page: a per-workflow summary table (run count,
// failure rate, total & average cost, average duration — the cost ⋈ orchestration
// roll-up, ADR-0022/V1) above the persisted run history, most recent first. Each run shows
// its name, mode, outcome, when it ran, how long it took, total metered cost, and a
// per-lane breakdown (agent, status glyph, credits). Both the run rows and the
// summary resolve ids to display names under forgeMu, like the cost breakdowns.
//
// The history is first sliced to the chosen 14/30/90-day window (V12) — the
// orchestration analog of the Telemetry trend's window selector — so a long history
// stays scannable; the window is clamped upstream (clampWindow) to spendWindows. The
// slice happens BEFORE both the summary roll-up and the history list, so an out-of-window
// run is dropped from both. Empty when no run store is wired or none have run yet.
func (s *Server) runsPartial(window int) string {
	rows := []map[string]any{}
	summary := []map[string]any{}
	laneShares := []map[string]any{}
	if s.runs != nil {
		records := windowRuns(s.runs.Records(), window)
		s.hub.forgeMu.Lock()
		for i := len(records) - 1; i >= 0; i-- { // newest first
			rows = append(rows, s.runRow(records[i], window))
		}
		for _, a := range telemetry.RunAggregates(records) {
			summary = append(summary, s.runSummaryRow(a))
		}
		for _, l := range telemetry.LaneShares(records) {
			laneShares = append(laneShares, s.laneShareRow(l))
		}
		s.hub.forgeMu.Unlock()
	}
	windows := make([]map[string]any, 0, len(spendWindows))
	for _, w := range spendWindows {
		windows = append(windows, map[string]any{"Value": w, "Active": w == window})
	}
	return frag("runsPage", map[string]any{
		"Rows": rows, "Summary": summary, "LaneShares": laneShares, "Windows": windows,
	})
}

// windowRuns slices a run history to the records started within `window` days of the
// most recent run — the Runs-page time-window selector (V12). The cutoff is anchored to
// the latest StartedAt rather than wall-clock now, so the slice is deterministic and a
// long-idle history still shows its most recent window — mirroring spendTrend's
// tail-relative day slice. A non-positive window, an empty history, or a history with no
// usable timestamp (all-zero StartedAt) returns the records unchanged. Pure: same inputs
// → same slice.
func windowRuns(records []telemetry.RunRecord, window int) []telemetry.RunRecord {
	if window <= 0 || len(records) == 0 {
		return records
	}
	var latest time.Time
	for _, r := range records {
		if r.StartedAt.After(latest) {
			latest = r.StartedAt
		}
	}
	if latest.IsZero() {
		return records
	}
	cutoff := latest.AddDate(0, 0, -window)
	out := make([]telemetry.RunRecord, 0, len(records))
	for _, r := range records {
		if !r.StartedAt.Before(cutoff) {
			out = append(out, r)
		}
	}
	return out
}

// runSummaryRow builds the template shape for one per-workflow summary row: the
// workflow's display name, its run/failure counts, failure rate as a percentage,
// and its total & average cost and duration. Total credits — the workflow's
// cumulative orchestrated spend — reads beside the average so a high run count and a
// high per-run cost are distinguishable (V13). Workflow ids resolve to names via
// workflowLabel — caller holds forgeMu.
func (s *Server) runSummaryRow(a telemetry.RunAggregate) map[string]any {
	return map[string]any{
		"Workflow":     s.workflowLabel(a.WorkflowID),
		"Runs":         a.Runs,
		"Failures":     a.Failures,
		"FailPct":      fmt.Sprintf("%.0f", a.FailureRate()*100),
		"HasFailures":  a.Failures > 0,
		"TotalCredits": telemetry.FormatCredits(a.TotalCredits),
		"AvgCredits":   telemetry.FormatCredits(a.AvgCredits),
		"AvgDuration":  humanDuration(a.AvgDuration),
	}
}

// laneShareRow builds the template shape for one "Cost by lane" breakdown row (V14):
// the finest orchestration-attribution grain — which lane in a workflow costs / fails
// most. The lane is named by its workflow, its step (lane index + 1), and its agent;
// the workflow and agent ids resolve to display names via workflowLabel/agentLabel —
// caller holds forgeMu. Pct/Width render the lane's credit fraction as a percentage
// and a meter bar, mirroring the Telemetry cost-share rows.
func (s *Server) laneShareRow(l telemetry.LaneShare) map[string]any {
	return map[string]any{
		"Workflow":    s.workflowLabel(l.WorkflowID),
		"Step":        l.LaneIndex + 1,
		"Agent":       s.agentLabel(l.AgentID),
		"Failures":    l.Failures,
		"HasFailures": l.Failures > 0,
		"Credits":     fmt.Sprintf("%.2f", l.Credits),
		"Pct":         fmt.Sprintf("%.0f", l.Fraction*100),
		"Width":       fmt.Sprintf("%.1f%%", l.Fraction*100),
	}
}

// runRow builds the template shape for one persisted run: its header (name, mode,
// outcome glyph, when, total cost), a per-lane breakdown, and a "Rerun" control. Agent
// ids resolve to display names via agentLabel — caller holds forgeMu.
//
// CanRerun gates the rerun button on the run's workflow still existing in the forge
// (ADR-0023): an orphan run — one whose workflow was renamed or deleted since — has
// nothing to re-execute, so it shows no control. The active window rides along so the
// button's POST carries it, preserving the selection if the rerun is refused (a
// just-deleted workflow or a busy server) and the Runs page re-renders.
func (s *Server) runRow(r telemetry.RunRecord, window int) map[string]any {
	glyph, state := runOutcomeGlyph(r.Outcome)
	lanes := make([]map[string]any, len(r.Lanes))
	for i, l := range r.Lanes {
		lg, ls := glyphFor(l.Status)
		lanes[i] = map[string]any{
			"Step": l.Index + 1, "Agent": s.agentLabel(l.AgentID),
			"Glyph": lg, "State": ls, "Status": l.Status,
			"Credits": telemetry.FormatCredits(l.Credits), "HasCredits": l.Credits > 0,
			// Attention attribution (S6): a lane that parked on a human shows its
			// pause count and the time it waited — where humans were the bottleneck.
			// PausedFor drops a sub-second span (humanDuration rounds it to "0s"): the
			// "⏸ N" count still shows, but "· 0s" would read as broken.
			"HasPauses": l.Pauses > 0, "Pauses": l.Pauses,
			"PausedFor": pausedFor(l.PausedMs),
		}
	}
	credits := r.Credits()
	dur := r.Duration()
	return map[string]any{
		"ID":   r.ID, // links the row header to its read-only step-timeline detail page (ADR-0052)
		"Name": r.Name, "Mode": r.Mode, "Glyph": glyph, "State": state, "Outcome": r.Outcome,
		"When": humanWhen(r.StartedAt), "Lanes": lanes,
		"Duration": humanDuration(dur), "HasDuration": dur > 0,
		"Credits": telemetry.FormatCredits(credits), "HasCredits": credits > 0,
		"WorkflowID": r.WorkflowID, "CanRerun": s.forge != nil && s.forge.Workflow(r.WorkflowID) != nil,
		"Window": window,
	}
}

// pausedFor renders a lane's total paused wall-clock for the Runs indicator, "" for a
// sub-second span — humanDuration rounds <1s to "0s", which beside the "⏸ N" count
// would read as broken; an empty string lets the template omit the "· …" part so a
// brief pause shows just its count. Pure.
func pausedFor(ms int64) string {
	d := humanDuration(time.Duration(ms) * time.Millisecond)
	if d == "0s" {
		return ""
	}
	return d
}

// humanDuration renders a run's wall-clock span compactly: "" for zero (an
// unfinished/zero-span run, so the cell stays empty), seconds under a minute,
// "Nm Ss" under an hour, "Nh Nm" beyond. Seconds/minutes are dropped when zero so
// a clean span reads "2m", not "2m 0s". The span is rounded to a clean resolution
// up front — to the second below an hour, the minute above — then decomposed, so a
// fractional unit that rounds up to 60 carries into the next unit (59.6s → "1m",
// not "60s") rather than printing an impossible "60s"/"1m 60s".
func humanDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	if d < time.Hour {
		d = d.Round(time.Second)
	} else {
		d = d.Round(time.Minute)
	}
	switch {
	case d < time.Minute:
		return strconv.Itoa(int(d/time.Second)) + "s"
	case d < time.Hour:
		m := d / time.Minute
		if sec := (d % time.Minute) / time.Second; sec != 0 {
			return strconv.Itoa(int(m)) + "m " + strconv.Itoa(int(sec)) + "s"
		}
		return strconv.Itoa(int(m)) + "m"
	default:
		h := d / time.Hour
		if m := (d % time.Hour) / time.Minute; m != 0 {
			return strconv.Itoa(int(h)) + "h " + strconv.Itoa(int(m)) + "m"
		}
		return strconv.Itoa(int(h)) + "h"
	}
}

// runOutcomeGlyph maps a run's overall outcome to its glyph and CSS state class.
func runOutcomeGlyph(outcome string) (glyph, state string) {
	if outcome == "failed" {
		return "✗", "failed"
	}
	return "✓", "done"
}

// --- Run inspector: read-only step timeline over the per-run event log (ADR-0052) ---

// runStep is one rendered row in the run-detail step timeline: a load-bearing
// event reconstructed from the per-run event log. Type is the CSS suffix and the
// step kind; Glyph/State drive the (aria-hidden) status mark; Label is the primary
// text (a tool name, or the event-kind label). The disclosure body is split into
// Text (message/reasoning/error/compaction summary), Args (tool input) and Result
// (tool output) — whichever are present render in the <details> pane. All strings
// are escaped by the template (ADR-0001).
type runStep struct {
	Type   string
	Glyph  string
	State  string
	Label  string
	Clock  string
	Text   string
	Args   string
	Result string
}

// laneSteps is one lane's slice of the timeline: its lane index, the ordered steps
// attributed to it, and Credits — the lane's per-step usage rolled up (the summed
// credits of its EvUsage turns, priced at time of use; ADR-0048/issue 0092). Credits
// is zero for a pre-O2 log (no priced usage events). LaneIndex -1 is the run-level
// (unattributed) group.
type laneSteps struct {
	LaneIndex int
	Steps     []runStep
	Credits   float64
}

// maxStepDetailLines bounds a step's disclosure body (tool args/result, text) so a
// huge command output can't balloon the page — the inspector analog of the live
// timeline's maxToolResultLines.
const maxStepDetailLines = 200

// buildRunTimeline reconstructs a run's lane-grouped step timeline from its event
// log (ADR-0052). It is PURE: same events → same timeline, no IO, no locking.
//
//   - Streaming deltas (EvMessageDelta/EvReasoningDelta/EvToolProgress) and the
//     non-load-bearing lifecycle events (EvIdle/EvUsage/…) are coalesced away — the
//     committed event is the record.
//   - EvToolStart+EvToolEnd join into ONE tool step (args from the start, result +
//     success from the end). The persisted record carries no call id, so the join
//     matches each end to the most recent still-open start IN THE SAME LANE,
//     preferring an equal tool name; an unmatched start renders running with no
//     result, an unmatched end renders on its own.
//   - Steps group by lane, ordered by lane index ascending, with the run-level
//     (-1, unattributed) group last.
func buildRunTimeline(events []telemetry.RunEvent) []laneSteps {
	order := []int{}               // lane indices, first-seen order
	groups := map[int]*laneSteps{} // lane index → its steps
	openTools := map[int][]int{}   // lane index → stack of open tool-step positions
	lane := func(idx int) *laneSteps {
		g, ok := groups[idx]
		if !ok {
			g = &laneSteps{LaneIndex: idx}
			groups[idx] = g
			order = append(order, idx)
		}
		return g
	}

	for _, e := range events {
		switch e.Type {
		case "EvToolStart":
			g := lane(e.LaneIndex)
			g.Steps = append(g.Steps, runStep{
				Type: "tool", Glyph: "●", State: "running",
				Label: def(e.Tool, "tool"), Clock: clockTime(e.At), Args: e.Args,
			})
			openTools[e.LaneIndex] = append(openTools[e.LaneIndex], len(g.Steps)-1)
		case "EvToolEnd":
			g := lane(e.LaneIndex)
			if pos, ok := popOpenTool(openTools, e.LaneIndex, def(e.Tool, "tool"), g.Steps); ok {
				st := &g.Steps[pos]
				st.Result = e.Result
				st.Glyph, st.State = toolEndGlyph(e.Success)
				continue
			}
			// Unmatched end (no recorded start) — render it on its own.
			glyph, state := toolEndGlyph(e.Success)
			g.Steps = append(g.Steps, runStep{
				Type: "tool", Glyph: glyph, State: state,
				Label: def(e.Tool, "tool"), Clock: clockTime(e.At), Result: e.Result,
			})
		case "EvUsage":
			// Usage is coalesced away as a step (ADR-0052) but its priced credits roll
			// into the owning lane's subtotal — the O2 per-step pricing grain (issue
			// 0092). A lane that ONLY metered (no visible step) still gets a group so its
			// spend is attributable.
			lane(e.LaneIndex).Credits += e.Credits
		default:
			if st, ok := loadBearingStep(e); ok {
				g := lane(e.LaneIndex)
				g.Steps = append(g.Steps, st)
			}
		}
	}

	out := make([]laneSteps, 0, len(order))
	for _, idx := range sortLaneIndices(order) {
		out = append(out, *groups[idx])
	}
	return out
}

// loadBearingStep maps a non-tool event to its timeline step, or returns false for
// a coalesced-away event (the streaming deltas and the non-load-bearing lifecycle
// events). The load-bearing whitelist is the ADR-0052 step vocabulary.
func loadBearingStep(e telemetry.RunEvent) (runStep, bool) {
	base := runStep{Clock: clockTime(e.At)}
	switch e.Type {
	case "EvUserMessage":
		base.Type, base.Glyph, base.Label, base.Text = "user", "❯", "User message", e.Text
	case "EvMessage":
		base.Type, base.Glyph, base.Label, base.Text = "message", "✎", "Message", e.Text
	case "EvReasoning":
		base.Type, base.Glyph, base.Label, base.Text = "reasoning", "✻", "Reasoning", e.Text
	case "EvPermission":
		base.Type, base.Glyph, base.Label = "permission", "⚑", "Permission request"
	case "EvToolDecision":
		base.Type, base.Glyph, base.Label = "decision", "⚖", "Tool decision"
	case "EvError":
		base.Type, base.Glyph, base.State, base.Label, base.Text = "error", "✗", "failed", "Error", e.Err
	case "EvSubagentStart":
		base.Type, base.Glyph, base.Label = "subagent", "⊕", "Sub-agent started"
	case "EvSubagentEnd":
		base.Type, base.Glyph, base.Label = "subagent", "⊖", "Sub-agent finished"
	case "EvCompactionStart":
		base.Type, base.Glyph, base.Label = "compaction", "⊞", "Compaction started"
	case "EvCompactionEnd":
		base.Type, base.Glyph, base.Label, base.Text = "compaction", "⊟", "Compaction finished", e.Text
	default:
		return runStep{}, false // a delta / progress / idle / usage / … — coalesced away
	}
	return base, true
}

// popOpenTool removes and returns the position of the still-open tool step in lane
// idx that an EvToolEnd joins to: the most recent open start whose label matches
// name (the persisted record carries no call id — ADR-0052). Returns false when no
// open start in the lane has that name, so the caller renders the end standalone
// rather than mis-attributing its result/success onto an unrelated tool. steps is
// the lane's step slice, read to compare labels.
func popOpenTool(open map[int][]int, idx int, name string, steps []runStep) (int, bool) {
	stack := open[idx]
	// Walk the stack from the top (most recent) for a label match.
	for i := len(stack) - 1; i >= 0; i-- {
		if steps[stack[i]].Label == name {
			pos := stack[i]
			open[idx] = append(stack[:i:i], stack[i+1:]...)
			return pos, true
		}
	}
	return 0, false
}

// toolEndGlyph maps a tool's success flag to its settled glyph + CSS state via
// glyphFor — the single source of truth for status glyphs (run_render.go) — so the
// inspector's tool steps never drift from the live lanes / Runs history.
func toolEndGlyph(success bool) (glyph, state string) {
	if success {
		return glyphFor("done")
	}
	return glyphFor("failed")
}

// sortLaneIndices orders lane groups by index ascending with the run-level (-1)
// group last, preserving the lane-then-run-level reading order. Lane indices are
// unique, so the unstable sort.Slice is well-defined.
func sortLaneIndices(order []int) []int {
	out := append([]int(nil), order...)
	sort.Slice(out, func(i, j int) bool { return laneLess(out[i], out[j]) })
	return out
}

// laneLess orders lane indices ascending, treating -1 (run-level) as greater than
// every real lane so it sorts last.
func laneLess(a, b int) bool {
	if a == b {
		return false
	}
	if a < 0 {
		return false
	}
	if b < 0 {
		return true
	}
	return a < b
}

// clockTime renders an event's timestamp as wall-clock HH:MM:SS for the timeline
// row, or "" for a zero time (so the cell stays empty rather than printing an epoch).
func clockTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("15:04:05")
}

// handleRunDetail serves GET /page/runs/{id}: the read-only step-timeline detail
// page for one persisted run (ADR-0052). An unknown run id 404s cleanly; the route
// never touches the live run state — it reads the run history (s.runs) and the
// per-run event log only.
func (s *Server) handleRunDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rec, ok := s.findRun(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(s.runDetailPartial(rec, clampWindow(r.URL.Query().Get("window")))))
}

// findRun looks up a persisted run by id, returning false when no run store is
// wired or the id is unknown. Reads the append-only store's own snapshot — it does
// not touch s.mu / the live run state.
func (s *Server) findRun(id string) (telemetry.RunRecord, bool) {
	if s.runs == nil {
		return telemetry.RunRecord{}, false
	}
	for _, r := range s.runs.Records() {
		if r.ID == id {
			return r, true
		}
	}
	return telemetry.RunRecord{}, false
}

// runDetailPartial renders the run-detail page: a summary card (the runRow
// vocabulary — outcome glyph, mode, when, duration, cost) above the lane-grouped
// step timeline reconstructed from the per-run event log. A run with no log file
// (a pre-event-log run, or the log disabled) renders the card + a "no event log"
// note — no error. Lane labels resolve under forgeMu, like runRow. — ADR-0052.
func (s *Server) runDetailPartial(rec telemetry.RunRecord, window int) string {
	glyph, state := runOutcomeGlyph(rec.Outcome)
	dur := rec.Duration()
	credits := rec.Credits()

	var laneRows []map[string]any
	hasLog := false
	loadErr := false
	var logCredits float64
	if s.eventLogDir != "" {
		if log, err := telemetry.LoadRunEventLog(s.eventLogDir, rec.ID); err != nil {
			// A present-but-unreadable log (malformed/truncated <id>.json) is a data
			// problem, not an absent log — surface it distinctly, not as "no log".
			s.logger.Printf("run inspector: load event log %q: %v", rec.ID, err)
			loadErr = true
		} else if log.Count() > 0 {
			hasLog = true
			lanes := buildRunTimeline(log.Records())
			s.hub.forgeMu.Lock()
			for _, lt := range lanes {
				logCredits += lt.Credits
				laneRows = append(laneRows, s.laneTimelineRow(lt, rec))
			}
			s.hub.forgeMu.Unlock()
		}
	}

	// Per-run mini-reconciliation (issue 0092): when the log carries priced usage,
	// show its summed credits beside RunRecord.Credits and amber a non-trivial
	// mismatch — the V15 reconcile discipline at run grain. A pre-O2 log (logCredits
	// == 0) shows nothing extra, so the header stays unpriced-clean.
	hasLogCredits := logCredits > 0
	return frag("runDetailPage", map[string]any{
		"Name": rec.Name, "Mode": rec.Mode, "Outcome": rec.Outcome,
		"Glyph": glyph, "State": state, "When": humanWhen(rec.StartedAt),
		"Duration": humanDuration(dur), "HasDuration": dur > 0,
		"Credits": telemetry.FormatCredits(credits), "HasCredits": credits > 0,
		"LogCredits": telemetry.FormatCredits(logCredits), "HasLogCredits": hasLogCredits,
		"Amber":  hasLogCredits && math.Abs(logCredits-credits) >= reconcileEpsilon,
		"Window": window, "HasLog": hasLog, "LoadErr": loadErr, "Lanes": laneRows,
	})
}

// laneTimelineRow builds the template shape for one lane's slice of the timeline:
// its label (step N · agent, resolved from the run's recorded lanes — caller holds
// forgeMu — or "run-level" for the unattributed -1 group) and its rendered steps,
// each with its disclosure parts clamped. — ADR-0052.
func (s *Server) laneTimelineRow(lt laneSteps, rec telemetry.RunRecord) map[string]any {
	label := "run-level"
	if lt.LaneIndex >= 0 {
		label = fmt.Sprintf("step %d", lt.LaneIndex+1)
		if lt.LaneIndex < len(rec.Lanes) {
			label += " · " + s.agentLabel(rec.Lanes[lt.LaneIndex].AgentID)
		}
	}
	steps := make([]map[string]any, len(lt.Steps))
	for i, st := range lt.Steps {
		args := clampLines(st.Args, maxStepDetailLines)
		result := clampLines(st.Result, maxStepDetailLines)
		text := clampLines(st.Text, maxStepDetailLines)
		steps[i] = map[string]any{
			"Type": st.Type, "Glyph": st.Glyph, "State": def(st.State, st.Type),
			"Label": st.Label, "Clock": st.Clock,
			"Args": args, "HasArgs": args != "",
			"Result": result, "HasResult": result != "",
			"Text": text, "HasText": text != "",
			"HasDetail": args != "" || result != "" || text != "",
		}
	}
	return map[string]any{
		"Label": label, "Steps": steps,
		// The lane's rolled-up per-step usage (issue 0092), shown only when this lane
		// metered — a pre-O2 lane (no priced usage) stays cost-clean.
		"Credits": telemetry.FormatCredits(lt.Credits), "HasCredits": lt.Credits > 0,
	}
}
