// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package telemetry

import (
	"math"
	"strings"
	"sync"
	"testing"
)

func approx(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("got %.12f, want %.12f", got, want)
	}
}

func TestMeterPriceBookExposed(t *testing.T) {
	pb := DefaultPriceBook()
	m := NewMeter(pb)
	// A sibling meter (e.g. a per-session scope) must price against the same book
	// so its credits/estimate match the account-wide meter to the cent.
	if m.PriceBook() != pb {
		t.Fatal("PriceBook must return the meter's own book so a scoped meter shares rates")
	}
	// A nil book is replaced with the default, and that default is exposed.
	if NewMeter(nil).PriceBook() == nil {
		t.Fatal("a nil-constructed meter must still expose a usable price book")
	}
}

func TestPriceExactModel(t *testing.T) {
	pb := DefaultPriceBook()
	// gpt-5: in $1.25/Mt, cached $0.125/Mt, out $10/Mt.
	c := Price(pb, Usage{Model: "gpt-5", InputTokens: 1_000_000, OutputTokens: 1_000_000})
	if !c.Matched {
		t.Fatal("expected exact match for gpt-5")
	}
	approx(t, c.InputUSD, 1.25)
	approx(t, c.OutputUSD, 10.0)
	approx(t, c.USD(), 11.25)
	// 1 credit = $0.01, so $11.25 == 1125 credits.
	approx(t, c.Credits(), 1125.0)
}

func TestPriceCachedTokens(t *testing.T) {
	pb := DefaultPriceBook()
	c := Price(pb, Usage{Model: "gpt-5", CachedTokens: 2_000_000})
	approx(t, c.CachedUSD, 0.25) // 2M * $0.125/Mt
	approx(t, c.InputUSD, 0)
	approx(t, c.OutputUSD, 0)
}

func TestPriceUnknownModelUsesFallback(t *testing.T) {
	pb := DefaultPriceBook()
	c := Price(pb, Usage{Model: "totally-made-up", InputTokens: 1_000_000})
	if c.Matched {
		t.Fatal("did not expect a match for an unknown model")
	}
	// Fallback input rate is $2.50/Mt.
	approx(t, c.InputUSD, 2.50)
	if c.Model != "totally-made-up" {
		t.Fatalf("fallback should preserve requested model name, got %q", c.Model)
	}
}

func TestNilPriceBookIsZero(t *testing.T) {
	var pb *PriceBook
	c := Price(pb, Usage{Model: "gpt-5", InputTokens: 5_000_000, OutputTokens: 5_000_000})
	if c.USD() != 0 || c.Credits() != 0 {
		t.Fatalf("nil price book should price to zero, got %v", c)
	}
}

func TestModelNormalization(t *testing.T) {
	pb := DefaultPriceBook()
	for _, name := range []string{"GPT-5", "gpt_5", " gpt-5 ", "Gpt 5"} {
		r, ok := pb.Rate(name)
		if !ok {
			t.Fatalf("expected %q to normalize to gpt-5", name)
		}
		approx(t, r.InputPerMTok, 1.25)
	}
}

func TestPriceBookSetAndOverride(t *testing.T) {
	pb := DefaultPriceBook()
	pb.Set(ModelRate{Model: "gpt-5", InputPerMTok: 99, OutputPerMTok: 0, CachedInputPerMTok: 0})
	c := Price(pb, Usage{Model: "gpt-5", InputTokens: 1_000_000})
	approx(t, c.InputUSD, 99)
}

func TestBuildPriceBookAppliesOverridesOverDefaults(t *testing.T) {
	pb := BuildPriceBook(map[string][]float64{
		"gpt-5": {99, 9, 88},
	})
	// The overridden model prices at the new rate.
	gpt := Price(pb, Usage{Model: "gpt-5", InputTokens: 1_000_000, CachedTokens: 1_000_000, OutputTokens: 1_000_000})
	approx(t, gpt.InputUSD, 99)
	approx(t, gpt.CachedUSD, 9)
	approx(t, gpt.OutputUSD, 88)
	// A non-overridden model prices at its built-in default (claude-opus-4.7 = $15/Mt in).
	opus := Price(pb, Usage{Model: "claude-opus-4.7", InputTokens: 1_000_000})
	approx(t, opus.InputUSD, 15)
}

