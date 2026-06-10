package web

// The contrast/structure guard for the token foundation (issue 0063, ADR-0036).
//
// app.css is the single hand-written stylesheet; this test makes its two core
// invariants fail loud at `go test` time, before the axe both-theme e2e scan
// ever runs:
//
//  1. Structure — the `@layer tokens, base, components, utilities` contract:
//     the order statement comes first, every top-level construct is layered
//     (one un-layered rule would outrank every layer and silently invert the
//     cascade), the vendored Open Props subset is imported into the tokens
//     layer, and component rules never reach past the semantic tier into the
//     `--p-*` primitives.
//
//  2. Contrast — every text-role/surface pair the UI relies on clears WCAG AA
//     (4.5:1) in BOTH themes, computed from the OKLCH primitives via a real
//     OKLCH → sRGB conversion (Björn Ottosson's OKLab matrices), not eyeballed.
//     The guard parses the constrained token grammar the tokens layer commits
//     to: `--p-<name>: oklch(<L>% <C> <H>)` primitives and
//     `--<role>: light-dark(var(--p-a), var(--p-b))` semantics.

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// ---- shared parsing helpers ----

func appCSS(t *testing.T) string {
	t.Helper()
	b, err := staticFS.ReadFile("static/app.css")
	if err != nil {
		t.Fatalf("read embedded app.css: %v", err)
	}
	return string(b)
}

var (
	cssComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	wsRun      = regexp.MustCompile(`\s+`)
	layerName  = regexp.MustCompile(`^@layer\s+([\w-]+)\s*\{`)
	propertyRe = regexp.MustCompile(`@property\s+--[\w-]+\s*\{[^}]*syntax:`)
	// primitiveDecl is the loose form of every --p-* declaration; primitiveRe
	// is the strict guarded grammar. Anything matching the former but not the
	// latter fails the guard instead of silently escaping it.
	primitiveDecl = regexp.MustCompile(`--p-([\w-]+)\s*:`)
)

func stripComments(css string) string { return cssComment.ReplaceAllString(css, "") }

// topLevelStatements splits the comment-stripped stylesheet into top-level
// constructs: `...;` statements and `...{...}` blocks (brace-balanced).
// Quoted strings are opaque, so a brace/semicolon inside e.g. a url("…")
// or content value can't mis-split a statement.
func topLevelStatements(css string) []string {
	var out []string
	var buf strings.Builder
	depth := 0
	var quote rune // 0 when outside a string
	for _, r := range css {
		buf.WriteRune(r)
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '"', '\'':
			quote = r
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				out = append(out, strings.TrimSpace(buf.String()))
				buf.Reset()
			}
		case ';':
			if depth == 0 {
				out = append(out, strings.TrimSpace(buf.String()))
				buf.Reset()
			}
		}
	}
	if s := strings.TrimSpace(buf.String()); s != "" {
		out = append(out, s)
	}
	return out
}

// layerBlock returns the concatenated bodies of every `@layer <name> { ... }`
// block with the given name.
func layerBlock(css, name string) string {
	var out strings.Builder
	prefix := "@layer " + name
	for _, st := range topLevelStatements(stripComments(css)) {
		if !strings.HasPrefix(st, prefix) {
			continue
		}
		open := strings.Index(st, "{")
		if open < 0 {
			continue // the order statement, not a block
		}
		out.WriteString(st[open+1 : len(st)-1])
	}
	return out.String()
}

// ---- 1. structure: the @layer contract (ADR-0036) ----

