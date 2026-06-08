package web

import (
	"math"
	"strconv"
	"strings"
)

// Server-rendered inline-SVG chart builders for the Telemetry KPI dashboard
// (V23, ADR-0027). Each is a pure function from a numeric series to an <svg>
// string, mirroring the string-building renderers (render.go, palette.go): the
// coordinate math is unit-tested directly (svg_test.go) and the result is
// injected as trusted HTML. Charts carry NO color literal — stroke/fill resolve
// from currentColor or a design token, so they theme automatically (ADR-0025) —
// and each is role="img" with a <title> + aria-label accessible name so the
// both-theme axe scan passes. Zero JS; they re-render server-side on the
// existing ?window= htmx swap. Fixed viewBox coordinate space; the rendered
// size is CSS-controlled (the SVG scales to its box). — ADR-0027.

// Sparkline geometry: a small fixed coordinate box; the per-card trend polyline.
const (
	sparkW   = 100.0
	sparkH   = 28.0
	sparkPad = 2.0
	bandW    = 100.0
	bandH    = 40.0
	bandPad  = 3.0
	bulletW  = 100.0
	bulletH  = 14.0
	bulletP  = 1.0
)

// svgNum formats a coordinate as the shortest decimal after rounding to 0.01, so
// the emitted path/points are deterministic and compact (50.0 → "50").
func svgNum(v float64) string {
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
}

// scaleX maps series index i of n into the inner width [pad, w-pad]. A single
// point (n<=1) is centered. Pure.
func scaleX(i, n int, w, pad float64) float64 {
	if n <= 1 {
		return w / 2
	}
	return pad + float64(i)/float64(n-1)*(w-2*pad)
}

// scaleY maps a value into the inner height with SVG's downward y: the series max
// sits at the top (pad), zero on the baseline (h-pad). A non-positive max keeps
// everything on the baseline (no divide-by-zero). Pure.
func scaleY(v, max, h, pad float64) float64 {
	base := h - pad
	if max <= 0 {
		return base
	}
	return base - (v/max)*(h-2*pad)
}

// seriesMax returns the largest value in the series, or 0 for an empty series.
func seriesMax(series []float64) float64 {
	max := 0.0
	for _, v := range series {
		if v > max {
			max = v
		}
	}
	return max
}

// sparkPoints maps a numeric series to a "x,y x,y …" polyline-points string in a
// w×h box (pad inset), normalized to the series max. Empty series → "". Pure;
// the coordinates are unit-tested directly.
func sparkPoints(series []float64, w, h, pad float64) string {
	if len(series) == 0 {
		return ""
	}
	max := seriesMax(series)
	pts := make([]string, len(series))
	for i, v := range series {
		x := scaleX(i, len(series), w, pad)
		y := scaleY(v, max, h, pad)
		pts[i] = svgNum(x) + "," + svgNum(y)
	}
	return strings.Join(pts, " ")
}

// areaPath builds a closed area path under the series line: from the first
// point's baseline, up through each point, down to the last point's baseline,
// closed — so a fill sits under the curve. Empty series → "". Pure; the
// coordinates are unit-tested directly.
func areaPath(series []float64, w, h, pad float64) string {
	if len(series) == 0 {
		return ""
	}
	max := seriesMax(series)
	base := h - pad
	var b strings.Builder
	x0 := scaleX(0, len(series), w, pad)
	b.WriteString("M " + svgNum(x0) + "," + svgNum(base))
	for i, v := range series {
		x := scaleX(i, len(series), w, pad)
		y := scaleY(v, max, h, pad)
		b.WriteString(" L " + svgNum(x) + "," + svgNum(y))
	}
	xn := scaleX(len(series)-1, len(series), w, pad)
	b.WriteString(" L " + svgNum(xn) + "," + svgNum(base))
	b.WriteString(" Z")
	return b.String()
}

// bulletGeom maps a measure value and a target onto a track of inner width
// (w-2*pad): the measure-bar width and the target marker's x, each clamped to the
// track so an over-budget value never overflows the viewBox. Pure; unit-tested.
func bulletGeom(value, target, scaleMax, w, pad float64) (barW, targetX float64) {
	inner := w - 2*pad
	frac := func(v float64) float64 {
		if scaleMax <= 0 {
			return 0
		}
		f := v / scaleMax
		if f < 0 {
			f = 0
		}
		if f > 1 {
			f = 1
		}
		return f
	}
	return frac(value) * inner, pad + frac(target)*inner
}

// svgOpen emits the opening <svg> with a fixed viewBox, a CSS class, and the
// role="img" + <title> + aria-label accessible name the both-theme axe scan
// requires. The label is HTML-escaped (it flows into a title text node and an
// attribute). preserveAspectRatio="none" lets the chart stretch to its CSS box.
func svgOpen(class string, w, h float64, label string) string {
	l := esc(label)
	return `<svg class="` + class + `" viewBox="0 0 ` + svgNum(w) + ` ` + svgNum(h) + `" ` +
		`preserveAspectRatio="none" role="img" aria-label="` + l + `">` +
		`<title>` + l + `</title>`
}

