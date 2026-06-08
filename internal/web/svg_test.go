package web

import "strings"

import "testing"

func TestSparkPointsCoordinates(t *testing.T) {
	// Two points, max 10, box 100×28, pad 2 → inner 96×24. The first sits on the
	// baseline (value 0), the last at the top (value == max). SVG y grows down, so
	// a higher value maps to a smaller y.
	got := sparkPoints([]float64{0, 10}, 100, 28, 2)
	if got != "2,26 98,2" {
		t.Fatalf("sparkPoints = %q, want %q", got, "2,26 98,2")
	}
	// A flat series sits entirely on the baseline (no divide-by-zero on max 0).
	if got := sparkPoints([]float64{0, 0, 0}, 100, 28, 2); got != "2,26 50,26 98,26" {
		t.Fatalf("flat sparkPoints = %q, want all on baseline", got)
	}
	// A single point is centered horizontally.
	if got := sparkPoints([]float64{4}, 100, 28, 2); got != "50,2" {
		t.Fatalf("single-point sparkPoints = %q, want %q", got, "50,2")
	}
	// An empty series renders no points.
	if got := sparkPoints(nil, 100, 28, 2); got != "" {
		t.Fatalf("empty sparkPoints = %q, want empty", got)
	}
}

func TestAreaPathCoordinates(t *testing.T) {
	// The closed area starts and ends on the baseline so the fill sits under the
	// line: M (x0,baseline) → line through each point → down to (xLast,baseline) → Z.
	got := areaPath([]float64{0, 10}, 100, 28, 2)
	want := "M 2,26 L 2,26 L 98,2 L 98,26 Z"
	if got != want {
		t.Fatalf("areaPath = %q, want %q", got, want)
	}
}

func TestBulletGeomFractions(t *testing.T) {
	// value/target as fractions of the scale max, mapped into the inner width.
	barW, targetX := bulletGeom(30, 80, 100, 100, 0)
	if barW != 30 || targetX != 80 {
		t.Fatalf("bulletGeom = (%v,%v), want (30,80)", barW, targetX)
	}
	// Over-scale values clamp to the track (never overflow the viewBox).
	barW, targetX = bulletGeom(150, 200, 100, 100, 0)
	if barW != 100 || targetX != 100 {
		t.Fatalf("clamped bulletGeom = (%v,%v), want (100,100)", barW, targetX)
	}
}

func TestSparklineSVGAccessibleAndThemed(t *testing.T) {
	svg := sparklineSVG([]float64{1, 2, 3}, "Total spend trend, last 14 days")
	for _, want := range []string{
		`class="spark"`,
		`role="img"`,
		`<title>Total spend trend, last 14 days</title>`,
		`aria-label="Total spend trend, last 14 days"`,
		`<polyline`,
		`viewBox=`,
	} {
		if !strings.Contains(svg, want) {
			t.Fatalf("sparklineSVG missing %q\n%s", want, svg)
		}
	}
	// No raw color literal — strokes theme via currentColor / a token.
	if strings.Contains(svg, "#") || strings.Contains(svg, "rgb") {
		t.Fatalf("sparklineSVG must not hard-code a color\n%s", svg)
	}
}

func TestTrendBandSVGHasActualsAndDashedForecast(t *testing.T) {
	band := trendBandSVG([]float64{0, 5, 10}, []float64{10, 14, 18}, "Cumulative spend, last 14 days")
	for _, want := range []string{
		`class="band"`,
		`role="img"`,
		`<title>Cumulative spend, last 14 days</title>`,
		`<path`,            // the cumulative actuals area
		`stroke-dasharray`, // the forecast continuation is dashed
	} {
		if !strings.Contains(band, want) {
			t.Fatalf("trendBandSVG missing %q\n%s", want, band)
		}
	}
}

func TestBulletSVGRendersTrackBarAndTarget(t *testing.T) {
	bullet := bulletSVG(40, 60, 100, true, "Spend 40 cr against 100 cr budget")
	for _, want := range []string{
		`class="bullet"`,
		`role="img"`,
		`<title>Spend 40 cr against 100 cr budget</title>`,
		`class="bullet-track"`,
		`class="bullet-bar`, // the measure bar (class may carry an over-target modifier)
		`class="bullet-target"`,
	} {
		if !strings.Contains(bullet, want) {
			t.Fatalf("bulletSVG missing %q\n%s", want, bullet)
		}
	}
}

func TestSVGEscapesAccessibleName(t *testing.T) {
	// The label flows into a <title> and aria-label; it must be HTML-escaped.
	svg := sparklineSVG([]float64{1}, `spend <b> & "more"`)
	if strings.Contains(svg, "<b>") {
		t.Fatalf("accessible name must be escaped\n%s", svg)
	}
}
