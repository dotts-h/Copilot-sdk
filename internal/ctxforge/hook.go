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

// Agent modes a hook can be scoped to (mode binding, ADR-0031). They mirror the
// per-turn agent modes the web layer threads to Send: autopilot runs tools
// unattended, interactive gates more, plan is read-and-propose. A hook with an
// empty Modes set applies in EVERY mode; the built-in mandatory ruleset leaves
// Modes empty so the G2 floor can never be weakened by mode binding. The empty
// active mode ("" — no explicit mode) is the session default and matches only
// hooks with an empty Modes set.
const (
	ModeAutopilot   = "autopilot"
	ModeInteractive = "interactive"
	ModePlan        = "plan"
)

var validHookEvents = map[string]bool{HookPreToolUse: true, HookPostToolUse: true}
var validHookActions = map[string]bool{HookAllow: true, HookDeny: true, HookAsk: true}
var validHookModes = map[string]bool{ModeAutopilot: true, ModeInteractive: true, ModePlan: true}

// EffectiveAutoApprove resolves whether a session running in the given agent
// mode blanket-approves the non-mandatory remainder (an ordinary ask or a
// no-match default-ask). It is the mode-binding baseline (ADR-0031): autopilot
// forces auto-approve ON (strict defaults on, unattended) and interactive forces
// it OFF (fully interactive — more gates), regardless of the session's static
// AutoApproveTools config; any other mode (plan, shell, or no explicit mode)
// defers to configDefault. The mandatory dangerous subset is enforced by the
// bridge BEFORE this baseline applies, so a true here can never bypass a
// mandatory deny/ask. Pure, so the bridge stays a thin caller. — ADR-0030, ADR-0031.
func EffectiveAutoApprove(mode string, configDefault bool) bool {
	switch mode {
	case ModeAutopilot:
		return true
	case ModeInteractive:
		return false
	default:
		return configDefault
	}
}

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
	// Modes scopes the hook to a set of agent modes (mode binding, ADR-0031):
	// the hook participates only when the session's active mode is in this set.
	// An empty set means the hook applies in EVERY mode — the built-in mandatory
	// ruleset leaves it empty so mode binding can never weaken the G2 floor.
	Modes []string `json:"modes,omitempty"`
	// Mandatory marks a hook whose decision is **unbypassable by config**: a
	// mandatory deny rejects and a mandatory ask gates EVEN when the session runs
	// with AutoApproveTools. The built-in dangerous-action ruleset (DangerousHooks)
	// is mandatory; user hooks and the safe-read defaults are not. It does not
	// change the deny > ask > allow precedence (a user deny — more restrictive —
	// still wins over a mandatory ask); it only forecloses the auto-approve escape
	// hatch. — ADR-0030.
	Mandatory bool `json:"mandatory,omitempty"`
	// Command is an external local command a PostToolUse hook runs AFTER a matching
	// tool completes (the executor — G5, ADR-0032). It is the program to exec; its
	// CommandArgs are the arguments. Both may carry ${VAR} references (the MCP env
	// shape, ADR-0020) resolved at EXECUTION by the seam — the secret is never
	// stored, only the reference. A command is valid ONLY on a post-tool-use hook
	// (a PreToolUse hook with a command is rejected): the command's output is
	// UNTRUSTED display-only telemetry, never a gate, so it must never sit on the
	// pre-tool decision path. The domain keeps the field opaque — it validates the
	// shape (post-only, no dangling ${VAR}) but never resolves or executes it.
	Command string `json:"command,omitempty"`
	// CommandArgs are the arguments passed to Command (run directly, no shell — no
	// pipes/redirects/chaining). Each may carry a ${VAR} reference. Meaningless
	// without a Command, so Validate rejects args with an empty Command. — ADR-0032.
	CommandArgs []string `json:"commandArgs,omitempty"`
}

// HasCommand reports whether the hook carries a PostToolUse command to execute
// (G5, ADR-0032). A whitespace-only Command does not count.
func (h Hook) HasCommand() bool { return strings.TrimSpace(h.Command) != "" }

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
	for _, m := range h.Modes {
		if !validHookModes[m] {
			return fmt.Errorf("hook %q: invalid mode %q", h.ID, m)
		}
	}
	if err := h.validateCommand(); err != nil {
		return err
	}
	return nil
}

