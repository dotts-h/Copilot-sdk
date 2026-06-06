package telemetry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