func TestAppCSSLayerContract(t *testing.T) {
	raw := appCSS(t)
	css := stripComments(raw)
	stmts := topLevelStatements(css)
	if len(stmts) == 0 {
		t.Fatal("app.css parsed to zero top-level statements")
	}

	const order = "@layer tokens, base, components, utilities;"
	if got := wsRun.ReplaceAllString(stmts[0], " "); got != order {
		t.Errorf("first statement must be the layer order contract\n got: %s\nwant: %s", got, order)
	}

	// Every top-level construct must be layered (or a cascade-independent
	// at-rule): one un-layered rule would beat all four layers.
	for _, st := range stmts {
		switch {
		case strings.HasPrefix(st, "@layer"),
			strings.HasPrefix(st, "@import"),
			strings.HasPrefix(st, "@property"),
			strings.HasPrefix(st, "@charset"):
		default:
			head := st
			if len(head) > 60 {
				head = head[:60] + "…"
			}
			t.Errorf("un-layered top-level rule (outranks every @layer): %q", head)
		}
	}

	// Exactly the four contracted layer names — a fifth layer (or a typo'd
	// name) silently lands outside the documented cascade. Derived from the
	// order statement so the contract has one authoritative spelling.
	allowed := map[string]bool{}
	for _, name := range strings.Split(strings.TrimSuffix(strings.TrimPrefix(order, "@layer "), ";"), ", ") {
		allowed[name] = true
	}
	for _, st := range stmts[1:] {
		if m := layerName.FindStringSubmatch(st); m != nil && !allowed[m[1]] {
			t.Errorf("undeclared layer %q (contract: %s)", m[1], order)
		}
	}

	// The vendored Open Props subset loads into the tokens layer (ADR-0036).
	for _, f := range []string{"open-props.easings.min.css", "open-props.animations.min.css"} {
		want := fmt.Sprintf(`@import url("%s") layer(tokens);`, f)
		if !strings.Contains(css, want) {
			t.Errorf("missing vendored import: %s", want)
		}
	}

	// Components consume semantic/component tokens only — never primitives.
	for _, layer := range []string{"base", "components", "utilities"} {
		if strings.Contains(layerBlock(raw, layer), "var(--p-") {
			t.Errorf("layer %q references a --p-* primitive directly; only the tokens layer may (three-tier contract)", layer)
		}
	}

	// At least one registered animatable token (the @property seam W3 consumes).
	if !propertyRe.MatchString(css) {
		t.Error(`no @property registration found (the charter ships at least one animatable token)`)
	}
}

func TestVendoredOpenPropsSubset(t *testing.T) {
	for file, marker := range map[string]string{
		"static/open-props.easings.min.css":    "--ease-spring-1:linear(", // the W3 spring seam
		"static/open-props.animations.min.css": "--animation-fade-in:",
	} {
		b, err := staticFS.ReadFile(file)
		if err != nil {
			t.Fatalf("vendored file missing from embed: %s: %v", file, err)
		}
		if !strings.Contains(string(b), marker) {
			t.Errorf("%s: published marker %q not found — wrong or truncated vendoring", file, marker)
		}
	}
}

// ---- 2. contrast: WCAG AA from the OKLCH primitives ----

// srgb is a color in (gamma-encoded) sRGB, channels 0..1.
type srgb struct{ r, g, b float64 }

// oklchToSRGB converts via OKLab (Ottosson's matrices). It errors when the
// color falls outside the sRGB gamut: browsers gamut-map unpredictably, so an
// out-of-gamut primitive would render — and contrast — differently per engine.
func oklchToSRGB(l, c, hDeg float64) (srgb, error) {
	h := hDeg * math.Pi / 180
	a, bb := c*math.Cos(h), c*math.Sin(h)

	l_ := l + 0.3963377774*a + 0.2158037573*bb
	m_ := l - 0.1055613458*a - 0.0638541728*bb
	s_ := l - 0.0894841775*a - 1.2914855480*bb
	l3, m3, s3 := l_*l_*l_, m_*m_*m_, s_*s_*s_

	rl := +4.0767416621*l3 - 3.3077115913*m3 + 0.2309699292*s3
	gl := -1.2684380046*l3 + 2.6097574011*m3 - 0.3413193965*s3
	bl := -0.0041960863*l3 - 0.7034186147*m3 + 1.7076147010*s3

	const slack = 0.002 // float noise only — not a gamut-mapping allowance
	for _, ch := range []float64{rl, gl, bl} {
		if ch < -slack || ch > 1+slack {
			return srgb{}, fmt.Errorf("oklch(%.3f %.3f %.0f) outside sRGB gamut (linear channel %.4f)", l, c, hDeg, ch)
		}
	}
	enc := func(u float64) float64 {
		u = math.Min(1, math.Max(0, u))
		if u <= 0.0031308 {
			return 12.92 * u
		}
		return 1.055*math.Pow(u, 1/2.4) - 0.055
	}
	return srgb{enc(rl), enc(gl), enc(bl)}, nil
}

