package ctxforge

import (
	"fmt"
	"regexp"
	"strings"
)

// This file is the hooks governance primitive: a forge-backed Pre/PostToolUse
// rule and the pure evaluator the bridge consults before a tool runs. It is the
// third product pillar alongside cost-awareness and orchestration — governance
// is what lets autopilot run unattended without being reckless. — ADR-0029.

// Hook events: the lifecycle point a hook fires at. PreToolUse is consulted
// before a tool runs and yields an allow/deny/ask decision; PostToolUse observes
// a completed call (logging only in this build — no command execution yet).
const (
	HookPreToolUse  = "pre-tool-use"
	HookPostToolUse = "post-tool-use"
)

// Hook actions. They mirror the bridge's three permission outcomes: allow
// auto-approves, deny rejects with the reason fed back to the agent, and ask
// falls through to the interactive human-in-the-loop gate.
const (
	HookAllow = "allow"
	HookDeny  = "deny"
	HookAsk   = "ask"
)

var validHookEvents = map[string]bool{HookPreToolUse: true, HookPostToolUse: true}
var validHookActions = map[string]bool{HookAllow: true, HookDeny: true, HookAsk: true}

// validHookKinds is the set of tool kinds a hook may match on — the SDK
// permission kinds my-orchestra governs. G2 may widen this; keep it tight here.
var validHookKinds = map[string]bool{"read": true, "write": true, "shell": true, "mcp": true}

// HookMatch selects which tool calls a hook applies to: a tool ToolKind
// (read|write|shell|mcp; empty = any kind) and an optional command Pattern
// (empty = any command). At least one must be set. Both Pattern forms match
// **anywhere** in the command (contains semantics, so a deny can't be bypassed by
// a leading token like `sudo `): a metacharacter-free Pattern is a plain
// substring, and a Pattern with the glob metacharacters '*'/'?' is an
// **unanchored** wildcard search ('*' = any run of characters incl. none, '?' =
// exactly one). Matching is case-sensitive.
type HookMatch struct {
	ToolKind string `json:"toolKind,omitempty"`
	Pattern  string `json:"pattern,omitempty"`
}

// Hook is a forge-backed governance rule fired by the bridge around a tool call.
// It is a first-class forge entity, persisted in forge.json and CRUD-managed like
// every other entity (skills, agents, MCP servers, …). The safe-by-default policy
// is itself expressed as built-in hooks (DefaultHooks) that run through the same
// Evaluate as user hooks. — ADR-0029.
type Hook struct {
	ID      string    `json:"id"`
	Event   string    `json:"event"`  // pre-tool-use | post-tool-use
	Match   HookMatch `json:"match"`  // tool kind + optional command pattern
	Action  string    `json:"action"` // allow | deny | ask
	Reason  string    `json:"reason,omitempty"`
	Enabled bool      `json:"enabled"`
}

// wellFormedVarRef matches a complete ${NAME} reference (UPPER_SNAKE), the shape
// reused from the MCP env indirection (ADR-0020). A "${" that does not begin one
// is a dangling reference and is rejected by Validate.
var wellFormedVarRef = regexp.MustCompile(`\$\{[A-Z_][A-Z0-9_]*\}`)

// hasDanglingVarRef reports whether s contains a "${" that does not begin a
// well-formed ${NAME} reference (an unclosed or empty/invalid placeholder).
func hasDanglingVarRef(s string) bool {
	for rest := s; ; {
		i := strings.Index(rest, "${")
		if i < 0 {
			return false
		}
		loc := wellFormedVarRef.FindStringIndex(rest[i:])
		if loc == nil || loc[0] != 0 {
			return true
		}
		rest = rest[i+loc[1]:]
	}
}

// Validate reports whether the hook is well-formed: a slug id, a known event and
// action, a non-empty match (a valid tool kind when set), and no dangling ${VAR}
// reference in the pattern or reason.
func (h Hook) Validate() error {
	if err := validateID("hook", h.ID); err != nil {
		return err
	}
	if !validHookEvents[h.Event] {
		return fmt.Errorf("hook %q: invalid event %q", h.ID, h.Event)
	}
	if !validHookActions[h.Action] {
		return fmt.Errorf("hook %q: invalid action %q", h.ID, h.Action)
	}
	if strings.TrimSpace(h.Match.ToolKind) == "" && strings.TrimSpace(h.Match.Pattern) == "" {
		return fmt.Errorf("hook %q: match requires a toolKind or pattern", h.ID)
	}
	if k := h.Match.ToolKind; k != "" && !validHookKinds[k] {
		return fmt.Errorf("hook %q: invalid toolKind %q", h.ID, k)
	}
	if hasDanglingVarRef(h.Match.Pattern) || hasDanglingVarRef(h.Reason) {
		return fmt.Errorf("hook %q: dangling ${VAR} reference", h.ID)
	}
	return nil
}

// matches reports whether the hook applies to a tool call of the given kind and
// command. An empty ToolKind matches any kind; an empty Pattern matches any
// command.
func (m HookMatch) matches(toolKind, command string) bool {
	if m.ToolKind != "" && m.ToolKind != toolKind {
		return false
	}
	if m.Pattern != "" && !patternMatch(m.Pattern, command) {
		return false
	}
	return true
}

