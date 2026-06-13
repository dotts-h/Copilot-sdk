// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

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
	"sync"
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
	// CacheWritePerMTok is USD per 1,000,000 prompt-cache *write* tokens — a
	// category distinct from fresh input and cached (read) input, additive to the
	// bill (Anthropic ≈ 1.25× input). Zero means "derive 1.25× input" at price-book
	// construction (see NewPriceBook); an explicit non-zero value pins it. Reasoning
	// ("thinking") tokens are a subset of output and already priced at OutputPerMTok,
	// so there is no reasoning rate — charging one would double-count. — ADR-0034.
	CacheWritePerMTok float64
	// PremiumMultiplier is the legacy premium-request weight, kept for display
	// parity with GitHub's older meter. It does not affect credit math.
	PremiumMultiplier float64
}

// DefaultCacheWriteMultiplier is the confirmed cache-write (cache-creation) rate
// as a multiple of the model's base input rate (Anthropic ≈ 1.25× input). It is
// the default whenever a rate carries no explicit CacheWritePerMTok. — ADR-0034.
const DefaultCacheWriteMultiplier = 1.25

// withDerivedCacheWrite fills a rate's cache-write price from the 1.25× input
// default when it was left unset (zero), so every rate in a book prices cache-write
// rather than treating it as free. An explicit (non-zero) value is preserved.
func withDerivedCacheWrite(r ModelRate) ModelRate {
	if r.CacheWritePerMTok == 0 {
		r.CacheWritePerMTok = DefaultCacheWriteMultiplier * r.InputPerMTok
	}
	return r
}

// PriceBook maps model identifiers to their rates. The zero value is not
// useful; use DefaultPriceBook or NewPriceBook.
//
// A book is shared by reference across the account meter and every per-session
// meter (each built on the account meter's book — see web.Hub.newSession). Its
// contents (rates + fallback) are guarded by an internal RWMutex: reads (Rate,
// Models, and so the Price/EstimateTurn pricing path) take the read lock and a
// live settings-page reprice (Replace) takes the write lock, so a reprice is
// race-safe against concurrent pricing. Always use it via *PriceBook (never copy a
// PriceBook by value — the mutex makes a value copy unsafe).
type PriceBook struct {
	mu       sync.RWMutex
	rates    map[string]ModelRate
	fallback ModelRate
}

// NewPriceBook builds a price book from the given rates. The fallback is used
// (with its rate values) whenever a queried model is not present, so the engine
// degrades gracefully instead of charging zero for unknown models.
func NewPriceBook(fallback ModelRate, rates ...ModelRate) *PriceBook {
	pb := &PriceBook{rates: make(map[string]ModelRate, len(rates)), fallback: withDerivedCacheWrite(fallback)}
	for _, r := range rates {
		pb.rates[normalizeModel(r.Model)] = withDerivedCacheWrite(r)
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
	pb.mu.RLock()
	defer pb.mu.RUnlock()
	if r, ok := pb.rates[normalizeModel(model)]; ok {
		return r, true
	}
	fb := pb.fallback
	fb.Model = model
	return fb, false
}

// Set inserts or replaces a rate. It is safe to call from a settings page.
func (pb *PriceBook) Set(r ModelRate) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	if pb.rates == nil {
		pb.rates = make(map[string]ModelRate)
	}
	pb.rates[normalizeModel(r.Model)] = r
}

// Models returns the known model identifiers in stable, sorted order.
func (pb *PriceBook) Models() []string {
	pb.mu.RLock()
	out := make([]string, 0, len(pb.rates))
	for _, r := range pb.rates {
		out = append(out, r.Model)
	}
	pb.mu.RUnlock()
	sort.Strings(out)
	return out
}

// Replace atomically swaps this book's contents (rates + fallback) for src's, in
// place — so every meter that shares this *PriceBook by reference (the account
// meter and all per-session meters) reprices the next turn at once, without a
// restart and without drift. It is the live-apply seam the Settings price-override
// editor uses: the web layer builds a *fresh* book from BuildPriceBook (defaults +
// the saved overrides) and Replaces the live book with it. Because the source is
// always rebuilt from defaults, an override that was removed reverts to its default
// (the rebuild-not-incremental guarantee — see BuildPriceBook). A nil src is a
// no-op. src is locked before pb; src is always a freshly-built, unshared book, so
// there is no lock-ordering risk.
func (pb *PriceBook) Replace(src *PriceBook) {
	if pb == nil || src == nil || pb == src {
		return // a self-replace would deadlock (RLock then Lock the same mutex)
	}
	src.mu.RLock()
	rates := make(map[string]ModelRate, len(src.rates))
	for k, v := range src.rates {
		rates[k] = v
	}
	fb := src.fallback
	src.mu.RUnlock()

	pb.mu.Lock()
	pb.rates = rates
	pb.fallback = fb
	pb.mu.Unlock()
}

// BuildPriceBook returns a fresh price book seeded from DefaultPriceBook and then
// layered with the given per-model overrides (model → [input, cached, output] and
// an OPTIONAL 4th element, the cache-write rate — all USD per million tokens). A
// 3-element override derives cache-write at the 1.25× default off the *overridden*
// input, so a legacy (pre-0059) override still prices cache-write coherently; a
// 4-element override pins it explicitly (ADR-0034). It always starts from the
// defaults, so the result reflects exactly the supplied overrides: a model absent
// from overrides prices at its default, and an override removed between two builds
// reverts to the default (rebuild-not-incremental — the reason a live reprice
// rebuilds rather than calling Set on the existing book, which would never reset a
// removed/lowered override). An override with fewer than 3 elements is ignored
// (validation rejects it upstream — see config.Validate). It is pure (same
// overrides → same book) and dependency-free. Mirrors the startup price-book build
// in internal/bootstrap.
func BuildPriceBook(overrides map[string][]float64) *PriceBook {
	pb := DefaultPriceBook()
	for model, r := range overrides {
		if len(r) < 3 {
			continue
		}
		// Start from the model's default rate so an override of the prices preserves
		// the rest (e.g. the display-only PremiumMultiplier) rather than resetting it
		// to zero; an unknown model inherits the fallback's fields.
		rate, _ := pb.Rate(model)
		rate.Model = model
		rate.InputPerMTok, rate.CachedInputPerMTok, rate.OutputPerMTok = r[0], r[1], r[2]
		if len(r) >= 4 {
			rate.CacheWritePerMTok = r[3]
		} else {
			// Re-derive from the new input rate (clearing any inherited default so
			// withDerivedCacheWrite recomputes 1.25× the overridden input).
			rate.CacheWritePerMTok = 0
			rate = withDerivedCacheWrite(rate)
		}
		pb.Set(rate)
	}
	return pb
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
