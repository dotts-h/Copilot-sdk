package telemetry

import (
	"fmt"
	"sync"
	"time"
)

// Usage is a single accounted unit of token consumption, typically emitted once
// per assistant turn by the Copilot runtime.
type Usage struct {
	Model        string
	InputTokens  int64
	CachedTokens int64 // prompt-cache reads
	OutputTokens int64
	// CacheWriteTokens and ReasoningTokens are tracked for display only. GitHub's
	// authoritative AIU already accounts for them, and the estimate prices reads
	// (CachedTokens) and output (which includes reasoning); surfacing the raw
	// counts powers the statusline without affecting the credit math.
	CacheWriteTokens int64
	ReasoningTokens  int64
	// At is when the usage was recorded; zero means "now" at insertion time.
	At time.Time
}

// TotalTokens returns the sum of every metered token category.
func (u Usage) TotalTokens() int64 {
	return u.InputTokens + u.CachedTokens + u.OutputTokens
}

// Cost is the fully-broken-down result of pricing a Usage.
type Cost struct {
	Model     string
	InputUSD  float64
	CachedUSD float64
	OutputUSD float64
	// Matched reports whether the model had an exact price-book entry.
	Matched bool
}

// USD is the total dollar cost.
func (c Cost) USD() float64 { return c.InputUSD + c.CachedUSD + c.OutputUSD }

// Credits is the cost expressed in GitHub AI Credits (1 credit = $0.01).
func (c Cost) Credits() float64 { return c.USD() / USDPerCredit }

// Price computes the cost of a single Usage against a price book. It never
// panics: a nil price book prices everything at zero.
func Price(pb *PriceBook, u Usage) Cost {
	rate, matched := pb.Rate(u.Model)
	c := Cost{
		Model:     u.Model,
		InputUSD:  float64(u.InputTokens) * rate.InputPerMTok / 1_000_000,
		CachedUSD: float64(u.CachedTokens) * rate.CachedInputPerMTok / 1_000_000,
		OutputUSD: float64(u.OutputTokens) * rate.OutputPerMTok / 1_000_000,
		Matched:   matched,
	}
	return c
}

// EstimateTurn gives a pre-flight estimate of the next turn's cost by pricing
// the current context-window fill as fresh input at the model's rate. Output is
// unknown before the turn runs, so the estimate is the knowable input cost of
// resending the accumulated context — enough to make an abort decision informed.
// It is pure (same inputs → same Cost) and never panics: a nil price book or a
// non-positive context prices at zero.
func EstimateTurn(pb *PriceBook, model string, contextTokens int64) Cost {
	if contextTokens < 0 {
		contextTokens = 0
	}
	return Price(pb, Usage{Model: model, InputTokens: contextTokens})
}

// Meter accumulates usage across a session in a goroutine-safe way and exposes
// running totals in tokens, USD, and credits. It is the single source of truth
// for the telemetry dashboard.
type Meter struct {
	mu          sync.RWMutex
	pb          *PriceBook
	events      []Usage
	perModel    map[string]*ModelTotals
	totalCost   Cost
	reportedAIU float64 // GitHub-authoritative cost, in AI units

	// Display-only running counts (not priced; see Usage).
	cacheWriteTokens int64
	reasoningTokens  int64
}

// RecordReportedAIU folds GitHub's own authoritative per-call cost (already
// converted from nano-AIU to AIU) into the running reported total. This is the
// ground truth the estimate is validated against.
func (m *Meter) RecordReportedAIU(aiu float64) {
	if aiu == 0 {
		return
	}
	m.mu.Lock()
	m.reportedAIU += aiu
	m.mu.Unlock()
}

// ReportedAIU returns the accumulated GitHub-authoritative cost in AI units.
// It is zero when the runtime never reported usage (e.g. the offline mock).
func (m *Meter) ReportedAIU() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.reportedAIU
}

// HasReported reports whether the runtime has reported any authoritative cost on this
// meter — i.e. whether ActualCredits reads the reported figure rather than the
// price-book estimate. — ADR-0033.
func (m *Meter) HasReported() bool { return HasReported(m.ReportedAIU()) }

// ActualCredits returns this meter's actual spend on the authoritative-cost-first
// basis (ADR-0033): the reported AIU (1 AIU = 1 credit) once the runtime has reported,
// else the price-book estimate from Totals. The estimate stays the pre-flight/forecast
// figure and the offline fallback — never the truth for actual spend when a reported
// figure is in hand.
func (m *Meter) ActualCredits() float64 {
	return ActualCredits(m.Totals().Credits(), m.ReportedAIU())
}

// ModelTotals aggregates usage and cost for one model.
type ModelTotals struct {
	Model        string
	InputTokens  int64
	CachedTokens int64
	OutputTokens int64
	InputUSD     float64
	CachedUSD    float64
	OutputUSD    float64
	Turns        int64
}

// USD totals the dollar cost for the model.
func (m ModelTotals) USD() float64 { return m.InputUSD + m.CachedUSD + m.OutputUSD }

// Credits totals the credit cost for the model.
func (m ModelTotals) Credits() float64 { return m.USD() / USDPerCredit }

// NewMeter constructs a Meter bound to a price book. A nil price book is
// replaced with DefaultPriceBook so the meter is always usable.
func NewMeter(pb *PriceBook) *Meter {
	if pb == nil {
		pb = DefaultPriceBook()
	}
	return &Meter{pb: pb, perModel: make(map[string]*ModelTotals)}
}