// relLuminance is WCAG relative luminance.
func relLuminance(c srgb) float64 {
	lin := func(u float64) float64 {
		if u <= 0.04045 {
			return u / 12.92
		}
		return math.Pow((u+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.r) + 0.7152*lin(c.g) + 0.0722*lin(c.b)
}

func contrastRatio(a, b srgb) float64 {
	la, lb := relLuminance(a), relLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

var (
	primitiveRe = regexp.MustCompile(`--p-([\w-]+)\s*:\s*oklch\(\s*([\d.]+)%\s+([\d.]+)\s+([\d.]+)\s*\)`)
	semanticRe  = regexp.MustCompile(`--([\w-]+)\s*:\s*light-dark\(\s*var\(--p-([\w-]+)\)\s*,\s*var\(--p-([\w-]+)\)\s*\)`)
)

// tokenColors resolves every guarded semantic token to its light/dark sRGB pair.
func tokenColors(t *testing.T) map[string][2]srgb {
	t.Helper()
	tokens := layerBlock(appCSS(t), "tokens")

	prims := map[string]srgb{}
	for _, m := range primitiveRe.FindAllStringSubmatch(tokens, -1) {
		l, _ := strconv.ParseFloat(m[2], 64)
		c, _ := strconv.ParseFloat(m[3], 64)
		h, _ := strconv.ParseFloat(m[4], 64)
		col, err := oklchToSRGB(l/100, c, h)
		if err != nil {
			t.Errorf("primitive --p-%s: %v", m[1], err)
			continue
		}
		prims[m[1]] = col
	}
	if len(prims) == 0 {
		t.Fatal("no --p-* oklch() primitives found in the tokens layer")
	}
	// Completeness: every --p-* declaration must match the guarded grammar.
	// A primitive written with an alpha channel, relative-color syntax, or any
	// other shape would otherwise silently escape the gamut + contrast checks.
	for _, m := range primitiveDecl.FindAllStringSubmatch(tokens, -1) {
		if _, ok := prims[m[1]]; !ok {
			t.Errorf("primitive --p-%s does not match the guarded grammar `--p-<name>: oklch(<L>%% <C> <H>)` — it would escape the contrast guard", m[1])
		}
	}

	sem := map[string][2]srgb{}
	for _, m := range semanticRe.FindAllStringSubmatch(tokens, -1) {
		light, okL := prims[m[2]]
		dark, okD := prims[m[3]]
		if !okL || !okD {
			t.Errorf("semantic --%s references undefined primitive (--p-%s / --p-%s)", m[1], m[2], m[3])
			continue
		}
		sem[m[1]] = [2]srgb{light, dark}
	}
	return sem
}

func TestTokenContrastAA(t *testing.T) {
	sem := tokenColors(t)

	// Every semantic role ADR-0025/0036/0038 commits to must resolve through
	// the primitive tier (so the contrast math below actually covers it).
	for _, role := range []string{"bg", "panel", "overlay", "fg", "dim", "subtle", "accent", "accent2", "good", "warn", "bad", "on-bright"} {
		if _, ok := sem[role]; !ok {
			t.Fatalf("semantic token --%s not defined as light-dark(var(--p-…), var(--p-…)) in the tokens layer", role)
		}
	}

	// The pair set is rule-shaped, not hand-listed: every text role × every
	// surface role, plus --on-bright on every solid fill — so a new text role
	// or surface (W2's luminance ladder) extends the guard by editing one
	// slice, and combinations the UI already uses (accent headings on --panel
	// cards, status colors on the panel) can't be forgotten.
	textRoles := []string{"fg", "dim", "accent", "accent2", "good", "warn", "bad"}
	surfaces := []string{"bg", "panel", "overlay"}     // the surface luminance ladder, base → raised → overlay (ADR-0038)
	fills := []string{"accent", "good", "warn", "bad"} // --on-bright is the single companion on ANY solid fill
	type pair struct{ text, surface string }
	var pairs []pair
	for _, txt := range textRoles {
		for _, s := range surfaces {
			pairs = append(pairs, pair{txt, s})
		}
	}
	for _, f := range fills {
		pairs = append(pairs, pair{"on-bright", f})
	}
	const aa = 4.5
	for _, p := range pairs {
		for theme, idx := range map[string]int{"light": 0, "dark": 1} {
			got := contrastRatio(sem[p.text][idx], sem[p.surface][idx])
			if got < aa {
				t.Errorf("--%s on --%s (%s) = %.2f:1, below WCAG AA %.1f:1", p.text, p.surface, theme, got, aa)
			}
		}
	}
}

// ---- 3. elevation + scales: the W2 token contract (issue 0064, ADR-0038) ----

// TestSurfaceLadderDarkStepsLighter proves the dark theme's elevation channel:
// shadows are invisible on a dark canvas, so a raised surface must step
// LIGHTER instead — base (--bg) < raised (--panel) < overlay (--overlay) in
// luminance. Light theme carries elevation via the layered shadows below, so
// its ladder is only required to be non-inverted (never darker when raised).
func TestSurfaceLadderDarkStepsLighter(t *testing.T) {
	sem := tokenColors(t)
	ladder := []string{"bg", "panel", "overlay"}
	for i := 0; i+1 < len(ladder); i++ {
		lo, hi := ladder[i], ladder[i+1]
		if _, ok := sem[lo]; !ok {
			t.Fatalf("ladder surface --%s not defined in the guarded grammar", lo)
		}
		if _, ok := sem[hi]; !ok {
			t.Fatalf("ladder surface --%s not defined in the guarded grammar", hi)
		}
		const dark = 1
		if a, b := relLuminance(sem[lo][dark]), relLuminance(sem[hi][dark]); a >= b {
			t.Errorf("dark ladder inverted: --%s (%.4f) must be lighter than --%s (%.4f)", hi, b, lo, a)
		}
		const light = 0
		if a, b := relLuminance(sem[lo][light]), relLuminance(sem[hi][light]); b < a {
			t.Errorf("light ladder inverted: --%s (%.4f) darker than --%s (%.4f)", hi, b, lo, a)
		}
	}
}

// TestTokenScales guards the structure of the W2 scales in the tokens layer:
// the dual-channel elevation tokens (one --shadow-color hue variable, layered
// --shadow-1/2/3 stacks), the constrained radius ladder + 8px/4px-half-step
// space scale, the type scale with display tracking, and the glass border.
// Presence + shape only — the contrast math lives above.
func TestTokenScales(t *testing.T) {
	tokens := layerBlock(appCSS(t), "tokens")

	wanted := []string{
		"--shadow-color:", // the single hue variable every shadow layer stacks
		"--border-glass:", // 1px translucent border for glass surfaces
		"--font-sans:", "--font-mono:",
		"--tracking-display:", // negative tracking at display sizes
		"--tracking-caps:",    // open tracking on uppercase labels
	}
	for _, r := range []string{"1", "2", "3", "4", "5", "full"} {
		wanted = append(wanted, "--radius-"+r+":")
	}
	for i := 1; i <= 6; i++ {
		wanted = append(wanted, fmt.Sprintf("--space-%d:", i))
	}
	for i := 0; i <= 5; i++ {
		wanted = append(wanted, fmt.Sprintf("--text-%d:", i))
	}
	for _, w := range wanted {
		if !strings.Contains(tokens, w) {
			t.Errorf("tokens layer missing scale token %q", w)
		}
	}

	// Layered-shadow grammar (Comeau-style): every --shadow-N layer consumes
	// var(--shadow-color) — one hue to retune — and deeper steps stack MORE
	// layers (alpha accumulates where layers overlap, so depth compounds).
	shadowRe := regexp.MustCompile(`--shadow-([123])\s*:\s*([^;]+);`)
	layers := map[string]int{}
	for _, m := range shadowRe.FindAllStringSubmatch(tokens, -1) {
		layers[m[1]] = strings.Count(m[2], "var(--shadow-color)")
	}
	if layers["1"] < 2 || layers["2"] <= layers["1"] || layers["3"] <= layers["2"] {
		t.Errorf("layered shadows must deepen by stacking var(--shadow-color) layers: got --shadow-1=%d --shadow-2=%d --shadow-3=%d",
			layers["1"], layers["2"], layers["3"])
	}

	// Display tracking is negative (the engineered register: tight at size).
	trackRe := regexp.MustCompile(`--tracking-display\s*:\s*(-?[\d.]+)em`)
	if m := trackRe.FindStringSubmatch(tokens); m == nil {
		t.Error("--tracking-display must be an em value")
	} else if v, _ := strconv.ParseFloat(m[1], 64); v >= 0 {
		t.Errorf("--tracking-display = %sem; display tracking must be negative", m[1])
	}
}

// TestNoRadiusLiteralsOutsideTokens — the radius migration is total: every
// border-radius in base/components/utilities consumes the --radius-* ladder,
// never a px literal (the scattered 4/5/6/8/10/999px this replaced).
func TestNoRadiusLiteralsOutsideTokens(t *testing.T) {
	re := regexp.MustCompile(`border-radius\s*:[^;}]*[0-9]+px`)
	css := appCSS(t)
	for _, layer := range []string{"base", "components", "utilities"} {
		for _, m := range re.FindAllString(layerBlock(css, layer), -1) {
			t.Errorf("layer %q: border-radius px literal (use the --radius-* ladder): %q", layer, m)
		}
	}
}
