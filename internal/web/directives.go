package web

import (
	"html/template"
	"regexp"
	"strings"
)

// Container directives (R5, issue 0081, ADR-0047): an in-house `:::name{attrs}`
// … `:::` block — the remark-directive *syntax* without the Node toolchain —
// resolved against a closed Go registry. The registry, not the parser, is the
// trust boundary: a name the registry does not hold is not a directive at all
// (it degrades to ordinary escaped markdown). Shipping a new designed block is
// adding a registry entry + a frag; forgetting to register fails closed.

// mdDirectiveOpen matches a directive opening line (already trimmed): `:::`
// followed by a bare component name and an optional brace-delimited attribute
// list — `:::card`, `:::details{summary=Build steps}`. The bare close `:::`
// has no name, so it does not match (it is handled separately).
var mdDirectiveOpen = regexp.MustCompile(`^:::([a-z][a-z0-9-]*)(?:\{(.*)\})?$`)

const (
	// maxDirectiveDepth bounds directive nesting so pathological input can't blow
	// the parse stack; beyond it a `:::name` line degrades to text (ADR-0047).
	maxDirectiveDepth = 8
	// maxDirectiveAttrSpan caps the brace-span length so a giant attribute string
	// can't be weaponized into quadratic work; an over-long span drops its attrs.
	maxDirectiveAttrSpan = 256
)

// directiveSpec maps a registered name to its fragment template and a projector
// that turns the parsed, validated attributes plus the already-rendered body
// into that template's data. Applying defaults and dropping unrecognized keys
// lives in the projector, so the registry entry fully owns the component.
type directiveSpec struct {
	frag    string
	project func(attrs map[string]string, body template.HTML) any
}

// directiveRegistry is the closed allowlist. An unregistered name never reaches
// a template — there is no "unknown directive" code path.
var directiveRegistry = map[string]directiveSpec{
	// :::card — a token-styled raised card surface; body = the recursively
	// rendered child blocks. No attributes in v1.
	"card": {
		frag: "dirCard",
		project: func(_ map[string]string, body template.HTML) any {
			return struct{ Body template.HTML }{Body: body}
		},
	},
	// :::details{summary=…} — a native, JS-free collapsible. summary rides the
	// inline escape pass; absent → a default label.
	"details": {
		frag: "dirDetails",
		project: func(attrs map[string]string, body template.HTML) any {
			summary := strings.TrimSpace(attrs["summary"])
			if summary == "" {
				summary = "Details"
			}
			return struct {
				Summary template.HTML
				Body    template.HTML
			}{
				Summary: template.HTML(inline(summary, nil)), //nolint:gosec // inline() escapes first, whitelist-only tags
				Body:    body,
			}
		},
	},
}

// directiveBlock is a parsed container directive (ADR-0047): name is a key into
// directiveRegistry (validated at parse time, so the render-time lookup always
// hits), attrs the parsed/validated attributes, and children the recursively
// parsed body. Storing the name (not the spec) keeps the block comparable for
// the parse tests.
type directiveBlock struct {
	name     string
	attrs    map[string]string
	children []Block
}

func (d directiveBlock) renderTo(b *strings.Builder, cs *citeScope) {
	spec, ok := directiveRegistry[d.name]
	if !ok {
		// Unreachable: only a registered name produces a directiveBlock. Fail
		// closed (emit nothing) rather than render an unknown component.
		return
	}
	// Body is composed from the escaped child renderers under the document
	// citation scope; the projector escapes any attribute-derived value before it
	// rides into the template.
	body := template.HTML(renderBlocksScoped(d.children, cs)) //nolint:gosec // composed from escaped block renderers
	b.WriteString(frag(spec.frag, spec.project(d.attrs, body)))
}

// directiveAt reports whether lines[i] opens a registered directive with a
// matching close found within the depth budget, returning the parsed name,
// attributes, body lines, and the index past the close. It is fence-balanced:
// any `:::name` line in the body increments depth and a bare `:::` decrements,
// so only the close that returns depth to zero ends the outer container. An
// unregistered name, an over-depth open, or an unterminated container all
// return ok=false — the caller degrades the line to ordinary text.
func directiveAt(lines []string, i, depth int) (name string, attrs map[string]string, body []string, next int, ok bool) {
	if depth >= maxDirectiveDepth {
		return "", nil, nil, 0, false
	}
	m := mdDirectiveOpen.FindStringSubmatch(strings.TrimSpace(lines[i]))
	if m == nil {
		return "", nil, nil, 0, false
	}
	if _, registered := directiveRegistry[m[1]]; !registered {
		return "", nil, nil, 0, false
	}
	fence := 1
	inCode := false
	for j := i + 1; j < len(lines); j++ {
		t := strings.TrimSpace(lines[j])
		// A fenced code block inside the body is opaque to the fence balancer: a
		// `:::` line within ``` … ``` is verbatim code, not a container token
		// (matching how parseLines treats the same fence). Without this a card
		// whose body shows a `:::` code sample would close early.
		if strings.HasPrefix(t, "```") {
			inCode = !inCode
			continue
		}
		if inCode {
			continue
		}
		switch {
		case t == ":::":
			fence--
			if fence == 0 {
				return m[1], parseDirectiveAttrs(m[2]), lines[i+1 : j], j + 1, true
			}
		case mdDirectiveOpen.MatchString(t):
			fence++
		}
	}
	return "", nil, nil, 0, false // unterminated: degrade
}

// isDirectiveOpenLine reports whether a line is a registered directive open —
// the cheap, O(1) check the paragraph/table loops use to stop accumulating (the
// same one-line peek posture as startsTable). It does not verify a matching
// close: an open without a usable close is degraded to text by parseLinesDepth,
// so this only decides where a block boundary is, not whether one parses.
func isDirectiveOpenLine(line string) bool {
	m := mdDirectiveOpen.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return false
	}
	_, registered := directiveRegistry[m[1]]
	return registered
}

// parseDirectiveAttrs parses a brace-span into a map of attributes: a
// space-separated list of key=value / key="quoted value" pairs. The span is
// length-bounded (an over-long span drops wholesale), bare keys with no value
// are ignored, and values stay raw here — each registered component escapes the
// values it consumes (escape-first) before they enter a template.
func parseDirectiveAttrs(raw string) map[string]string {
	attrs := map[string]string{}
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxDirectiveAttrSpan {
		return attrs
	}
	for i, n := 0, len(raw); i < n; {
		for i < n && raw[i] == ' ' {
			i++
		}
		if i >= n {
			break
		}
		ks := i
		for i < n && raw[i] != '=' && raw[i] != ' ' {
			i++
		}
		key := raw[ks:i]
		if i >= n || raw[i] != '=' {
			continue // bare key (no value) — ignore
		}
		i++ // consume '='
		var val string
		if i < n && raw[i] == '"' {
			i++
			vs := i
			for i < n && raw[i] != '"' {
				i++
			}
			val = raw[vs:i]
			if i < n {
				i++ // consume closing quote
			}
		} else {
			vs := i
			for i < n && raw[i] != ' ' {
				i++
			}
			val = raw[vs:i]
		}
		if key != "" {
			attrs[key] = val
		}
	}
	return attrs
}
