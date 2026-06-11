package bootstrap

// guard_test.go — additional guard tests for items 11–13.

import (
	"os"
	"testing"

	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// ── Item 11: seedRuns populates history ──────────────────────────────────────

// TestSeedRunsPopulatesHistory mirrors TestSeedSpendPopulatesDeterministicHistory
// for the run store: seedRuns must add at least 2 records, include a skipped lane,
// and use distinct workflow IDs.
func TestSeedRunsPopulatesHistory(t *testing.T) {
	store, err := telemetry.LoadRunStore("") // ephemeral
	if err != nil {
		t.Fatalf("LoadRunStore: %v", err)
	}
	seedRuns(store)
	recs := store.Records()
	if len(recs) < 2 {
		t.Fatalf("seedRuns should add at least 2 run records, got %d", len(recs))
	}

	// Distinct workflow IDs — a run store spanning only one workflow would not
	// exercise the multi-workflow reconciliation view.
	wfIDs := map[string]bool{}
	for _, r := range recs {
		wfIDs[r.WorkflowID] = true
	}
	if len(wfIDs) < 2 {
		t.Fatalf("seedRuns should span at least 2 workflow IDs, got: %v", wfIDs)
	}

	// At least one skipped lane must exist (the branching run guards the per-lane
	// outcome the Runs page needs to render the "skipped" state offline).
	hasSkipped := false
	for _, r := range recs {
		for _, lane := range r.Lanes {
			if lane.Status == "skipped" {
				hasSkipped = true
			}
		}
	}
	if !hasSkipped {
		t.Fatal("seedRuns should include at least one skipped lane (branch-run guard)")
	}
}

// ── Item 12: Build rejects a config dir that is a regular file ───────────────

// TestBuildBadConfigDir guards that Build returns a non-nil error (not a panic)
// when the config dir path points to a regular file rather than a directory.
func TestBuildBadConfigDir(t *testing.T) {
	// Create a temp file and pass its path as the config dir.
	f, err := os.CreateTemp("", "bad-config-*")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	_, closeFn, err := Build(f.Name(), false)
	if err == nil {
		if closeFn != nil {
			closeFn()
		}
		t.Fatal("Build with a file path as configDir should return an error, got nil")
	}
	// closeFn is nil on error — no cleanup needed.
}

// ── Item 13: DefaultConfigDir env-var override and fallback ──────────────────

// TestDefaultConfigDir guards both branches of DefaultConfigDir:
// • MY_ORCHESTRA_HOME set → return that value
// • MY_ORCHESTRA_HOME unset → return a non-empty fallback path
func TestDefaultConfigDir(t *testing.T) {
	const envVar = "MY_ORCHESTRA_HOME"

	t.Run("env override", func(t *testing.T) {
		t.Setenv(envVar, "/custom/path")
		got := DefaultConfigDir()
		if got != "/custom/path" {
			t.Fatalf("DefaultConfigDir with %s=/custom/path = %q, want /custom/path", envVar, got)
		}
	})

	t.Run("fallback non-empty", func(t *testing.T) {
		// Ensure the env var is unset for this sub-test.
		t.Setenv(envVar, "")
		got := DefaultConfigDir()
		if got == "" {
			t.Fatal("DefaultConfigDir fallback should return a non-empty path")
		}
	})
}
