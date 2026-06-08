package ctxforge

import (
	"fmt"
	"path/filepath"
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
// (read|write|shell|mcp; empty = any kind), an optional command Pattern
// (empty = any command), and an optional OutsideWorkspace predicate. At least
// one dimension must be set. Both Pattern forms match **anywhere** in the command
// (contains semantics, so a deny can't be bypassed by a leading token like
// `sudo `): a metacharacter-free Pattern is a plain substring, and a Pattern with
// the glob metacharacters '*'/'?' is an **unanchored** wildcard search ('*' = any
// run of characters incl. none, '?' = exactly one). Matching is case-sensitive.
//
// OutsideWorkspace is the path-aware **workspace fence** dimension the glob
// matcher can't express: when set, the hook applies only when the call's target
// path (a write request's file name) resolves OUTSIDE the session workspace root
// threaded into Evaluate. A built-in mandatory hook uses it to gate writes that
// escape the project tree; the fence is inert when no workspace root is known
// (the evaluator can't fence against nothing). — ADR-0030.
type HookMatch struct {
	ToolKind         string `json:"toolKind,omitempty"`
	Pattern          string `json:"pattern,omitempty"`
	OutsideWorkspace bool   `json:"outsideWorkspace,omitempty"`
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
	// Mandatory marks a hook whose decision is **unbypassable by config**: a
	// mandatory deny rejects and a mandatory ask gates EVEN when the session runs
	// with AutoApproveTools. The built-in dangerous-action ruleset (DangerousHooks)
	// is mandatory; user hooks and the safe-read defaults are not. It does not
	// change the deny > ask > allow precedence (a user deny — more restrictive —
	// still wins over a mandatory ask); it only forecloses the auto-approve escape
	// hatch. — ADR-0030.
	Mandatory bool `json:"mandatory,omitempty"`
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
	if strings.TrimSpace(h.Match.ToolKind) == "" && strings.TrimSpace(h.Match.Pattern) == "" && !h.Match.OutsideWorkspace {
		return fmt.Errorf("hook %q: match requires a toolKind, pattern, or outsideWorkspace", h.ID)
	}
	if k := h.Match.ToolKind; k != "" && !validHookKinds[k] {
		return fmt.Errorf("hook %q: invalid toolKind %q", h.ID, k)
	}
	if hasDanglingVarRef(h.Match.Pattern) || hasDanglingVarRef(h.Reason) {
		return fmt.Errorf("hook %q: dangling ${VAR} reference", h.ID)
	}
	return nil
}

// matches reports whether the hook applies to a tool call of the given kind,
// command, and workspace root. An empty ToolKind matches any kind; an empty
// Pattern matches any command; OutsideWorkspace additionally requires the command
// (a write request's target path) to resolve outside workspace. All set
// dimensions must hold (AND).
func (m HookMatch) matches(toolKind, command, workspace string) bool {
	if m.ToolKind != "" && m.ToolKind != toolKind {
		return false
	}
	if m.OutsideWorkspace && !isOutsideWorkspace(command, workspace) {
		return false
	}
	if m.Pattern != "" && !patternMatch(m.Pattern, command) {
		return false
	}
	return true
}

// isOutsideWorkspace reports whether target resolves to a path outside the
// workspace root — the pure core of the workspace fence (ADR-0030). A relative
// target is resolved against workspace (so an in-tree relative write is inside);
// an absolute target is compared directly. The check is inert (returns false)
// when either is empty: with no known workspace root there is nothing to fence
// against, and gating every write would be noise. A target that cannot be made
// relative to the root (a different volume on Windows) counts as outside.
func isOutsideWorkspace(target, workspace string) bool {
	if workspace == "" || target == "" {
		return false
	}
	ws := filepath.Clean(workspace)
	p := target
	if !filepath.IsAbs(p) {
		p = filepath.Join(ws, p)
	}
	p = filepath.Clean(p)
	if p == ws {
		return false
	}
	rel, err := filepath.Rel(ws, p)
	if err != nil {
		return true
	}
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
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
// resolved Action (allow|deny|ask), the Reason of the winning hook (fed back to
// the agent on deny, surfaced in the timeline otherwise), and whether the winning
// decision came from a Mandatory hook. Mandatory is consulted by the bridge on
// the auto-approve path: a mandatory deny/ask is enforced even with
// AutoApproveTools, while a non-mandatory ask falls to the blanket approval.
// — ADR-0030.
type Decision struct {
	Action    string
	Reason    string
	Mandatory bool
}

// Evaluate resolves the governance policy for a tool call against the hook set.
//
// A hook participates when it is Enabled, its Event equals event, and its Match
// applies to (toolKind, command, workspace). Among the participating hooks the
// most restrictive action wins: **deny > ask > allow**. With no participating
// hook the default is **ask** — the call falls through to the interactive gate,
// so the policy is safe (never silently auto-approving) when nothing explicitly
// allows it. The reported Reason is that of the winning hook (a Mandatory hook of
// the winning action is preferred, so the dangerous-action reason surfaces over a
// coincident user reason); ordering only chooses which same-action reason is
// surfaced, the action itself is order-independent. Decision.Mandatory reports
// whether a mandatory hook drove the winning action — the bridge enforces a
// mandatory deny/ask even under AutoApproveTools (ADR-0030). The workspace root
// powers the OutsideWorkspace fence (empty = fence inert). The function is pure —
// built-in safe-read defaults, the built-in dangerous ruleset, and user hooks all
// evaluate through this one path. — ADR-0029, ADR-0030.
func Evaluate(hooks []Hook, event, toolKind, command, workspace string) Decision {
	var deny, ask, allow *Hook
	var mandatoryDeny, mandatoryAsk *Hook
	for i := range hooks {
		h := &hooks[i]
		if !h.Enabled || h.Event != event || !h.Match.matches(toolKind, command, workspace) {
			continue
		}
		switch h.Action {
		case HookDeny:
			if deny == nil {
				deny = h
			}
			if h.Mandatory && mandatoryDeny == nil {
				mandatoryDeny = h
			}
		case HookAsk:
			if ask == nil {
				ask = h
			}
			if h.Mandatory && mandatoryAsk == nil {
				mandatoryAsk = h
			}
		case HookAllow:
			if allow == nil {
				allow = h
			}
		}
	}
	switch {
	case deny != nil:
		win := deny
		if mandatoryDeny != nil {
			win = mandatoryDeny
		}
		return Decision{Action: HookDeny, Reason: win.Reason, Mandatory: mandatoryDeny != nil}
	case ask != nil:
		win := ask
		if mandatoryAsk != nil {
			win = mandatoryAsk
		}
		return Decision{Action: HookAsk, Reason: win.Reason, Mandatory: mandatoryAsk != nil}
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

// DangerousHooks returns the built-in, MANDATORY dangerous-action ruleset (G2):
// clearly-destructive patterns are hard-denied and risky-but-legitimate ones are
// force-gated, even when a session runs with AutoApproveTools — config alone
// cannot bypass them (Hook.Mandatory; ADR-0030). They run through the SAME
// Evaluate as the safe-read defaults and user hooks, so deny > ask > allow holds:
// a user deny (more restrictive) still wins over a mandatory ask, but a user allow
// (or a blanket auto-approve) can never weaken a mandatory deny/ask. Compile folds
// these into every session's policy.
//
// Patterns are unanchored substrings/globs (matching anywhere, so a leading token
// can't dodge them) and are deliberately conservative — defense-in-depth at the
// permission gate, not a hardened sandbox. Two limits of the string matcher are
// accepted and documented here: a recursive force-delete of any ABSOLUTE path
// under `/` or `~`/`$HOME` is denied (relative cleanup like `rm -rf ./build` is
// left to the gate), and exotic obfuscations (process substitution, unusual
// spacing) are out of scope.
func DangerousHooks() []Hook {
	deny := func(id, pattern, reason string) Hook {
		return Hook{ID: id, Event: HookPreToolUse, Match: HookMatch{ToolKind: "shell", Pattern: pattern}, Action: HookDeny, Reason: reason, Mandatory: true, Enabled: true}
	}
	return []Hook{
		// rm -rf / -fr targeting the root filesystem or the home directory:
		// irreversible mass deletion, never a legitimate unattended action. The
		// pattern requires the path to begin at `/`, `~`, or `$HOME` (right after
		// the flags), so a relative `rm -rf ./build` inside the tree is NOT caught.
		deny("builtin-deny-rm-root", "rm -rf /", "blocked: recursive force-delete targeting the root filesystem"),
		deny("builtin-deny-rm-root-fr", "rm -fr /", "blocked: recursive force-delete targeting the root filesystem"),
		deny("builtin-deny-rm-home", "rm -rf ~", "blocked: recursive force-delete targeting the home directory"),
		deny("builtin-deny-rm-home-fr", "rm -fr ~", "blocked: recursive force-delete targeting the home directory"),
		deny("builtin-deny-rm-homevar", "rm -rf $HOME", "blocked: recursive force-delete targeting $HOME"),
		deny("builtin-deny-rm-homevar-fr", "rm -fr $HOME", "blocked: recursive force-delete targeting $HOME"),
		// Pipe a download straight into a shell — remote code execution. The glob
		// `curl*|*sh` covers `| sh`, `|sh`, and `| bash` (bash ends in "sh").
		deny("builtin-deny-curl-pipe-shell", "curl*|*sh", "blocked: piping a download into a shell (remote code execution)"),
		deny("builtin-deny-wget-pipe-shell", "wget*|*sh", "blocked: piping a download into a shell (remote code execution)"),
		// Pipe a download into an editor — editors run startup/macro code, so this is
		// RCE-adjacent.
		deny("builtin-deny-curl-pipe-editor", "curl*|*vim", "blocked: piping a download into an editor (executes editor macros)"),
		deny("builtin-deny-wget-pipe-editor", "wget*|*vim", "blocked: piping a download into an editor (executes editor macros)"),
		// Obvious exfiltration: piping data to netcat (a reverse shell / data tunnel),
		// or POSTing well-known secret material over the network with curl.
		deny("builtin-deny-pipe-netcat", "| nc ", "blocked: piping data to netcat (exfiltration / reverse shell)"),
		deny("builtin-deny-pipe-netcat-nospace", "|nc ", "blocked: piping data to netcat (exfiltration / reverse shell)"),
		deny("builtin-deny-curl-ssh-key", "curl*id_rsa", "blocked: sending an SSH private key over the network"),
		deny("builtin-deny-curl-ssh-dir", "curl*.ssh/", "blocked: sending .ssh material over the network"),
		deny("builtin-deny-curl-aws-creds", "curl*.aws/credentials", "blocked: sending AWS credentials over the network"),
		deny("builtin-deny-curl-netrc", "curl*.netrc", "blocked: sending .netrc credentials over the network"),
		// sudo — privilege escalation. Sometimes legitimate, so it is force-gated
		// (mandatory ask) rather than hard-denied: a human must approve even in auto.
		{
			ID: "builtin-ask-sudo", Event: HookPreToolUse,
			Match:  HookMatch{ToolKind: "shell", Pattern: "sudo "},
			Action: HookAsk, Reason: "confirm: sudo escalates privileges", Mandatory: true, Enabled: true,
		},
		// A write whose target resolves OUTSIDE the session workspace — the path-aware
		// fence (ADR-0030). Legitimate sometimes (writing to /tmp, ~/.config), so it is
		// force-gated, not denied; the fence is inert when no workspace root is known.
		{
			ID: "builtin-ask-write-outside-workspace", Event: HookPreToolUse,
			Match:  HookMatch{ToolKind: "write", OutsideWorkspace: true},
			Action: HookAsk, Reason: "confirm: write target is outside the workspace", Mandatory: true, Enabled: true,
		},
	}
}
