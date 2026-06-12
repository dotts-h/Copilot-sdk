package web

import (
	"fmt"

	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// This file adds the run inspector's transcript view (O3, issue 0093): an alternate
// rendering of the same per-run event log the step timeline reads, flattened into
// chat reading order — the "Messages view" every vendor ships beside the step tree.
// The timeline is for *locating* a step; the transcript is for *understanding* the
// run. It reuses the timeline's tool-join and lane-label machinery (runs.go) and
// renders message bodies through the block-AST designed-output pipeline (renderMarkdown,
// epic 0076's second consumer). — ADR-0052.

const (
	// viewTimeline is the default run-detail view: the lane-grouped step timeline.
	viewTimeline = "timeline"
	// viewTranscript is the chat-order transcript view, selected by ?view=transcript.
	viewTranscript = "transcript"
)

// clampRunView maps the raw ?view= query value to a known view, defaulting to the
// timeline for an empty or unrecognized value — the same clamp discipline as
// clampWindow, so a garbage view can never render a half-built page. Only the
// explicit "transcript" opts into the chat-order view.
func clampRunView(raw string) string {
	if raw == viewTranscript {
		return viewTranscript
	}
	return viewTimeline
}

// transcriptItem is one row in the chat-order transcript reconstruction: a message
// turn (user/assistant), a tool card, an error, or a lane-transition marker — in
// event order, not grouped by lane. It is the pure shape; lane labels are resolved
// by transcriptRows (which holds forgeMu). Kind is the CSS suffix and the row kind;
// Body carries raw markdown (rendered through renderMarkdown by the template); Args,
// Result and Text carry escaped-as-pre disclosure bodies; Credits is the per-turn
// priced usage (O2) attached to an assistant turn. All strings are escaped by the
// template (ADR-0001).
type transcriptItem struct {
	Kind      string // "lane" | "user" | "assistant" | "tool" | "error"
	LaneIndex int
	Glyph     string
	State     string
	Label     string
	Clock     string
	Body      string
	Args      string
	Result    string
	Text      string
	Credits   float64
}

// buildRunTranscript reconstructs a run's chat-order transcript from its event log.
// It is PURE: same events → same transcript, no IO, no locking.
//
//   - Events render in their recorded (chronological) order — the chat reading
//     order — NOT grouped by lane like the timeline. A lane-transition marker is
//     emitted whenever the active lane changes, so an interleaved multi-lane run
//     stays legible.
//   - EvUserMessage / EvMessage become user / assistant turns whose bodies ride
//     raw for the block-AST renderer; EvError becomes a compact error row;
//     EvToolStart+EvToolEnd join into ONE compact tool card (args from the start,
//     result + success from the end), matched by name within the lane like the
//     timeline (ADR-0052). Streaming deltas, reasoning, and the other lifecycle
//     events are coalesced away — the committed turn is the record.
//   - EvUsage carries no visible row; its priced credits (O2, issue 0092) attach to
//     the assistant turn they belong to. Usage is reported at the END of a turn, so
//     it attaches to the most recent assistant turn in its lane; usage seen before
//     any assistant turn in a lane is held pending and seeds the next one.
func buildRunTranscript(events []telemetry.RunEvent) []transcriptItem {
	var items []transcriptItem
	lastLane := -2                      // sentinel: the first real event always emits a marker
	openTools := map[int][]int{}        // lane → stack of open tool-card positions (into items)
	lastAssistant := map[int]int{}      // lane → position of its most recent assistant turn
	pendingCredits := map[int]float64{} // lane → usage credits seen before its next assistant turn

	mark := func(lane int) {
		if lane != lastLane {
			items = append(items, transcriptItem{Kind: "lane", LaneIndex: lane})
			lastLane = lane
		}
	}

	for _, e := range events {
		switch e.Type {
		case "EvUserMessage":
			mark(e.LaneIndex)
			items = append(items, transcriptItem{
				Kind: "user", LaneIndex: e.LaneIndex, Glyph: "❯",
				Label: "User", Clock: clockTime(e.At), Body: e.Text,
			})
		case "EvMessage":
			mark(e.LaneIndex)
			items = append(items, transcriptItem{
				Kind: "assistant", LaneIndex: e.LaneIndex, Glyph: "✎",
				Label: "Assistant", Clock: clockTime(e.At), Body: e.Text,
				Credits: pendingCredits[e.LaneIndex],
			})
			delete(pendingCredits, e.LaneIndex)
			lastAssistant[e.LaneIndex] = len(items) - 1
		case "EvError":
			mark(e.LaneIndex)
			items = append(items, transcriptItem{
				Kind: "error", LaneIndex: e.LaneIndex, Glyph: "✗", State: "failed",
				Label: "Error", Clock: clockTime(e.At), Text: e.Err,
			})
		case "EvToolStart":
			mark(e.LaneIndex)
			items = append(items, transcriptItem{
				Kind: "tool", LaneIndex: e.LaneIndex, Glyph: "●", State: "running",
				Label: def(e.Tool, "tool"), Clock: clockTime(e.At), Args: e.Args,
			})
			openTools[e.LaneIndex] = append(openTools[e.LaneIndex], len(items)-1)
		case "EvToolEnd":
			if pos, ok := popOpenTool(openTools, e.LaneIndex, def(e.Tool, "tool"), func(p int) string { return items[p].Label }); ok {
				items[pos].Result = e.Result
				items[pos].Glyph, items[pos].State = toolEndGlyph(e.Success)
				continue
			}
			// Unmatched end (no recorded start) — render it on its own card.
			mark(e.LaneIndex)
			glyph, state := toolEndGlyph(e.Success)
			items = append(items, transcriptItem{
				Kind: "tool", LaneIndex: e.LaneIndex, Glyph: glyph, State: state,
				Label: def(e.Tool, "tool"), Clock: clockTime(e.At), Result: e.Result,
			})
		case "EvUsage":
			// Priced usage (O2) attaches to the turn it belongs to — the most recent
			// assistant turn in the lane, or the next one when usage leads.
			if pos, ok := lastAssistant[e.LaneIndex]; ok {
				items[pos].Credits += e.Credits
			} else {
				pendingCredits[e.LaneIndex] += e.Credits
			}
		}
	}
	return items
}

// transcriptRows projects the pure transcript items into the template shapes,
// resolving each lane marker's label (step N · agent, like the timeline) — the
// caller holds forgeMu. Tool disclosure bodies are clamped like the timeline's;
// message bodies ride raw for the markdown renderer (escaped there). — ADR-0052.
func (s *Server) transcriptRows(items []transcriptItem, rec telemetry.RunRecord) []map[string]any {
	rows := make([]map[string]any, len(items))
	for i, it := range items {
		if it.Kind == "lane" {
			rows[i] = map[string]any{"Kind": "lane", "Label": s.laneLabel(it.LaneIndex, rec)}
			continue
		}
		args := clampLines(it.Args, maxStepDetailLines)
		result := clampLines(it.Result, maxStepDetailLines)
		text := clampLines(it.Text, maxStepDetailLines)
		rows[i] = map[string]any{
			"Kind": it.Kind, "Glyph": it.Glyph, "State": def(it.State, it.Kind),
			"Label": it.Label, "Clock": it.Clock,
			"Body": it.Body, "HasBody": it.Body != "",
			"Args": args, "HasArgs": args != "",
			"Result": result, "HasResult": result != "",
			"Text": text, "HasText": text != "",
			"HasDetail": args != "" || result != "",
			"Credits":   telemetry.FormatCredits(it.Credits), "HasCredits": it.Credits > 0,
		}
	}
	return rows
}

// laneLabel names a lane group for the inspector: "step N · agent" (1-based, the
// agent resolved from the run's recorded lanes — caller holds forgeMu) or
// "run-level" for the unattributed -1 group. Shared by the timeline and the
// transcript so the two views label lanes identically. — ADR-0052.
func (s *Server) laneLabel(laneIndex int, rec telemetry.RunRecord) string {
	if laneIndex < 0 {
		return "run-level"
	}
	label := fmt.Sprintf("step %d", laneIndex+1)
	if laneIndex < len(rec.Lanes) {
		label += " · " + s.agentLabel(rec.Lanes[laneIndex].AgentID)
	}
	return label
}

// sumEventCredits totals the priced usage credits across an event log — the per-run
// figure the detail header cross-checks against RunRecord.Credits (issue 0092). A
// property of the log, not the view, so the timeline and transcript reconcile against
// the same number. Pure; a pre-O2 log (no priced usage) sums to zero.
func sumEventCredits(events []telemetry.RunEvent) float64 {
	var total float64
	for _, e := range events {
		if e.Type == "EvUsage" {
			total += e.Credits
		}
	}
	return total
}