func TestBuildPriceBookPreservesNonPriceFields(t *testing.T) {
	// Overriding the three prices must not reset the model's other rate fields
	// (e.g. the display-only PremiumMultiplier) to zero.
	pb := BuildPriceBook(map[string][]float64{"claude-opus-4.7": {20, 2, 100}})
	r, ok := pb.Rate("claude-opus-4.7")
	if !ok {
		t.Fatal("overridden model should still be a known rate")
	}
	approx(t, r.InputPerMTok, 20)
	if r.PremiumMultiplier != 27 {
		t.Errorf("PremiumMultiplier reset by override: got %v, want the default 27", r.PremiumMultiplier)
	}
}

func TestBuildPriceBookRemovedOverrideRevertsToDefault(t *testing.T) {
	// Rebuild-not-incremental: a model overridden in one build but absent from the
	// next reverts to its default, because each build starts from DefaultPriceBook.
	withOverride := BuildPriceBook(map[string][]float64{"gpt-5": {99, 9, 88}})
	approx(t, Price(withOverride, Usage{Model: "gpt-5", InputTokens: 1_000_000}).InputUSD, 99)

	withoutOverride := BuildPriceBook(nil)
	approx(t, Price(withoutOverride, Usage{Model: "gpt-5", InputTokens: 1_000_000}).InputUSD, 1.25)
}

func TestPriceBookReplaceRepricesSharedMeters(t *testing.T) {
	// The account meter and a per-session meter share one *PriceBook by reference
	// (web.Hub.newSession builds the session meter on the account book). Replacing
	// the shared book's contents in place must reprice BOTH at once — no drift.
	live := DefaultPriceBook()
	account := NewMeter(live)
	session := NewMeter(live) // shares the same book, as the hub does

	live.Replace(BuildPriceBook(map[string][]float64{"gpt-5": {99, 9, 88}}))

	approx(t, account.EstimateTurn("gpt-5", 1_000_000).InputUSD, 99)
	approx(t, session.EstimateTurn("gpt-5", 1_000_000).InputUSD, 99)

	// Removing the override and replacing again reverts both to the default.
	live.Replace(BuildPriceBook(nil))
	approx(t, account.EstimateTurn("gpt-5", 1_000_000).InputUSD, 1.25)
	approx(t, session.EstimateTurn("gpt-5", 1_000_000).InputUSD, 1.25)
}

func TestFormatHelpers(t *testing.T) {
	if got := FormatUSD(1.23456); got != "$1.2346" {
		t.Errorf("FormatUSD = %q, want $1.2346 (4-decimal precision)", got)
	}
	if got := FormatCredits(12.5); got != "12.50 cr" {
		t.Errorf("FormatCredits = %q, want '12.50 cr'", got)
	}
}

func TestMeterExtraTokens(t *testing.T) {
	m := NewMeter(DefaultPriceBook())
	m.Record(Usage{Model: "gpt-5", InputTokens: 10, CacheWriteTokens: 7, ReasoningTokens: 5})
	m.Record(Usage{Model: "gpt-5", CacheWriteTokens: 3, ReasoningTokens: 2})
	cw, reasoning := m.ExtraTokens()
	if cw != 10 || reasoning != 7 {
		t.Fatalf("ExtraTokens = (%d, %d), want (10, 7)", cw, reasoning)
	}
}

func TestPriceBookModels(t *testing.T) {
	pb := NewPriceBook(ModelRate{}, ModelRate{Model: "gpt-5"}, ModelRate{Model: "claude-opus"})
	got := pb.Models()
	if len(got) != 2 || got[0] != "claude-opus" || got[1] != "gpt-5" {
		t.Fatalf("Models() = %v, want sorted [claude-opus gpt-5]", got)
	}
}

func TestModelRateString(t *testing.T) {
	r := ModelRate{Model: "gpt-5", InputPerMTok: 1.25, CachedInputPerMTok: 0.125, OutputPerMTok: 10, PremiumMultiplier: 1}
	got := r.String()
	if !strings.Contains(got, "gpt-5") || !strings.Contains(got, "in=$1.2500/Mt") {
		t.Fatalf("ModelRate.String() = %q", got)
	}
}

func TestMeterAccumulates(t *testing.T) {
	m := NewMeter(DefaultPriceBook())
	m.Record(Usage{Model: "gpt-5", InputTokens: 1_000_000, OutputTokens: 1_000_000})
	m.Record(Usage{Model: "gpt-5", InputTokens: 1_000_000})
	if m.Count() != 2 {
		t.Fatalf("expected 2 events, got %d", m.Count())
	}
	in, cached, out := m.TotalTokens()
	if in != 2_000_000 || cached != 0 || out != 1_000_000 {
		t.Fatalf("token totals wrong: in=%d cached=%d out=%d", in, cached, out)
	}
	// $11.25 + $1.25 = $12.50 => 1250 credits.
	approx(t, m.Totals().USD(), 12.50)
	approx(t, m.Totals().Credits(), 1250.0)
}

