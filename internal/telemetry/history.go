package telemetry

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

// This file adds a persisted, append-only spend ledger so telemetry survives a
// restart — turning the live in-memory Meter (a gauge) into an accountable
// record. It mirrors config's persistence discipline: a versioned JSON document
// written atomically (temp-file + rename), missing-file = empty, present-but-
// invalid = error. The aggregation helpers (DailyTotals, ModelShares,
// AgentShares, WorkflowShares, WriteCSV) are pure functions over a record slice
// — the SpendStore is the only IO edge,
// keeping the rest of the package dependency-free and unit-testable. — ADR-0009.

// SpendRecord is one append-only ledger entry: the metered cost of a single
// assistant turn, tagged with the session it belongs to and when it happened.
// Records are immutable once written; history only ever grows.
type SpendRecord struct {
	At           time.Time `json:"at"`
	SessionID    string    `json:"session,omitempty"`
	Model        string    `json:"model"`
	InputTokens  int64     `json:"in"`
	CachedTokens int64     `json:"cached"`
	OutputTokens int64     `json:"out"`
	// USD is the estimate-priced dollar cost of this turn (the Meter's number).
	USD float64 `json:"usd"`
	// AIU is GitHub's authoritative cost in AI units, when the runtime reported
	// it (zero for the offline mock / unreported turns).
	AIU float64 `json:"aiu,omitempty"`
	// AgentID attributes the turn to the active agent persona that incurred it
	// (empty = the built-in chat agent / no persona). WorkflowID and LaneIndex are
	// set additionally when a multi-agent workflow run owned the turn, naming the
	// run and the lane within it. All three are additive, schema-v2 tags (older
	// readers ignore them; older records read back empty) so the ledger answers
	// "which agent / which workflow burned the budget." — ADR-0018.
	AgentID    string `json:"agent,omitempty"`
	WorkflowID string `json:"workflow,omitempty"`
	// LaneIndex is meaningful only alongside a non-empty WorkflowID; its zero value
	// (the first lane, or a non-workflow turn) is correct, so it omits cleanly.
	LaneIndex int `json:"lane,omitempty"`
}

// Credits expresses the record's USD cost in GitHub AI Credits (1 cr = $0.01).
func (r SpendRecord) Credits() float64 { return r.USD / USDPerCredit }

// Day is the record's UTC calendar day (YYYY-MM-DD), the bucket key for trends.
func (r SpendRecord) Day() string { return r.At.UTC().Format("2006-01-02") }

const (
	spendFile = "spend.json"
	// SpendSchemaVersion is the on-disk schema version. Bumps must keep the
	// "records" array readable by older code (additive fields only) or ship a
	// migration — see CONTRACTS.md §4 and docs/REGRESSIONS.md. v2 added the
	// additive agent/workflow/lane attribution tags (ADR-0018): older v1 readers
	// ignore the new keys and tolerate the higher version; v1 records read back
	// with empty tags.
	SpendSchemaVersion = 2
)

// spendDoc is the on-disk envelope: a version tag plus the record array.
type spendDoc struct {
	Version int           `json:"version"`
	Records []SpendRecord `json:"records"`
}

// SpendStore is the persisted spend ledger. It is goroutine-safe and atomic on
// write. A store with an empty dir is ephemeral (in-memory only) — used by the
// offline demo and tests so they never touch a real config directory.
type SpendStore struct {
	mu      sync.Mutex
	dir     string
	records []SpendRecord
}

// LoadSpendStore reads the ledger from dir/spend.json, returning an empty store
// when the file is absent (first run) and an error when it is present but
// unparseable. An empty dir yields an ephemeral, in-memory-only store.
func LoadSpendStore(dir string) (*SpendStore, error) {
	s := &SpendStore{dir: dir}
	if dir == "" {
		return s, nil
	}
	data, err := os.ReadFile(filepath.Join(dir, spendFile))
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read spend history: %w", err)
	}
	var doc spendDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse spend history %s: %w", filepath.Join(dir, spendFile), err)
	}
	// The records array is the stable contract; a newer version's extra fields
	// are ignored by json.Unmarshal, so the file stays forward-readable.
	s.records = doc.Records
	return s, nil
}

