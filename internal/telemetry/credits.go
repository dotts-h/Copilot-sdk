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
	CachedTokens int64
	OutputTokens int64
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
	return cost
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
// AI Credit grant) so the UI can warn before overage.
type Budget struct {
	// AllowanceCredits is the included monthly credit grant.
	AllowanceCredits float64
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

// FormatUSD renders a dollar amount with cent precision.
func FormatUSD(v float64) string { return fmt.Sprintf("$%.4f", v) }

// FormatCredits renders a credit amount with two decimals.
func FormatCredits(v float64) string { return fmt.Sprintf("%.2f cr", v) }
