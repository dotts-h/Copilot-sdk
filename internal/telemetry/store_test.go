package telemetry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// widget is a throwaway record type used to exercise the generic AppendOnlyStore[T]
// directly — independent of the SpendRecord/RunRecord shapes — so the shared
// machinery (round-trip, ephemeral, missing, corrupt, newer-version, atomic write)
// is covered on its own, not only through the two wrappers.
type widget struct {
	Name string `json:"name"`
	N    int    `json:"n"`
}

// loadWidgetStore is a tiny constructor mirroring LoadSpendStore/LoadRunStore: it
// pins the generic store to a "widgets.json" file with a "widgets" array key and no
// per-record stamp (nil hook), so the test drives the bare machinery.
func loadWidgetStore(dir string) (*AppendOnlyStore[widget], error) {
	return loadAppendOnlyStore[widget](dir, "widgets.json", "widgets", "widget store", 1, nil)
}

func TestAppendOnlyStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := loadWidgetStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Append(widget{Name: "a", N: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.Append(widget{Name: "b", N: 2}); err != nil {
		t.Fatal(err)
	}
	// Atomic write leaves the canonical file in place (no stray .tmp on success).
	if _, err := os.Stat(filepath.Join(dir, "widgets.json")); err != nil {
		t.Fatalf("store file not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "widgets.json.tmp")); !os.IsNotExist(err) {
		t.Fatalf("temp file should be renamed away, stat err = %v", err)
	}
	// A second store reads the persisted records back (survives "restart").
	reloaded, err := loadWidgetStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := reloaded.Records()
	if len(got) != 2 || got[0].Name != "a" || got[1].N != 2 {
		t.Fatalf("round trip lost records: %+v", got)
	}
}

func TestAppendOnlyStoreEphemeralNeverWrites(t *testing.T) {
	s, err := loadWidgetStore("") // empty dir => in-memory only
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Append(widget{Name: "x"}); err != nil {
		t.Fatal(err)
	}
	if s.Count() != 1 {
		t.Fatalf("ephemeral store should still accumulate, got %d", s.Count())
	}
}

func TestAppendOnlyStoreMissingIsEmpty(t *testing.T) {
	s, err := loadWidgetStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if s.Count() != 0 {
		t.Fatalf("fresh store should be empty, got %d", s.Count())
	}
}

func TestAppendOnlyStoreRejectsCorruptFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "widgets.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadWidgetStore(dir); err == nil {
		t.Fatal("expected error on corrupt store file")
	}
}

func TestAppendOnlyStoreToleratesNewerSchema(t *testing.T) {
	// Forward-compatible: a higher version with extra envelope/record fields still
	// yields its records — the array under the key is the stable contract.
	dir := t.TempDir()
	body := `{"version":99,"widgets":[{"name":"z","n":7,"future":"ignored"}],"extra":true}`
	if err := os.WriteFile(filepath.Join(dir, "widgets.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := loadWidgetStore(dir)
	if err != nil {
		t.Fatalf("newer schema should be readable: %v", err)
	}
	if s.Count() != 1 || s.Records()[0].N != 7 {
		t.Fatalf("want the one newer-schema record, got %+v", s.Records())
	}
}

func TestAppendOnlyStoreMissingKeyIsEmpty(t *testing.T) {
	// A well-formed envelope that simply lacks the array key (e.g. an empty document)
	// yields an empty store, not an error.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "widgets.json"), []byte(`{"version":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := loadWidgetStore(dir)
	if err != nil {
		t.Fatalf("missing array key should load empty, got err: %v", err)
	}
	if s.Count() != 0 {
		t.Fatalf("want empty store, got %d", s.Count())
	}
}

// TestSpendStoreOnDiskTagsAreStable pins the spend envelope's literal on-disk tags:
// the file the generic store writes still carries the `version` + `records` keys (and
// the v2 record tags), so the stable contract didn't drift in the H1 extraction.
func TestSpendStoreOnDiskTagsAreStable(t *testing.T) {
	dir := t.TempDir()
	s, err := LoadSpendStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	rec := SpendRecord{Model: "gpt-5", USD: 0.5, AgentID: "builder", WorkflowID: "ship", LaneIndex: 2}
	if err := s.Append(rec); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "spend.json"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, want := range []string{`"version": 4`, `"records": [`, `"model": "gpt-5"`, `"agent": "builder"`, `"workflow": "ship"`, `"lane": 2`} {
		if !strings.Contains(body, want) {
			t.Fatalf("spend.json missing stable tag %q in:\n%s", want, body)
		}
	}
}

// TestRunStoreOnDiskTagsAreStable pins the run envelope's literal on-disk tags: the
// file still carries the `version` + `runs` keys (and the lane tags), so the stable
// contract didn't drift in the H1 extraction.
func TestRunStoreOnDiskTagsAreStable(t *testing.T) {
	dir := t.TempDir()
	s, err := LoadRunStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Append(sampleRun()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "runs.json"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, want := range []string{`"version": 2`, `"runs": [`, `"workflow": "build-and-harden"`, `"lanes": [`, `"status": "done"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("runs.json missing stable tag %q in:\n%s", want, body)
		}
	}
}
