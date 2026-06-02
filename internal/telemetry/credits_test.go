package telemetry

import (
	"math"
	"sync"
	"testing"
)

func approx(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("got %.12f, want %.12f", got, want)
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

func TestUsageTotalTokens(t *testing.T) {
	u := Usage{InputTokens: 3, CachedTokens: 5, OutputTokens: 7}
	if u.TotalTokens() != 15 {
		t.Fatalf("TotalTokens = %d, want 15", u.TotalTokens())
	}
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
