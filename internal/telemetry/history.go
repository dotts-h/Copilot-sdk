// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package telemetry

import (
	"encoding/csv"
	"io"
	"math"
	"sort"
	"strconv"
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
	// CacheWriteTokens is prompt-cache write tokens, priced into USD (ADR-0034);
	// ReasoningTokens is the display-only subset of OutputTokens. Both are additive,
	// schema-v3 fields: older records read back 0 and older readers ignore them, so
	// the ledger stays backward-readable (like the v2 attribution tags). They let
	// the all-time per-model breakdown surface cache-write/reasoning from history.
	CacheWriteTokens int64 `json:"cw,omitempty"`
	ReasoningTokens  int64 `json:"reasoning,omitempty"`
	// USD is the estimate-priced dollar cost of this turn (the Meter's number),
	// inclusive of the cache-write charge (ADR-0034).
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
	// SubagentID/SubagentName attribute the turn to the sub-agent INSTANCE that
	// spent it (empty = a root/persona turn, not a sub-agent). A sub-agent's spend
	// is the sub-agent's, not the agent that spawned it, so AgentShares excludes a
	// sub-agent-tagged record from the agent buckets (no double-count) and
	// SubagentShares rolls it up on its own. The name is carried so the Telemetry
	// breakdown labels the row even after a restart drops the live registry. Both
	// are additive, schema-v4 tags (older readers ignore them; older records read
	// back empty) — the same ADR-0018 discipline (epic 0069 S3, issue 0072).
	SubagentID   string `json:"sub,omitempty"`
	SubagentName string `json:"subname,omitempty"`
}

// Credits expresses the record's estimate-priced USD cost in GitHub AI Credits
// (1 cr = $0.01). It is the price-book *estimate*, not the actual spend — prefer
// ActualCredits where what the turn really cost matters (ADR-0033). Retained for the
// aggregation readers (DailyTotals/shares) that bucket estimate-priced USD.
func (r SpendRecord) Credits() float64 { return r.USD / USDPerCredit }

// EstimateCredits is the record's price-book estimate in credits — Credits under its
// authoritative-cost-first name (ADR-0033), so a reader's intent is explicit.
func (r SpendRecord) EstimateCredits() float64 { return r.Credits() }

// HasReported reports whether the runtime reported this turn's authoritative cost
// (a non-zero AIU), so ActualCredits reads the reported figure not the estimate.
func (r SpendRecord) HasReported() bool { return HasReported(r.AIU) }

// ActualCredits is the record's actual spend on the authoritative-cost-first basis
// (ADR-0033): the reported AIU (1 AIU = 1 credit) when the runtime reported it, else
// the price-book EstimateCredits as the offline fallback.
func (r SpendRecord) ActualCredits() float64 { return ActualCredits(r.EstimateCredits(), r.AIU) }

// Day is the record's UTC calendar day (YYYY-MM-DD), the bucket key for trends.
func (r SpendRecord) Day() string { return r.At.UTC().Format("2006-01-02") }

const (
	spendFile = "spend.json"
	// SpendSchemaVersion is the on-disk schema version. Bumps must keep the
	// "records" array readable by older code (additive fields only) or ship a
	// migration — see CONTRACTS.md §4 and docs/REGRESSIONS.md. v2 added the
	// additive agent/workflow/lane attribution tags (ADR-0018); v3 added the
	// additive cache-write/reasoning token counts (ADR-0034); v4 added the additive
	// sub-agent id/name attribution tags (issue 0072). Each is additive: older
	// readers ignore the new keys and tolerate the higher version, and older records
	// read back with the new fields zero.
	SpendSchemaVersion = 4
)

// SpendStore is the persisted spend ledger: a thin typed wrapper over the shared
// AppendOnlyStore[SpendRecord] (store.go). It is goroutine-safe and atomic on
// write. A store with an empty dir is ephemeral (in-memory only) — used by the
// offline demo and tests so they never touch a real config directory. Append,
// Records, and Count are promoted from the embedded generic store; the on-disk
// envelope (`{"version":2,"records":[…]}`) is unchanged — the stable contract.
type SpendStore struct {
	*AppendOnlyStore[SpendRecord]
}

// LoadSpendStore reads the ledger from dir/spend.json, returning an empty store
// when the file is absent (first run) and an error when it is present but
// unparseable. An empty dir yields an ephemeral, in-memory-only store. Append
// stamps At with the current time when zero (the per-record hook), and a v1 file
// without the v2 attribution tags loads unchanged (the tags are additive, so
// older records simply read back empty — no migration code, just additive JSON).
func LoadSpendStore(dir string) (*SpendStore, error) {
	inner, err := loadAppendOnlyStore(dir, spendFile, "records", "spend history", SpendSchemaVersion, stampSpend)
	if err != nil {
		return nil, err
	}
	return &SpendStore{inner}, nil
}

// stampSpend defaults a record's At to now when the caller left it zero — the
// spend store's per-record Append hook.
func stampSpend(r SpendRecord) SpendRecord {
	if r.At.IsZero() {
		r.At = time.Now()
	}
	return r
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
	Count    int     // number of records folded into this group
}