// PriceBook returns the meter's price book so a sibling meter — e.g. a per-
// session scope built alongside the account-wide one — can price against the
// same rates and stay consistent to the cent. The book is shared, not copied.
func (m *Meter) PriceBook() *PriceBook { return m.pb }

// Record prices a usage event and folds it into the running totals, returning
// the cost of just that event.
func (m *Meter) Record(u Usage) Cost {
	if u.At.IsZero() {
		u.At = time.Now()
	}
	cost := Price(m.pb, u)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, u)

	mt := m.perModel[u.Model]
	if mt == nil {
		mt = &ModelTotals{Model: u.Model}
		m.perModel[u.Model] = mt
	}
	mt.InputTokens += u.InputTokens
	mt.CachedTokens += u.CachedTokens
	mt.OutputTokens += u.OutputTokens
	mt.InputUSD += cost.InputUSD
	mt.CachedUSD += cost.CachedUSD
	mt.OutputUSD += cost.OutputUSD
	mt.Turns++

	m.totalCost.InputUSD += cost.InputUSD
	m.totalCost.CachedUSD += cost.CachedUSD
	m.totalCost.OutputUSD += cost.OutputUSD

	m.cacheWriteTokens += u.CacheWriteTokens
	m.reasoningTokens += u.ReasoningTokens
	return cost
}

// ExtraTokens returns the display-only running counts that fall outside the
// priced input/cached/output categories: prompt-cache writes and reasoning
// ("thinking") tokens.
func (m *Meter) ExtraTokens() (cacheWrite, reasoning int64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cacheWriteTokens, m.reasoningTokens
}

// EstimateTurn prices the next turn against the meter's price book. See the
// package-level EstimateTurn for the model; the meter holds the (overridable)
// rates so the estimate tracks any settings-page price changes.
func (m *Meter) EstimateTurn(model string, contextTokens int64) Cost {
	return EstimateTurn(m.pb, model, contextTokens)
}

// Totals returns the aggregate cost across all models.
func (m *Meter) Totals() Cost {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c := m.totalCost
	c.Model = "all"
	c.Matched = true
	return c
}

// TotalTokens returns the summed input, cached, and output token counts.
func (m *Meter) TotalTokens() (input, cached, output int64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, mt := range m.perModel {
		input += mt.InputTokens
		cached += mt.CachedTokens
		output += mt.OutputTokens
	}
	return
}

// ByModel returns a snapshot of per-model totals, sorted by descending credit
// cost so the biggest spenders surface first.
func (m *Meter) ByModel() []ModelTotals {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ModelTotals, 0, len(m.perModel))
	for _, mt := range m.perModel {
		out = append(out, *mt)
	}
	// Simple insertion-style sort by credits desc (small N).
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Credits() > out[j-1].Credits(); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// Count returns how many usage events have been recorded.
func (m *Meter) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.events)
}

// Budget tracks consumption against a credit allowance (e.g. a plan's monthly
// AI Credit grant) so the UI can warn before overage and, optionally, pause a
// turn that would breach a hard ceiling.
type Budget struct {
	// AllowanceCredits is the included monthly credit grant.
	AllowanceCredits float64
	// WarnFraction is the soft threshold: once used/allowance crosses it the UI
	// raises an ambient warning. Zero (or no allowance) disables the soft warn.
	WarnFraction float64
	// HardCapCredits is an absolute ceiling in credits; a turn whose projected
	// spend would exceed it is paused for confirmation. Zero disables the cap.
	HardCapCredits float64
}

// Remaining returns credits left in the allowance (may be negative on overage).
func (b Budget) Remaining(used float64) float64 { return b.AllowanceCredits - used }

// FractionUsed returns used/allowance in [0, +inf). A zero allowance yields +Inf
// for any positive usage and 0 for no usage, which the UI renders as "no budget".
func (b Budget) FractionUsed(used float64) float64 {
	if b.AllowanceCredits == 0 {
		if used == 0 {
			return 0
		}
		return 1e9
	}
	return used / b.AllowanceCredits
}

// Warned reports whether spend has crossed the soft warning threshold. It is
// false when no allowance or no warn fraction is configured, so a missing budget
// never nags. Pure: same inputs → same answer.
func (b Budget) Warned(used float64) bool {
	if b.AllowanceCredits <= 0 || b.WarnFraction <= 0 {
		return false
	}
	return used/b.AllowanceCredits >= b.WarnFraction
}

// CapExceeded reports whether a projected spend would breach the hard cap. A
// zero cap disables the ceiling, so nothing is ever gated. The comparison is
// strict (>) so a projection exactly at the cap is still allowed.
func (b Budget) CapExceeded(projected float64) bool {
	if b.HardCapCredits <= 0 {
		return false
	}
	return projected > b.HardCapCredits
}

// FormatUSD renders a dollar amount with sub-cent (4-decimal) precision — per-turn
// costs are often a fraction of a cent, and the CSV export relies on this width.
func FormatUSD(v float64) string { return fmt.Sprintf("$%.4f", v) }

// FormatCredits renders a credit amount with two decimals.
func FormatCredits(v float64) string { return fmt.Sprintf("%.2f cr", v) }