func TestMeterByModelSortedByCreditsDesc(t *testing.T) {
	m := NewMeter(DefaultPriceBook())
	m.Record(Usage{Model: "gpt-5-mini", InputTokens: 1_000_000})      // cheap
	m.Record(Usage{Model: "claude-opus-4.7", InputTokens: 1_000_000}) // expensive
	m.Record(Usage{Model: "gpt-5", InputTokens: 1_000_000})           // mid
	got := m.ByModel()
	if len(got) != 3 {
		t.Fatalf("expected 3 models, got %d", len(got))
	}
	if got[0].Model != "claude-opus-4.7" {
		t.Fatalf("most expensive model should sort first, got %q", got[0].Model)
	}
	if got[len(got)-1].Model != "gpt-5-mini" {
		t.Fatalf("cheapest model should sort last, got %q", got[len(got)-1].Model)
	}
	// Verify totals are non-decreasing in reverse.
	for i := 1; i < len(got); i++ {
		if got[i].Credits() > got[i-1].Credits() {
			t.Fatalf("not sorted desc at index %d", i)
		}
	}
}

func TestNilMeterPriceBookDefaults(t *testing.T) {
	m := NewMeter(nil)
	c := m.Record(Usage{Model: "gpt-5", InputTokens: 1_000_000})
	approx(t, c.InputUSD, 1.25)
}

func TestBudget(t *testing.T) {
	b := Budget{AllowanceCredits: 1500}
	approx(t, b.Remaining(500), 1000)
	approx(t, b.Remaining(2000), -500) // overage
	approx(t, b.FractionUsed(750), 0.5)

	zero := Budget{AllowanceCredits: 0}
	approx(t, zero.FractionUsed(0), 0)
	if zero.FractionUsed(10) < 1 {
		t.Fatal("zero allowance with usage should report effectively over-budget")
	}
}

func TestBudgetWarned(t *testing.T) {
	b := Budget{AllowanceCredits: 1000, WarnFraction: 0.8}
	if b.Warned(799) {
		t.Error("under the soft threshold should not warn")
	}
	if !b.Warned(800) {
		t.Error("at the soft threshold should warn (>=)")
	}
	if !b.Warned(1200) {
		t.Error("over the allowance should warn")
	}

	// A zero allowance or zero warn fraction disables the soft warn entirely, so a
	// missing budget never nags.
	if (Budget{AllowanceCredits: 0, WarnFraction: 0.8}).Warned(50) {
		t.Error("no allowance should disable the soft warn")
	}
	if (Budget{AllowanceCredits: 1000, WarnFraction: 0}).Warned(999) {
		t.Error("zero warn fraction should disable the soft warn")
	}
}

func TestBudgetCapExceeded(t *testing.T) {
	b := Budget{HardCapCredits: 100}
	if b.CapExceeded(100) {
		t.Error("projected spend at the cap is allowed (strict >)")
	}
	if !b.CapExceeded(100.01) {
		t.Error("projected spend over the cap must be flagged")
	}

	// A zero cap disables the hard ceiling, so nothing is ever gated.
	if (Budget{HardCapCredits: 0}).CapExceeded(1e6) {
		t.Error("zero cap should disable the hard ceiling")
	}
}

func TestLeash(t *testing.T) {
	if (Leash{}).Active() {
		t.Error("a zero-value leash must be inert (off by default)")
	}
	if (Leash{}).Breached(1e9, 1e9) {
		t.Error("an inactive leash never gates")
	}
	tests := []struct {
		name    string
		leash   Leash
		credits float64
		turns   int64
		want    bool
	}{
		{"credits under cap", Leash{MaxCredits: 100}, 99.99, 0, false},
		{"credits at cap is breached (inclusive)", Leash{MaxCredits: 100}, 100, 0, true},
		{"credits over cap", Leash{MaxCredits: 100}, 100.01, 0, true},
		{"turns under cap", Leash{MaxTurns: 5}, 0, 4, false},
		{"turns at cap is breached (inclusive)", Leash{MaxTurns: 5}, 0, 5, true},
		{"either axis trips it", Leash{MaxCredits: 100, MaxTurns: 5}, 0, 5, true},
		{"zero credits axis is no-limit", Leash{MaxTurns: 5}, 1e9, 4, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.leash.Breached(tt.credits, tt.turns); got != tt.want {
				t.Errorf("Breached(%v,%d) = %v, want %v", tt.credits, tt.turns, got, tt.want)
			}
		})
	}
}