// Append records a turn's spend (stamping At with the current time when zero)
// and persists the whole ledger atomically. An ephemeral store keeps it in
// memory only. The atomic temp-file + rename means a crash mid-write never
// leaves a partial file — the prior history stays intact.
func (s *SpendStore) Append(r SpendRecord) error {
	if r.At.IsZero() {
		r.At = time.Now()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, r)
	return s.save()
}

// save writes the in-memory ledger to disk atomically. Caller holds s.mu. A
// no-op for an ephemeral store. Mirrors config.Save's temp-file + rename.
func (s *SpendStore) save() error {
	if s.dir == "" {
		return nil
	}
	doc := spendDoc{Version: SpendSchemaVersion, Records: s.records}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("encode spend history: %w", err)
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	path := filepath.Join(s.dir, spendFile)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write spend history: %w", err)
	}
	return os.Rename(tmp, path)
}

// Records returns a snapshot copy of the ledger, safe to read without the lock.
func (s *SpendStore) Records() []SpendRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SpendRecord, len(s.records))
	copy(out, s.records)
	return out
}

// Count returns how many records the ledger holds.
func (s *SpendStore) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.records)
}

// DayTotal aggregates one calendar day's spend across all sessions and models.
type DayTotal struct {
	Day     string
	USD     float64
	Credits float64
	Count   int // number of turns recorded that day
}

// DailyTotals groups records by UTC calendar day, ascending — the "spend over
// time" trend. Pure: same records → same result.
func DailyTotals(records []SpendRecord) []DayTotal {
	byDay := map[string]*DayTotal{}
	for _, r := range records {
		d := r.Day()
		t := byDay[d]
		if t == nil {
			t = &DayTotal{Day: d}
			byDay[d] = t
		}
		t.USD += r.USD
		t.Count++
	}
	days := make([]string, 0, len(byDay))
	for d := range byDay {
		days = append(days, d)
	}
	sort.Strings(days)
	out := make([]DayTotal, 0, len(days))
	for _, d := range days {
		t := byDay[d]
		t.Credits = t.USD / USDPerCredit
		out = append(out, *t)
	}
	return out
}

// MonthToDate sums the spend recorded within now's UTC calendar month — the
// account-wide "this month" total that the budget rows, the cost footer, and the
// hard-cap baseline read from the persisted ledger so they survive a restart
// (ADR-0016), rather than from the restart-amnesiac in-process Meter. Pure: same
// records + now → same result, no IO. The UTC calendar month matches the flat
// monthly-allowance mental model; a configurable billing-anchor day is a later,
// additive tweak over this same pure window.
//
// The ledger persists only each turn's total USD (not the input/cached/output
// split — see SpendRecord), so the month total is returned through Cost's
// USD()/Credits() accessors (the only fields the account-wide surfaces read); the
// per-component breakdown is not meaningful for an aggregate.
func MonthToDate(records []SpendRecord, now time.Time) Cost {
	y, m, _ := now.UTC().Date()
	var usd float64
	for _, r := range records {
		ry, rm, _ := r.At.UTC().Date()
		if ry == y && rm == m {
			usd += r.USD
		}
	}
	return Cost{InputUSD: usd}
}

// share is one group's slice of the total persisted spend — the shape behind the
// ModelShare/AgentShare/WorkflowShare breakdowns, which differ only in their key.
type share struct {
	Key      string
	USD      float64
	Credits  float64
	Fraction float64 // share of the included total in [0,1]
}