// sparklineSVG renders a per-card trend polyline: a normalized line over the
// series, stroked in currentColor (the card's accent), with no fill. Pure.
func sparklineSVG(series []float64, label string) string {
	pts := sparkPoints(series, sparkW, sparkH, sparkPad)
	return svgOpen("spark", sparkW, sparkH, label) +
		`<polyline class="spark-line" points="` + pts + `" fill="none" stroke="currentColor" ` +
		`stroke-width="1.5" stroke-linejoin="round" stroke-linecap="round" vector-effect="non-scaling-stroke"/>` +
		`</svg>`
}

// trendBandSVG renders the spend trend band: the cumulative-spend actuals as a
// filled area (solid) plus the burn-rate forecast as a dashed continuation line
// (ADR-0027). actual and forecast are cumulative credit series sharing a single
// x-axis (forecast's first point coincides with actual's last, so the dashed line
// continues the area); both normalize to the combined max. Pure.
func trendBandSVG(actual, forecast []float64, label string) string {
	combined := append(append([]float64{}, actual...), forecast...)
	max := seriesMax(combined)
	total := len(actual) + len(forecast) // shared x positions (forecast starts after actuals)
	if total < 1 {
		total = 1
	}

	var b strings.Builder
	b.WriteString(svgOpen("band", bandW, bandH, label))

	// Actuals: a filled cumulative area over the first len(actual) positions.
	if n := len(actual); n > 0 {
		base := bandH - bandPad
		b.WriteString(`<path class="band-area" d="M `)
		x0 := scaleX(0, total, bandW, bandPad)
		b.WriteString(svgNum(x0) + "," + svgNum(base))
		for i, v := range actual {
			x := scaleX(i, total, bandW, bandPad)
			y := scaleY(v, max, bandH, bandPad)
			b.WriteString(" L " + svgNum(x) + "," + svgNum(y))
		}
		xn := scaleX(n-1, total, bandW, bandPad)
		b.WriteString(" L " + svgNum(xn) + "," + svgNum(base) + ` Z" fill="var(--accent)" fill-opacity="0.18" stroke="var(--accent)" stroke-width="1.25" vector-effect="non-scaling-stroke"/>`)
	}

	// Forecast: a dashed continuation polyline from the last actual point through
	// the projected cumulative tail.
	if len(forecast) > 0 && len(actual) > 0 {
		pts := make([]string, 0, len(forecast)+1)
		lastIdx := len(actual) - 1
		lx := scaleX(lastIdx, total, bandW, bandPad)
		ly := scaleY(actual[lastIdx], max, bandH, bandPad)
		pts = append(pts, svgNum(lx)+","+svgNum(ly))
		for j, v := range forecast {
			x := scaleX(len(actual)+j, total, bandW, bandPad)
			y := scaleY(v, max, bandH, bandPad)
			pts = append(pts, svgNum(x)+","+svgNum(y))
		}
		b.WriteString(`<polyline class="band-forecast" points="` + strings.Join(pts, " ") + `" ` +
			`fill="none" stroke="var(--accent2)" stroke-width="1.25" stroke-dasharray="3 2" ` +
			`vector-effect="non-scaling-stroke"/>`)
	}

	b.WriteString(`</svg>`)
	return b.String()
}

// bulletSVG renders the spend-vs-budget bullet: a track (the budget range), a
// measure bar (month-to-date spend), and a target marker (the projected
// month-end spend at the current pace). overTarget flips the bar to the --bad
// token so an over-pace projection reads at a glance. value/target are clamped to
// the track. Pure.
func bulletSVG(value, target, scaleMax float64, overTarget bool, label string) string {
	barW, targetX := bulletGeom(value, target, scaleMax, bulletW, bulletP)
	inner := bulletW - 2*bulletP
	barClass := "bullet-bar"
	barFill := "var(--accent)"
	if overTarget {
		barClass += " over"
		barFill = "var(--bad)" // an over-pace projection reads at a glance
	}
	var b strings.Builder
	b.WriteString(svgOpen("bullet", bulletW, bulletH, label))
	// Track: the full budget range.
	b.WriteString(`<rect class="bullet-track" x="` + svgNum(bulletP) + `" y="` + svgNum(bulletH/2-3) +
		`" width="` + svgNum(inner) + `" height="6" rx="3" fill="var(--sunken)"/>`)
	// Measure bar: month-to-date spend.
	b.WriteString(`<rect class="` + barClass + `" x="` + svgNum(bulletP) + `" y="` + svgNum(bulletH/2-3) +
		`" width="` + svgNum(barW) + `" height="6" rx="3" fill="` + barFill + `"/>`)
	// Target marker: a vertical tick at the projected month-end spend.
	b.WriteString(`<rect class="bullet-target" x="` + svgNum(targetX-0.75) + `" y="` + svgNum(bulletH/2-5) +
		`" width="1.5" height="10" fill="var(--fg)"/>`)
	b.WriteString(`</svg>`)
	return b.String()
}
