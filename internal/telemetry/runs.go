package telemetry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// This file adds a persisted, append-only WORKFLOW RUN history — a sibling of the
// spend ledger (history.go). A run is the product's unit of orchestration (a set of
// lanes driven to completion), and unlike a metered turn it has a start/finish, an
// outcome, and per-lane results that include a SKIPPED branch which incurred no cost
// — none of which a spend record can express (a branch that didn't run leaves no
// turn). So runs get their own file, the same way spend does, mirroring config's
// persistence discipline: a versioned JSON document written atomically (temp-file +
// rename), missing-file = empty, present-but-invalid = error, empty dir = ephemeral.
// The RunStore is the only IO edge; the record types stay pure and dependency-free,
// like the rest of the package. — ADR-0022.

// RunLane is one lane's settled result within a persisted run: which agent ran, the
// terminal status (done | failed | skipped), and the credits it metered (zero for a
// skipped or free lane — Credits omits cleanly).
type RunLane struct {
	Index   int     `json:"index"`
	AgentID string  `json:"agentId,omitempty"`
	Status  string  `json:"status"`
	Credits float64 `json:"credits,omitempty"`
}

// RunRecord is one append-only entry: a completed workflow run. It names the run, the
// workflow definition it came from, the run mode, when it started and finished, the
// overall outcome (finished | failed), and each lane's settled result. Records are
// immutable once written; history only ever grows.
type RunRecord struct {
	ID         string    `json:"id"`
	WorkflowID string    `json:"workflow"`
	Name       string    `json:"name"`
	Mode       string    `json:"mode"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
	// Outcome is the run-level result: "finished" (every lane settled, no hard
	// failure) or "failed" (a step failed). A skipped branch is a normal outcome, not
	// a failure — see the per-lane Status (ADR-0021/0022).
	Outcome string    `json:"outcome"`
	Lanes   []RunLane `json:"lanes"`
}

// Credits totals the run's metered cost across its lanes (a skipped lane adds zero).
func (r RunRecord) Credits() float64 {
	var c float64
	for _, l := range r.Lanes {
		c += l.Credits
	}
	return c
}

// Duration is how long the run took (FinishedAt − StartedAt). A zero/unset
// StartedAt or FinishedAt (an in-flight or malformed record) or a non-positive
// span yields 0, so callers can sum and average over a history without guarding
// every entry. Pure: same record → same result.
func (r RunRecord) Duration() time.Duration {
	if r.StartedAt.IsZero() || r.FinishedAt.IsZero() {
		return 0
	}
	if d := r.FinishedAt.Sub(r.StartedAt); d > 0 {
		return d
	}
	return 0
}

const (
	runFile = "runs.json"
	// RunSchemaVersion is the on-disk schema version. Bumps must keep the "runs"
	// array readable by older code (additive fields only) or ship a migration — see
	// CONTRACTS.md §4 and docs/REGRESSIONS.md.
	RunSchemaVersion = 1
)

// runDoc is the on-disk envelope: a version tag plus the run array.
type runDoc struct {
	Version int         `json:"version"`
	Runs    []RunRecord `json:"runs"`
}

// RunStore is the persisted workflow-run history. It is goroutine-safe and atomic on
// write. A store with an empty dir is ephemeral (in-memory only) — used by the offline
// demo and tests so they never touch a real config directory.
type RunStore struct {
	mu   sync.Mutex
	dir  string
	runs []RunRecord
}

// LoadRunStore reads the history from dir/runs.json, returning an empty store when the
// file is absent (first run) and an error when it is present but unparseable. An empty
// dir yields an ephemeral, in-memory-only store.
func LoadRunStore(dir string) (*RunStore, error) {
	s := &RunStore{dir: dir}
	if dir == "" {
		return s, nil
	}
	data, err := os.ReadFile(filepath.Join(dir, runFile))
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read run history: %w", err)
	}
	var doc runDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse run history %s: %w", filepath.Join(dir, runFile), err)
	}
	// The runs array is the stable contract; a newer version's extra fields are
	// ignored by json.Unmarshal, so the file stays forward-readable.
	s.runs = doc.Runs
	return s, nil
}

// Append records a completed run (stamping FinishedAt with the current time when zero)
// and persists the whole history atomically. An ephemeral store keeps it in memory
// only. The atomic temp-file + rename means a crash mid-write never leaves a partial
// file — the prior history stays intact.
func (s *RunStore) Append(r RunRecord) error {
	if r.FinishedAt.IsZero() {
		r.FinishedAt = time.Now()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs = append(s.runs, r)
	return s.save()
}

// save writes the in-memory history to disk atomically. Caller holds s.mu. A no-op for
// an ephemeral store. Mirrors config.Save / SpendStore.save: temp-file + rename.
func (s *RunStore) save() error {
	if s.dir == "" {
		return nil
	}
	doc := runDoc{Version: RunSchemaVersion, Runs: s.runs}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("encode run history: %w", err)
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	path := filepath.Join(s.dir, runFile)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write run history: %w", err)
	}
	return os.Rename(tmp, path)
}

// Records returns a snapshot copy of the history, safe to read without the lock.
func (s *RunStore) Records() []RunRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]RunRecord, len(s.runs))
	copy(out, s.runs)
	return out
}

// Count returns how many runs the history holds.
func (s *RunStore) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.runs)
}

// RunAggregate rolls one workflow's run history into a summary: how many times it
// ran, how many of those failed, and the total/average metered cost and wall-clock
// duration across those runs. It is the orchestration half of cost attribution —
// where WorkflowShares answers "how much did this workflow spend?", RunAggregate
// adds "how often does it run, how long does it take, and how often does it fail?"
// — joining RunStore's grain (count / outcome / duration) to the cost the runs
// metered, with no schema change (the records already carry it). — ADR-0022.
type RunAggregate struct {
	WorkflowID    string
	Name          string
	Runs          int
	Failures      int
	TotalCredits  float64
	AvgCredits    float64
	TotalDuration time.Duration
	AvgDuration   time.Duration
	// LastOutcome and LastStartedAt name this workflow's most recent run — the
	// "last run" signal the Workflows page badges (V4): the outcome
	// ("finished" | "failed") and when it started, so a row can show how the
	// workflow last ended and how long ago. "Most recent" is by StartedAt; a tie
	// keeps the run with the later FinishedAt, then the earlier-seen (stable input
	// order) — a deterministic pick regardless of record order. The zero value
	// (empty outcome, zero time) is never produced for a present aggregate (a group
	// always has ≥ 1 run), so callers can treat a non-zero Runs as the guard.
	LastOutcome   string
	LastStartedAt time.Time
}

// FailureRate is the share of this workflow's runs that failed, in [0,1] (0 when
// it has never run). Pure: same aggregate → same result.
func (a RunAggregate) FailureRate() float64 {
	if a.Runs == 0 {
		return 0
	}
	return float64(a.Failures) / float64(a.Runs)
}

// RunAggregates rolls a run history up per workflow — a cousin of the *Shares
// spend readers — sorted by run count descending (ties broken by workflow id
// ascending) for a deterministic order. A run whose Outcome is "failed" counts as
// a failure; a skipped lane adds zero cost (RunRecord.Credits already excludes it),
// and an unfinished/zero-span run contributes zero duration (RunRecord.Duration
// guards it). Averages divide by the run count (always ≥ 1 for a present group, so
// no divide-by-zero). The display Name is the latest one seen for the workflow
// (records arrive in append/chronological order), so a since-renamed workflow reads
// under its current name. Pure: same records → same result.
func RunAggregates(records []RunRecord) []RunAggregate {
	by := map[string]*RunAggregate{}
	// lastFin tracks the FinishedAt of each group's current latest run, the tie-break
	// when two runs share a StartedAt (the StartedAt-equal case the Workflows demo and
	// any same-instant batch hit). Kept beside the aggregate (not on it) since it's a
	// roll-up internal, not part of the public RunAggregate.
	lastFin := map[string]time.Time{}
	for _, r := range records {
		a := by[r.WorkflowID]
		if a == nil {
			a = &RunAggregate{WorkflowID: r.WorkflowID}
			by[r.WorkflowID] = a
		}
		a.Name = r.Name // chronological append order ⇒ the latest name wins
		a.Runs++
		if r.Outcome == "failed" {
			a.Failures++
		}
		a.TotalCredits += r.Credits()
		a.TotalDuration += r.Duration()
		// The latest run wins by StartedAt, then by the later FinishedAt, then stable
		// (the first record of a group seeds it; a later one replaces only when strictly
		// later) — so the last-run badge is deterministic regardless of input order.
		if a.Runs == 1 || laterRun(r.StartedAt, r.FinishedAt, a.LastStartedAt, lastFin[r.WorkflowID]) {
			a.LastOutcome = r.Outcome
			a.LastStartedAt = r.StartedAt
			lastFin[r.WorkflowID] = r.FinishedAt
		}
	}
	out := make([]RunAggregate, 0, len(by))
	for _, a := range by {
		a.AvgCredits = a.TotalCredits / float64(a.Runs)
		a.AvgDuration = a.TotalDuration / time.Duration(a.Runs)
		out = append(out, *a)
	}
	// Runs desc, then workflow id asc — a total order (the id key is unique per
	// group), so the result is fully deterministic regardless of map iteration.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Runs != out[j].Runs {
			return out[i].Runs > out[j].Runs
		}
		return out[i].WorkflowID < out[j].WorkflowID
	})
	return out
}

// laterRun reports whether the run (s2, f2) is strictly more recent than (s1, f1):
// a later StartedAt, or — on an equal StartedAt — a later FinishedAt. Equal on both
// yields false, so an equal pair keeps the incumbent (the earlier-seen record),
// giving the last-run pick a stable, deterministic tie-break. Pure.
func laterRun(s2, f2, s1, f1 time.Time) bool {
	if !s2.Equal(s1) {
		return s2.After(s1)
	}
	return f2.After(f1)
}
