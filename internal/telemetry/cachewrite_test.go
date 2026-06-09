package telemetry

import "testing"

// ADR-0034: cache-write is a first-class priced category (default 1.25× input);
// reasoning is a subset of OutputTokens (already priced at the output rate) and
// must NOT be charged a second time.

func TestDefaultPriceBookCacheWriteIs125xInput(t *testing.T) {
	pb := DefaultPriceBook()
	for _, model := range pb.Models() {
		r, _ := pb.Rate(model)
		approx(t, r.CacheWritePerMTok, 1.25*r.InputPerMTok)
	}
	// The fallback rate carries the multiplier too, so an unknown model still
	// prices its cache-write rather than treating it as free.
	fb, _ := pb.Rate("totally-made-up")
	approx(t, fb.CacheWritePerMTok, 1.25*fb.InputPerMTok)
}

func TestPriceCacheWriteAtDefaultMultiplier(t *testing.T) {
	pb := DefaultPriceBook()
	// gpt-5 input $1.25/Mt → cache-write default 1.25× = $1.5625/Mt.
	c := Price(pb, Usage{Model: "gpt-5", CacheWriteTokens: 1_000_000})
	approx(t, c.CacheWriteUSD, 1.5625)
	approx(t, c.InputUSD, 0)
	approx(t, c.OutputUSD, 0)
	// Cache-write is additive to the turn's USD.
	approx(t, c.USD(), 1.5625)
}

func TestPriceDoesNotDoubleCountReasoning(t *testing.T) {
	pb := DefaultPriceBook()
	// Reasoning tokens are a subset of OutputTokens (SDK: "output tokens used for
	// reasoning"); output is already priced. Two turns with identical output but
	// different reasoning counts must cost exactly the same — no second charge.
	withReasoning := Price(pb, Usage{Model: "gpt-5", OutputTokens: 1_000_000, ReasoningTokens: 400_000})
	noReasoning := Price(pb, Usage{Model: "gpt-5", OutputTokens: 1_000_000})
	approx(t, withReasoning.USD(), noReasoning.USD())
	approx(t, withReasoning.OutputUSD, 10.0) // 1M × $10/Mt, reasoning included
	approx(t, withReasoning.USD(), 10.0)
}

func TestBuildPriceBookCacheWriteOverrideAndDerivedDefault(t *testing.T) {
	// A 4-element override pins the cache-write rate explicitly.
	pinned := BuildPriceBook(map[string][]float64{"gpt-5": {2, 0.2, 20, 5}})
	r, _ := pinned.Rate("gpt-5")
	approx(t, r.InputPerMTok, 2)
	approx(t, r.CacheWritePerMTok, 5)
	approx(t, Price(pinned, Usage{Model: "gpt-5", CacheWriteTokens: 1_000_000}).CacheWriteUSD, 5)

	// A 3-element (legacy) override derives cache-write from the *overridden* input
	// at the 1.25× default — so an old config still prices cache-write coherently.
	derived := BuildPriceBook(map[string][]float64{"gpt-5": {2, 0.2, 20}})
	rd, _ := derived.Rate("gpt-5")
	approx(t, rd.CacheWritePerMTok, 1.25*2)
}

func TestMeterRecordFoldsCacheWriteIntoTotals(t *testing.T) {
	m := NewMeter(DefaultPriceBook())
	// gpt-5: in $1.25/Mt (cache-write $1.5625/Mt), out $10/Mt.
	m.Record(Usage{Model: "gpt-5", InputTokens: 1_000_000, OutputTokens: 1_000_000, CacheWriteTokens: 1_000_000})
	tot := m.Totals()
	approx(t, tot.CacheWriteUSD, 1.5625)
	approx(t, tot.USD(), 1.25+10+1.5625)

	by := m.ByModel()
	if len(by) != 1 {
		t.Fatalf("want 1 model, got %d", len(by))
	}
	if by[0].CacheWriteTokens != 1_000_000 {
		t.Errorf("per-model CacheWriteTokens = %d, want 1000000", by[0].CacheWriteTokens)
	}
	approx(t, by[0].CacheWriteUSD, 1.5625)
	approx(t, by[0].USD(), 1.25+10+1.5625)
}