// shareBy totals spend per group (keyed by keyOf) and computes each group's
// fraction of the included whole, sorted by spend descending (ties broken by key
// for determinism). When includeEmpty is false, records whose key is "" are
// skipped entirely and excluded from the total — used by WorkflowShares so the
// "no workflow" chat spend doesn't swamp the per-workflow view. Pure: same
// records → same result.
func shareBy(records []SpendRecord, keyOf func(SpendRecord) string, includeEmpty bool) []share {
	by := map[string]float64{}
	var total float64
	for _, r := range records {
		k := keyOf(r)
		if k == "" && !includeEmpty {
			continue
		}
		by[k] += r.USD
		total += r.USD
	}
	out := make([]share, 0, len(by))
	for k, usd := range by {
		s := share{Key: k, USD: usd, Credits: usd / USDPerCredit}
		if total > 0 {
			s.Fraction = usd / total
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].USD != out[j].USD {
			return out[i].USD > out[j].USD
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// ModelShare is one model's slice of the total persisted spend.
type ModelShare struct {
	Model    string
	USD      float64
	Credits  float64
	Fraction float64 // share of total spend in [0,1]
}

// ModelShares totals spend per model and computes each model's fraction of the
// whole, sorted by spend descending (ties broken by model name for
// determinism). Pure: same records → same result.
func ModelShares(records []SpendRecord) []ModelShare {
	groups := shareBy(records, func(r SpendRecord) string { return r.Model }, true)
	out := make([]ModelShare, len(groups))
	for i, g := range groups {
		out[i] = ModelShare{Model: g.Key, USD: g.USD, Credits: g.Credits, Fraction: g.Fraction}
	}
	return out
}

// AgentShare is one agent persona's slice of the total persisted spend. An empty
// AgentID is spend incurred with no agent persona (the built-in chat agent) — the
// caller labels it for display.
type AgentShare struct {
	AgentID  string
	USD      float64
	Credits  float64
	Fraction float64 // share of total spend in [0,1]
}

// AgentShares totals spend per agent persona and computes each agent's fraction
// of all recorded spend (the empty-agent bucket included, since every turn has an
// agent), sorted by spend descending. Answers "which agent is burning my budget?"
// (ADR-0018). Pure: same records → same result.
func AgentShares(records []SpendRecord) []AgentShare {
	groups := shareBy(records, func(r SpendRecord) string { return r.AgentID }, true)
	out := make([]AgentShare, len(groups))
	for i, g := range groups {
		out[i] = AgentShare{AgentID: g.Key, USD: g.USD, Credits: g.Credits, Fraction: g.Fraction}
	}
	return out
}

// WorkflowShare is one workflow run-definition's slice of workflow-attributed
// spend.
type WorkflowShare struct {
	WorkflowID string
	USD        float64
	Credits    float64
	Fraction   float64 // share of workflow-attributed spend in [0,1]
}

// WorkflowShares totals spend per workflow across the records that a workflow run
// owned (turns with no workflow are excluded, so the fraction is each workflow's
// share of orchestrated — not all — spend), sorted by spend descending. The
// orchestration-aware half of cost attribution (ADR-0018). Pure: same records →
// same result.
func WorkflowShares(records []SpendRecord) []WorkflowShare {
	groups := shareBy(records, func(r SpendRecord) string { return r.WorkflowID }, false)
	out := make([]WorkflowShare, len(groups))
	for i, g := range groups {
		out[i] = WorkflowShare{WorkflowID: g.Key, USD: g.USD, Credits: g.Credits, Fraction: g.Fraction}
	}
	return out
}

// WriteCSV writes records as CSV (header + one row each) for export. Column
// order is fixed; credits are derived from USD so a spreadsheet needs no
// formula. Deterministic for the same input.
func WriteCSV(w io.Writer, records []SpendRecord) error {
	cw := csv.NewWriter(w)
	// New attribution columns (agent/workflow/lane) are appended at the end so the
	// pre-v2 column positions are unchanged — a backward-compatible header bump
	// (CONTRACTS §3). — ADR-0018.
	if err := cw.Write([]string{"at", "session", "model", "input", "cached", "output", "usd", "credits", "aiu", "agent", "workflow", "lane"}); err != nil {
		return err
	}
	for _, r := range records {
		row := []string{
			r.At.UTC().Format(time.RFC3339),
			r.SessionID,
			r.Model,
			strconv.FormatInt(r.InputTokens, 10),
			strconv.FormatInt(r.CachedTokens, 10),
			strconv.FormatInt(r.OutputTokens, 10),
			csvFloat(r.USD),
			csvFloat(r.Credits()),
			csvFloat(r.AIU),
			r.AgentID,
			r.WorkflowID,
			strconv.Itoa(r.LaneIndex),
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// csvFloat formats a float for CSV export: the shortest exact decimal after
// rounding away sub-micro binary-float noise (e.g. 1.7999999999999998 → "1.8"),
// so a spreadsheet column reads cleanly without losing meaningful precision.
func csvFloat(v float64) string {
	return strconv.FormatFloat(math.Round(v*1e6)/1e6, 'f', -1, 64)
}