// patternMatch applies the documented Pattern semantics: a glob over the whole
// command when the pattern carries '*'/'?', otherwise a substring match.
func patternMatch(pattern, command string) bool {
	if strings.ContainsAny(pattern, "*?") {
		return globMatch(pattern, command)
	}
	return strings.Contains(command, pattern)
}

// globMatch reports whether a '*'/'?' wildcard pattern occurs anywhere in s
// ('*' = any run of characters incl. spaces, '?' = exactly one). The regex is
// deliberately **unanchored** so a deny pattern matches regardless of leading or
// trailing tokens (e.g. `rm -rf *` still fires on `sudo rm -rf /`). A pattern
// that fails to compile never matches (it cannot, being built from escaped
// literals and the two metacharacters).
func globMatch(pattern, s string) bool {
	var b strings.Builder
	for _, r := range pattern {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	re, err := regexp.Compile(b.String())
	if err != nil {
		return false
	}
	return re.MatchString(s)
}

// Decision is the outcome of evaluating the hook policy for one tool call: the
// resolved Action (allow|deny|ask) and the Reason of the winning hook (fed back
// to the agent on deny, surfaced in the timeline otherwise).
type Decision struct {
	Action string
	Reason string
}

// Evaluate resolves the governance policy for a tool call against the hook set.
//
// A hook participates when it is Enabled, its Event equals event, and its Match
// applies to (toolKind, command). Among the participating hooks the most
// restrictive action wins: **deny > ask > allow**. With no participating hook the
// default is **ask** — the call falls through to the interactive gate, so the
// policy is safe (never silently auto-approving) when nothing explicitly allows
// it. The reported Reason is that of the first hook of the winning action class,
// so ordering only chooses which same-action reason is surfaced (the action
// itself is order-independent). The function is pure — built-in defaults and user
// hooks evaluate through this one path. — ADR-0029.
func Evaluate(hooks []Hook, event, toolKind, command string) Decision {
	var deny, ask, allow *Hook
	for i := range hooks {
		h := &hooks[i]
		if !h.Enabled || h.Event != event || !h.Match.matches(toolKind, command) {
			continue
		}
		switch h.Action {
		case HookDeny:
			if deny == nil {
				deny = h
			}
		case HookAsk:
			if ask == nil {
				ask = h
			}
		case HookAllow:
			if allow == nil {
				allow = h
			}
		}
	}
	switch {
	case deny != nil:
		return Decision{Action: HookDeny, Reason: deny.Reason}
	case ask != nil:
		return Decision{Action: HookAsk, Reason: ask.Reason}
	case allow != nil:
		return Decision{Action: HookAllow, Reason: allow.Reason}
	default:
		return Decision{Action: HookAsk}
	}
}

// Hook returns the hook with the given ID, or nil.
func (f *Forge) Hook(id string) *Hook {
	for i := range f.Hooks {
		if f.Hooks[i].ID == id {
			return &f.Hooks[i]
		}
	}
	return nil
}

// AddHook validates and appends a hook, rejecting duplicate IDs.
func (f *Forge) AddHook(h Hook) error {
	return f.mutate(func() error {
		if err := h.Validate(); err != nil {
			return err
		}
		if f.Hook(h.ID) != nil {
			return fmt.Errorf("hook %q already exists", h.ID)
		}
		f.Hooks = append(f.Hooks, h)
		return nil
	})
}

// UpdateHook replaces the hook identified by id with h, then validates the whole
// forge and rolls back to the prior value if the result is invalid (e.g. a
// rename collides with another id).
func (f *Forge) UpdateHook(id string, h Hook) error {
	return f.mutate(func() error {
		cur := f.Hook(id)
		if cur == nil {
			return fmt.Errorf("unknown hook %q", id)
		}
		*cur = h
		return nil
	})
}

// ToggleHook flips a hook's Enabled flag, returning the new state.
func (f *Forge) ToggleHook(id string) (bool, error) {
	h := f.Hook(id)
	if h == nil {
		return false, fmt.Errorf("unknown hook %q", id)
	}
	h.Enabled = !h.Enabled
	return h.Enabled, nil
}

// RemoveHook deletes the hook with the given id. Nothing within the forge
// references a hook; it routes through mutate for uniform discipline.
func (f *Forge) RemoveHook(id string) error {
	return f.mutate(func() error {
		for i := range f.Hooks {
			if f.Hooks[i].ID == id {
				f.Hooks = append(f.Hooks[:i], f.Hooks[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("unknown hook %q", id)
	})
}

// DefaultHooks returns the built-in, safe-by-default hook set (G1): read-only
// tool kinds are auto-approved, leaving writes, shell, and MCP calls to the
// interactive gate. The defaults make the out-of-the-box build safe — they run
// through the same Evaluate as user hooks, so a user deny still wins over a
// built-in allow (deny > allow) and a user ask downgrades a read to the gate.
// Compile prepends these to a session's compiled hooks. — ADR-0029.
func DefaultHooks() []Hook {
	return []Hook{
		{
			ID:      "builtin-allow-read",
			Event:   HookPreToolUse,
			Match:   HookMatch{ToolKind: "read"},
			Action:  HookAllow,
			Reason:  "read-only tool auto-approved by default",
			Enabled: true,
		},
	}
}