func TestMeterConcurrentSafe(t *testing.T) {
	m := NewMeter(DefaultPriceBook())
	var wg sync.WaitGroup
	const goroutines, perG = 16, 100
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				m.Record(Usage{Model: "gpt-5", InputTokens: 1_000})
			}
		}()
	}
	wg.Wait()
	if m.Count() != goroutines*perG {
		t.Fatalf("lost events under concurrency: got %d want %d", m.Count(), goroutines*perG)
	}
	in, _, _ := m.TotalTokens()
	if in != int64(goroutines*perG*1_000) {
		t.Fatalf("token total wrong under concurrency: %d", in)
	}
}

func TestReportedAIU(t *testing.T) {
	m := NewMeter(DefaultPriceBook())
	if m.ReportedAIU() != 0 {
		t.Fatal("expected zero reported AIU initially")
	}
	m.RecordReportedAIU(0) // no-op
	m.RecordReportedAIU(1.5)
	m.RecordReportedAIU(0.25)
	approx(t, m.ReportedAIU(), 1.75)
}

func TestReportedAIUConcurrent(t *testing.T) {
	m := NewMeter(nil)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); m.RecordReportedAIU(0.01) }()
	}
	wg.Wait()
	approx(t, m.ReportedAIU(), 1.0)
}

func TestUsageTotalTokens(t *testing.T) {
	u := Usage{InputTokens: 3, CachedTokens: 5, OutputTokens: 7}
	if u.TotalTokens() != 15 {
		t.Fatalf("TotalTokens = %d, want 15", u.TotalTokens())
	}
}

func TestEstimateTurn(t *testing.T) {
	pb := DefaultPriceBook()
	// The next turn resends the current context as fresh input. gpt-5 input is
	// $1.25/Mtok, so a 100k-token context ≈ $0.125 = 12.5 credits.
	c := EstimateTurn(pb, "gpt-5", 100_000)
	if !c.Matched {
		t.Fatal("gpt-5 should match the price book")
	}
	approx(t, c.InputUSD, 0.125)
	approx(t, c.CachedUSD, 0)
	approx(t, c.OutputUSD, 0)
	approx(t, c.USD(), 0.125)
	approx(t, c.Credits(), 12.5)
}

func TestEstimateTurnUnknownModelUsesFallback(t *testing.T) {
	pb := DefaultPriceBook()
	c := EstimateTurn(pb, "totally-made-up", 1_000_000)
	if c.Matched {
		t.Fatal("unknown model must not report a price-book match")
	}
	if c.USD() <= 0 {
		t.Fatalf("unknown model must price at the non-zero fallback, never free: %v", c.USD())
	}
}

func TestEstimateTurnNonPositiveContextIsZero(t *testing.T) {
	pb := DefaultPriceBook()
	for _, toks := range []int64{0, -100} {
		if c := EstimateTurn(pb, "gpt-5", toks); c.USD() != 0 {
			t.Fatalf("EstimateTurn(%d tokens) = %v, want 0", toks, c.USD())
		}
	}
}

func TestEstimateTurnNilPriceBookIsZero(t *testing.T) {
	if c := EstimateTurn(nil, "gpt-5", 100_000); c.USD() != 0 {
		t.Fatalf("nil price book must price at zero, got %v", c.USD())
	}
}

func TestMeterEstimateTurnUsesItsPriceBook(t *testing.T) {
	m := NewMeter(DefaultPriceBook())
	c := m.EstimateTurn("gpt-5", 100_000)
	approx(t, c.Credits(), 12.5)
}

// FuzzPriceNeverNegativeOrNaN ensures pricing is total: no input of non-negative
// token counts should ever yield a negative or NaN cost.
func FuzzPriceNeverNegativeOrNaN(f *testing.F) {
	f.Add("gpt-5", int64(1000), int64(0), int64(500))
	f.Add("unknown", int64(0), int64(0), int64(0))
	pb := DefaultPriceBook()
	f.Fuzz(func(t *testing.T, model string, in, cached, out int64) {
		if in < 0 || cached < 0 || out < 0 {
			t.Skip()
		}
		c := Price(pb, Usage{Model: model, InputTokens: in, CachedTokens: cached, OutputTokens: out})
		if math.IsNaN(c.USD()) || math.IsInf(c.USD(), 0) {
			t.Fatalf("cost is NaN/Inf for %q", model)
		}
		if c.USD() < 0 {
			t.Fatalf("negative cost %v for %q", c.USD(), model)
		}
	})
}
