package telemetry

import (
	"path/filepath"
	"time"
)

// This file adds a per-run event log — an optional, additive append-only log of
// the normalized run events (the Ev* stream) emitted during a workflow run. Built
// on the shared AppendOnlyStore[RunEvent] machinery (store.go), keyed by run id,
// so a run can be reconstructed step-by-step (replay/audit). Disabling it (or an
// absent log dir) leaves the existing run/spend recording and SSE path byte-identical
// — no behavior change to the live surface. — ADR-0048.

const (
	// eventLogSchemaVersion is the on-disk schema version for the run event log.
	// Bumps must keep the "events" array readable by older code (additive fields
	// only) or ship a migration — the same ADR-0009/ADR-0022 discipline.
	eventLogSchemaVersion = 1

	// eventLogSubdir is the subdirectory under the config dir that holds per-run
	// event log files. Keeping them separate from runs.json and spend.json avoids
	// a large-list-of-files in the config root.
	eventLogSubdir = "eventlogs"
)

// RunEvent is one entry in the per-run event log: a normalized event that was
// emitted by the Hub's pump during an active run. The fields capture the event's
// type and its type-specific payload so the sequence can be replayed/audited.
// Fields that are not applicable to a given event type are omitted (omitempty).
// The JSON tags are the stable on-disk contract — see ADR-0048.
type RunEvent struct {
	// At is when the event was recorded; always set.
	At time.Time `json:"at"`
	// RunID is the run this event belongs to; always set so each log is
	// self-identifying even if the file is read independently.
	RunID string `json:"runId"`
	// Type is the normalized event type name (e.g. "EvMessage", "EvToolStart").
	Type string `json:"type"`
	// LaneIndex is the lane within the run that this event belongs to. -1 means
	// the event could not be attributed to any lane (run-level or ambiguous).
	// Lane 0 is stored as 0 — distinguishable from -1 (unattributed). The display
	// "step N" is LaneIndex+1. Always present in the JSON (no omitempty) so lane 0
	// events are never confused with unattributed events.
	LaneIndex int `json:"laneIndex"`
	// Text carries streamed/committed message or reasoning text.
	Text string `json:"text,omitempty"`
	// Tool is the tool name for EvToolStart/EvToolEnd events.
	Tool string `json:"tool,omitempty"`
	// Args is the tool-call arguments for EvToolStart.
	Args string `json:"args,omitempty"`
	// Result is the tool-call result for EvToolEnd.
	Result string `json:"result,omitempty"`
	// Success reports EvToolEnd's success flag.
	Success bool `json:"success,omitempty"`
	// Err carries the error message for EvError events.
	Err string `json:"err,omitempty"`
	// TokensIn and TokensOut are the input/output token counts of an EvUsage turn,
	// stamped so the timeline can be priced at the finest grain (run → lane → step).
	// Additive (omitempty): a pre-O2 log reads them back zero. — issue 0092, ADR-0048.
	TokensIn  int64 `json:"tokensIn,omitempty"`
	TokensOut int64 `json:"tokensOut,omitempty"`
	// Credits is the price-book estimate the meter computed for an EvUsage turn AT
	// TIME OF USE — the same figure the run record accumulates per lane (the estimate
	// from telemetry.Price, NOT the reported-AIU authoritative basis), so the summed
	// log credits reconcile against RunRecord.Credits on one basis. Stamped once at log
	// time and never recomputed, so a later price-book change (or live repricing) can't
	// silently reprice history. Additive (omitempty): older logs read back zero.
	// — issue 0092, ADR-0048.
	Credits float64 `json:"credits,omitempty"`
}

// RunEventLog is the per-run event log: a thin typed wrapper over the shared
// AppendOnlyStore[RunEvent] (store.go), one file per run id. It is goroutine-safe
// and atomic on write. A store with an empty dir is ephemeral (in-memory only) —
// used by the offline demo and tests so they never touch a real config directory.
// Append, Records, and Count are promoted from the embedded generic store.
// — ADR-0048.
type RunEventLog struct {
	*AppendOnlyStore[RunEvent]
}

// RunEventLogPath returns the canonical on-disk path for a run's event log file.
// Exported so tests can inspect the file directly (e.g. for the tag-stability pin).
func RunEventLogPath(dir, runID string) string {
	return filepath.Join(dir, eventLogSubdir, runID+".json")
}

// LoadRunEventLog reads the event log for runID from dir/<eventLogSubdir>/<runID>.json,
// returning an empty log when the file is absent (first run) and an error when it
// is present but unparseable. An empty dir yields an ephemeral, in-memory-only log.
// Append stamps At with the current time when zero (the per-record hook).
func LoadRunEventLog(dir, runID string) (*RunEventLog, error) {
	// Each run lives in its own file: dir/eventlogs/<runID>.json.
	// The AppendOnlyStore uses dir+file as its storage path, so we pass the
	// subdir as the dir and the per-run file name as the file.
	var storeDir string
	if dir != "" {
		storeDir = filepath.Join(dir, eventLogSubdir)
	}
	inner, err := loadAppendOnlyStore[RunEvent](
		storeDir,
		runID+".json",
		"events",
		"run event log",
		eventLogSchemaVersion,
		stampRunEvent,
	)
	if err != nil {
		return nil, err
	}
	return &RunEventLog{inner}, nil
}

// stampRunEvent defaults a record's At to now when the caller left it zero —
// the event log's per-record Append hook.
func stampRunEvent(e RunEvent) RunEvent {
	if e.At.IsZero() {
		e.At = time.Now()
	}
	return e
}
