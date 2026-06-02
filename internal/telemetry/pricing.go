// Package telemetry models GitHub Copilot's usage-based (AI Credits) billing so
// my-orchestra can show a faithful, real-time estimate of the credits a session
// consumes.
//
// Billing model (effective 2026-06-01):
//   - Usage is metered on tokens: input, cached-input, and output.
//   - Each model has per-million-token USD rates.
//   - Cost in USD is converted to AI Credits at a fixed rate of 1 credit = $0.01.
//   - Code completions / next-edit suggestions are free and never metered here.
//
// All rates are data, not code: callers may override the price book at runtime
// (e.g. from a settings page) without recompiling.
package telemetry

import (
	"fmt"
	"sort"
	"strings"
)

// USDPerCredit is GitHub's fixed conversion: one AI Credit costs one US cent.
const USDPerCredit = 0.01

// ModelRate captures the per-million-token USD prices for a single model, plus
// the legacy premium-request multiplier GitHub still surfaces for reference.
type ModelRate struct {
	// Model is the canonical model identifier (e.g. "gpt-5", "claude-opus-4.7").
	Model string
	// InputPerMTok is USD charged per 1,000,000 fresh input tokens.
	InputPerMTok float64
	// CachedInputPerMTok is USD per 1,000,000 cached (reused) input tokens.
	CachedInputPerMTok float64
	// OutputPerMTok is USD per 1,000,000 generated output tokens.
	OutputPerMTok float64
	// PremiumMultiplier is the legacy premium-request weight, kept for display
	// parity with GitHub's older meter. It does not affect credit math.
	PremiumMultiplier float64
}

// PriceBook maps model identifiers to their rates. The zero value is not
// useful; use DefaultPriceBook or NewPriceBook.
type PriceBook struct {
	rates    map[string]ModelRate
	fallback ModelRate
}

// NewPriceBook builds a price book from the given rates. The fallback is used
// (with its rate values) whenever a queried model is not present, so the engine
// degrades gracefully instead of charging zero for unknown models.
func NewPriceBook(fallback ModelRate, rates ...ModelRate) *PriceBook {
	pb := &PriceBook{rates: make(map[string]ModelRate, len(rates)), fallback: fallback}
	for _, r := range rates {
		pb.rates[normalizeModel(r.Model)] = r
	}
	return pb
}

// DefaultPriceBook returns a price book seeded with representative rates for the
// models GitHub Copilot exposes. Rates approximate GitHub's published API
// pricing and are intended to be overridable from the settings page.
func DefaultPriceBook() *PriceBook {
	return NewPriceBook(
		// Fallback: priced like a mid-tier model so unknown models are never free.
		ModelRate{Model: "(unknown)", InputPerMTok: 2.50, CachedInputPerMTok: 0.25, OutputPerMTok: 10.00, PremiumMultiplier: 1},
		ModelRate{Model: "gpt-5", InputPerMTok: 1.25, CachedInputPerMTok: 0.125, OutputPerMTok: 10.00, PremiumMultiplier: 1},
		ModelRate{Model: "gpt-5-mini", InputPerMTok: 0.25, CachedInputPerMTok: 0.025, OutputPerMTok: 2.00, PremiumMultiplier: 0},
		ModelRate{Model: "gpt-5.4", InputPerMTok: 1.25, CachedInputPerMTok: 0.125, OutputPerMTok: 10.00, PremiumMultiplier: 6},
		ModelRate{Model: "claude-sonnet-4.6", InputPerMTok: 3.00, CachedInputPerMTok: 0.30, OutputPerMTok: 15.00, PremiumMultiplier: 1},
		ModelRate{Model: "claude-opus-4.7", InputPerMTok: 15.00, CachedInputPerMTok: 1.50, OutputPerMTok: 75.00, PremiumMultiplier: 27},
		ModelRate{Model: "o4-mini", InputPerMTok: 1.10, CachedInputPerMTok: 0.275, OutputPerMTok: 4.40, PremiumMultiplier: 1},
		ModelRate{Model: "gemini-2.5-pro", InputPerMTok: 1.25, CachedInputPerMTok: 0.31, OutputPerMTok: 10.00, PremiumMultiplier: 1},
	)
}

// Rate returns the rate for a model, falling back to the configured fallback
// (with its Model field swapped to the requested name) when not found. The
// second return value reports whether an exact match was found.
func (pb *PriceBook) Rate(model string) (ModelRate, bool) {
	if pb == nil {
		return ModelRate{}, false
	}
	if r, ok := pb.rates[normalizeModel(model)]; ok {
		return r, true
	}
	fb := pb.fallback
	fb.Model = model
	return fb, false
}

// Set inserts or replaces a rate. It is safe to call from a settings page.
func (pb *PriceBook) Set(r ModelRate) {
	if pb.rates == nil {
		pb.rates = make(map[string]ModelRate)
	}
	pb.rates[normalizeModel(r.Model)] = r
}

// Models returns the known model identifiers in stable, sorted order.
func (pb *PriceBook) Models() []string {
	out := make([]string, 0, len(pb.rates))
	for _, r := range pb.rates {
		out = append(out, r.Model)
	}
	sort.Strings(out)
	return out
}

// normalizeModel canonicalizes a model identifier so "GPT-5", "gpt_5" and
// "gpt-5" all resolve to the same entry.
func normalizeModel(m string) string {
	m = strings.ToLower(strings.TrimSpace(m))
	m = strings.ReplaceAll(m, "_", "-")
	m = strings.ReplaceAll(m, " ", "-")
	return m
}

// String renders a rate compactly for diagnostics.
func (r ModelRate) String() string {
	return fmt.Sprintf("%s in=$%.4f/Mt cached=$%.4f/Mt out=$%.4f/Mt x%.1f",
		r.Model, r.InputPerMTok, r.CachedInputPerMTok, r.OutputPerMTok, r.PremiumMultiplier)
}