// shareBy totals spend per group (keyed by keyOf) and computes each group's
// fraction of the included whole, sorted by spend descending (ties broken by key
// for determinism). When includeEmpty is false, records whose key is "" are
// skipped entirely and excluded from the total — used by WorkflowShares so the
// "no workflow" chat spend doesn't swamp the per-workflow view. Pure: same
// records → same result.
func shareBy(records []SpendRecord, keyOf func(SpendRecord) string, includeEmpty bool) []share {
	by := map[string]float64{}
	cnt := map[string]int{}
	var total float64
	for _, r := range records {
		k := keyOf(r)
		if k == "" && !includeEmpty {
			continue
		}
		by[k] += r.USD
		cnt[k]++
		total += r.USD
	}
	out := make([]share, 0, len(by))
	for k, usd := range by {
		s := share{Key: k, USD: usd, Credits: usd / USDPerCredit, Count: cnt[k]}
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
// (ADR-0018). Sub-agent-tagged turns are excluded: a sub-agent's spend is the
// sub-agent's, not the agent that spawned it, so it neither inflates the root
// (empty-agent) bucket nor any persona's — it rolls up in SubagentShares instead,
// no double-count (issue 0072). Pure: same records → same result.
func AgentShares(records []SpendRecord) []AgentShare {
	groups := shareBy(rootTurns(records), func(r SpendRecord) string { return r.AgentID }, true)
	out := make([]AgentShare, len(groups))
	for i, g := range groups {
		out[i] = AgentShare{AgentID: g.Key, USD: g.USD, Credits: g.Credits, Fraction: g.Fraction}
	}
	return out
}

// rootTurns filters out sub-agent-attributed records, leaving the turns the root
// or a persona spent directly — so the agent view doesn't double-count spend the
// SubagentShares view already attributes (issue 0072).
func rootTurns(records []SpendRecord) []SpendRecord {
	out := make([]SpendRecord, 0, len(records))
	for _, r := range records {
		if r.SubagentID == "" {
			out = append(out, r)
		}
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

// SubagentShare is one sub-agent instance's slice of sub-agent-attributed spend.
// SubagentName is the instance's display name as captured on the record, so the
// breakdown labels the row even after a restart drops the live registry.
type SubagentShare struct {
	SubagentID   string
	SubagentName string
	USD          float64
	Credits      float64
	Fraction     float64 // share of sub-agent-attributed spend in [0,1]
}

// SubagentShares totals spend per sub-agent instance across the records a
// sub-agent owned (turns with no sub-agent are excluded, so the fraction is each
// instance's share of sub-agent — not all — spend), sorted by spend descending.
// The cost-awareness half of the sub-agent epic: answers "which sub-agent is
// burning my budget?" without inflating the root agent's bucket (issue 0072).
// Pure: same records → same result.
func SubagentShares(records []SpendRecord) []SubagentShare {
	groups := shareBy(records, func(r SpendRecord) string { return r.SubagentID }, false)
	// Carry a display name per instance id (last non-empty wins) so the breakdown
	// labels each row from the ledger alone — the live registry doesn't survive a
	// restart, but the record's name does.
	names := map[string]string{}
	for _, r := range records {
		if r.SubagentID != "" && r.SubagentName != "" {
			names[r.SubagentID] = r.SubagentName
		}
	}
	out := make([]SubagentShare, len(groups))
	for i, g := range groups {
		out[i] = SubagentShare{
			SubagentID: g.Key, SubagentName: names[g.Key],
			USD: g.USD, Credits: g.Credits, Fraction: g.Fraction,
		}
	}
	return out
}

// SessionShare is one copilot session's slice of the persisted spend: its total
// credits and the number of metered turns rolled up under its id.
type SessionShare struct {
	SessionID string
	Credits   float64
	Turns     int // metered turns recorded under this session id
}

// SessionShares totals spend per copilot session id (turn count + credits),
// sorted by credits descending (ties broken by session id ascending) for
// determinism. Records with no session id are **excluded** (includeEmpty=false,
// like WorkflowShares): a session row needs a real id to join against, and the
// picker only lists real sessions, so a pre-attribution v1 row — or a turn
// recorded before a copilot session bound — has nothing to attach to. Powers the
// cost-aware Sessions picker (G2). Pure: same records → same result.
func SessionShares(records []SpendRecord) []SessionShare {
	groups := shareBy(records, func(r SpendRecord) string { return r.SessionID }, false)
	out := make([]SessionShare, len(groups))
	for i, g := range groups {
		out[i] = SessionShare{SessionID: g.Key, Credits: g.Credits, Turns: g.Count}
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
	if err := cw.Write([]string{"at", "session", "model", "input", "cached", "output", "usd", "credits", "aiu", "agent", "workflow", "lane", "cacheWrite", "reasoning", "subagent", "subagentName"}); err != nil {
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
			strconv.FormatInt(r.CacheWriteTokens, 10),
			strconv.FormatInt(r.ReasoningTokens, 10),
			r.SubagentID,
			r.SubagentName,
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