// validateCommand enforces the PostToolUse command field's shape (G5, ADR-0032):
// a command is allowed ONLY on a post-tool-use hook (a PreToolUse hook carrying a
// command is rejected so untrusted output can never become a pre-gate control
// surface); CommandArgs require a non-empty Command; and neither the Command nor
// any arg may contain a dangling ${VAR} (the well-formed shape is resolved at
// execution by the seam, ADR-0020). A hook with no command is unaffected.
func (h Hook) validateCommand() error {
	if !h.HasCommand() {
		if len(h.CommandArgs) > 0 {
			return fmt.Errorf("hook %q: commandArgs require a command", h.ID)
		}
		return nil
	}
	if h.Event != HookPostToolUse {
		return fmt.Errorf("hook %q: a command is only valid on a post-tool-use hook", h.ID)
	}
	if hasDanglingVarRef(h.Command) {
		return fmt.Errorf("hook %q: dangling ${VAR} reference in command", h.ID)
	}
	for _, a := range h.CommandArgs {
		if hasDanglingVarRef(a) {
			return fmt.Errorf("hook %q: dangling ${VAR} reference in command args", h.ID)
		}
	}
	return nil
}

// appliesInMode reports whether the hook participates when the session's active
// mode is mode. An empty Modes set applies in every mode (mode binding,
// ADR-0031); otherwise the active mode must be listed.
func (h Hook) appliesInMode(mode string) bool {
	if len(h.Modes) == 0 {
		return true
	}
	for _, m := range h.Modes {
		if m == mode {
			return true
		}
	}
	return false
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
	// A target that references the home directory (`~`) or carries an unexpanded
	// shell variable (`$HOME`, `${VAR}`) is NOT a workspace-relative path — joining
	// it onto the root would wrongly judge `~/.ssh/authorized_keys` "inside" the
	// tree. Treat such targets as outside so the fence gates them (fail-safe),
	// mirroring how the dangerous ruleset treats `~`/`$HOME` for `rm`.
	if strings.HasPrefix(target, "~") || strings.ContainsRune(target, '$') {
		return true
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

// PatternIsGlob reports whether a hook Pattern uses the glob wildcards '*'/'?'
// (an unanchored wildcard search) rather than plain substring semantics. The UI
// preflight surfaces which form a typed pattern takes before it is saved.
// — ADR-0031.
func PatternIsGlob(pattern string) bool { return strings.ContainsAny(pattern, "*?") }

// MatchPattern reports whether a hook Pattern matches command under the same
// glob/substring rules the evaluator applies — the single matcher, exported so
// the UI preflight can show a rule firing against a sample command before it is
// saved. — ADR-0031.
func MatchPattern(pattern, command string) bool { return patternMatch(pattern, command) }

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
	Action string
	Reason string
	// HookID is the id of the winning hook (empty for the no-match default ask).
	// The bridge uses it to explain a decision in the timeline ("auto-approved by
	// hook X") and to distinguish a built-in (builtin-* id) from a user hook so the
	// safe-read default's auto-approve stays silent while a user allow is surfaced.
	// — ADR-0031.
	HookID    string
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
// powers the OutsideWorkspace fence (empty = fence inert). mode is the session's
// active agent mode (mode binding, ADR-0031): a hook participates only when its
// Modes set is empty or lists mode, so the mandatory ruleset (empty Modes) holds
// in EVERY mode while a user hook can be scoped to one. The function is pure —
// built-in safe-read defaults, the built-in dangerous ruleset, and user hooks all
// evaluate through this one path. — ADR-0029, ADR-0030, ADR-0031.
func Evaluate(hooks []Hook, event, toolKind, command, workspace, mode string) Decision {
	var deny, ask, allow *Hook
	var mandatoryDeny, mandatoryAsk *Hook
	for i := range hooks {
		h := &hooks[i]
		if !h.Enabled || h.Event != event || !h.appliesInMode(mode) || !h.Match.matches(toolKind, command, workspace) {
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
		return Decision{Action: HookDeny, Reason: win.Reason, HookID: win.ID, Mandatory: mandatoryDeny != nil}
	case ask != nil:
		win := ask
		if mandatoryAsk != nil {
			win = mandatoryAsk
		}
		return Decision{Action: HookAsk, Reason: win.Reason, HookID: win.ID, Mandatory: mandatoryAsk != nil}
	case allow != nil:
		return Decision{Action: HookAllow, Reason: allow.Reason, HookID: allow.ID}
	default:
		return Decision{Action: HookAsk}
	}
}

// PostToolUseCommands selects, in declared order, the enabled PostToolUse hooks
// that carry a command and whose Match applies to a completed tool call of the
// given kind, command, and workspace under the active mode (G5, ADR-0032). It is
// the pure companion to Evaluate for the POST-tool path: unlike Evaluate it makes
// no allow/deny/ask decision (a post-tool command never gates) — it just returns
// which command hooks should fire, leaving ${VAR} resolution and execution to the
// seam. Mode binding (ADR-0031) and the Match dimensions apply exactly as for the
// pre-tool path; a hook without a command is ignored.
func PostToolUseCommands(hooks []Hook, toolKind, command, workspace, mode string) []Hook {
	var out []Hook
	for i := range hooks {
		h := &hooks[i]
		if !h.Enabled || h.Event != HookPostToolUse || !h.HasCommand() {
			continue
		}
		if !h.appliesInMode(mode) || !h.Match.matches(toolKind, command, workspace) {
			continue
		}
		out = append(out, *h)
	}
	return out
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

// reservedHookID rejects a user hook that claims the builtin- id prefix reserved
// for the shipped built-in policy (DefaultHooks/DangerousHooks). The built-ins
// own the prefix and never flow through Add/UpdateHook, so reserving it here (not
// in Validate, which the built-ins must pass) keeps the UI's built-in vs user
// distinction — read-only rows and the timeline "auto-approved by X" suppression —
// from being spoofed by a user hook. — ADR-0031.
func reservedHookID(id string) error {
	if strings.HasPrefix(id, "builtin-") {
		return fmt.Errorf("hook %q: the \"builtin-\" id prefix is reserved for built-in hooks", id)
	}
	return nil
}

// AddHook validates and appends a hook, rejecting duplicate IDs.
func (f *Forge) AddHook(h Hook) error {
	return f.mutate(func() error {
		if err := h.Validate(); err != nil {
			return err
		}
		if err := reservedHookID(h.ID); err != nil {
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
		if err := reservedHookID(h.ID); err != nil {
			return err
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
// clearly-destructive patterns are hard-denied and risky-but-heuristic ones are
// force-gated, even when a session runs with AutoApproveTools — config alone
// cannot bypass them (Hook.Mandatory; ADR-0030). They run through the SAME
// Evaluate as the safe-read defaults and user hooks, so deny > ask > allow holds:
// a user deny (more restrictive) still wins over a mandatory ask, but a user allow
// (or a blanket auto-approve) can never weaken a mandatory deny/ask. Compile folds
// these into every session's policy. The set has a deterministic order.
//
// Patterns are unanchored substrings/globs (matching anywhere, so a leading token
// can't dodge them) and are deliberately conservative — defense-in-depth at the
// permission gate, not a hardened sandbox. Where a substring is heuristic enough to
// hit a benign command (a credential-store path that could appear in a URL), the
// rule is a mandatory **gate** (ask) rather than a hard deny, so a false positive
// asks a human instead of an unoverridable block. Accepted/documented matcher limits:
// a recursive force-delete of any ABSOLUTE path under `/` or `~`/`$HOME` is denied
// (relative `rm -rf ./build` is left to the gate); a shell/editor token sharing a
// prefix with the pipe target (`curl … | sha256sum`) is a rare residual over-match;
// and non-pipe netcat (`nc host < file`) and exotic obfuscation (process
// substitution, unusual spacing) are out of scope for the string matcher.
func DangerousHooks() []Hook {
	deny := func(id, pattern, reason string) Hook {
		return Hook{ID: id, Event: HookPreToolUse, Match: HookMatch{ToolKind: "shell", Pattern: pattern}, Action: HookDeny, Reason: reason, Mandatory: true, Enabled: true}
	}
	gate := func(id, pattern, reason string) Hook {
		return Hook{ID: id, Event: HookPreToolUse, Match: HookMatch{ToolKind: "shell", Pattern: pattern}, Action: HookAsk, Reason: reason, Mandatory: true, Enabled: true}
	}
	hooks := []Hook{
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
		// Pipe data to netcat — a reverse shell / data tunnel, almost never benign.
		// The space-delimited `nc ` avoids matching `sync`/`rsync`/`func`.
		deny("builtin-deny-pipe-netcat", "| nc ", "blocked: piping data to netcat (exfiltration / reverse shell)"),
		deny("builtin-deny-pipe-netcat-nospace", "|nc ", "blocked: piping data to netcat (exfiltration / reverse shell)"),
	}
	// Pipe a download straight into a shell interpreter (sh/bash) or an editor
	// (vim/nano) — remote code execution. The target token must follow the pipe
	// DIRECTLY (no `*` between the pipe and the token), so a benign later "sh"
	// substring — `curl … | grep ssh`, `curl … | less` — does NOT match; only a
	// real pipe-into-interpreter does. The `*` before the pipe still allows the
	// URL + flags. Both spaced (`| sh`) and tight (`|sh`) forms are covered.
	pipeTargets := []struct{ tok, reason string }{
		{"sh", "blocked: piping a download into a shell (remote code execution)"},
		{"bash", "blocked: piping a download into a shell (remote code execution)"},
		{"vim", "blocked: piping a download into an editor (executes editor macros)"},
		{"nano", "blocked: piping a download into an editor (executes editor macros)"},
	}
	for _, dl := range []string{"curl", "wget"} {
		for _, t := range pipeTargets {
			for _, sep := range []struct{ id, s string }{{"sp", "| "}, {"tight", "|"}} {
				hooks = append(hooks, deny(
					fmt.Sprintf("builtin-deny-%s-pipe-%s-%s", dl, t.tok, sep.id),
					dl+"*"+sep.s+t.tok, t.reason))
			}
		}
	}
	// Sending an SSH private key over the network is unambiguous exfiltration.
	for _, dl := range []string{"curl", "wget"} {
		hooks = append(hooks, deny("builtin-deny-"+dl+"-ssh-key", dl+"*id_rsa",
			"blocked: sending an SSH private key over the network"))
	}
	hooks = append(hooks,
		// curl referencing a well-known credential STORE (.ssh dir, AWS creds,
		// .netrc). These substrings can also appear in a benign URL path, so they
		// are force-gated (mandatory ask) rather than hard-denied — a human confirms
		// instead of an unoverridable block on a possible false positive.
		gate("builtin-ask-curl-ssh-dir", "curl*.ssh/", "confirm: command references .ssh material over the network"),
		gate("builtin-ask-curl-aws-creds", "curl*.aws/credentials", "confirm: command references AWS credentials over the network"),
		gate("builtin-ask-curl-netrc", "curl*.netrc", "confirm: command references .netrc credentials over the network"),
		// sudo — privilege escalation. Sometimes legitimate, so it is force-gated
		// (mandatory ask) rather than hard-denied: a human must approve even in auto.
		gate("builtin-ask-sudo", "sudo ", "confirm: sudo escalates privileges"),
		// A write whose target resolves OUTSIDE the session workspace — the path-aware
		// fence (ADR-0030). Legitimate sometimes (writing to /tmp, ~/.config), so it is
		// force-gated, not denied; the fence is inert when no workspace root is known.
		Hook{
			ID: "builtin-ask-write-outside-workspace", Event: HookPreToolUse,
			Match:  HookMatch{ToolKind: "write", OutsideWorkspace: true},
			Action: HookAsk, Reason: "confirm: write target is outside the workspace", Mandatory: true, Enabled: true,
		},
	)
	return hooks
}
