package ctxforge

import (
	"encoding/json"
	"testing"
)

// The PostToolUse command field (G5, ADR-0032): a forge-backed external command a
// PostToolUse hook runs after a matching tool completes. The domain stays pure —
// it validates the field's shape (post-only, slug, no dangling ${VAR}) and selects
// the matching command hooks; the seam resolves ${VAR} and executes.

func postCommandHook() Hook {
	return Hook{
		ID:          "fmt-go",
		Event:       HookPostToolUse,
		Match:       HookMatch{ToolKind: "write", Pattern: ".go"},
		Action:      HookAsk, // inert for post-tool — kept for a uniform schema
		Command:     "gofmt",
		CommandArgs: []string{"-w", "${TARGET_FILE}"},
		Enabled:     true,
	}
}

func TestHookCommandValidatesOnPostOnly(t *testing.T) {
	if err := postCommandHook().Validate(); err != nil {
		t.Fatalf("a well-formed post-tool-use command hook should validate: %v", err)
	}
	// A PreToolUse hook carrying a command is rejected — an untrusted command must
	// never become a pre-gate control surface (ADR-0032).
	pre := postCommandHook()
	pre.Event = HookPreToolUse
	if err := pre.Validate(); err == nil {
		t.Fatal("a pre-tool-use hook with a command must be rejected")
	}
}

func TestHookCommandRejectsDanglingVarRef(t *testing.T) {
	bad := postCommandHook()
	bad.Command = "gofmt ${BAD"
	if err := bad.Validate(); err == nil {
		t.Fatal("a dangling ${VAR in Command must be rejected")
	}
	badArg := postCommandHook()
	badArg.CommandArgs = []string{"-w", "${}"}
	if err := badArg.Validate(); err == nil {
		t.Fatal("a malformed ${} in CommandArgs must be rejected")
	}
	// A well-formed ${VAR} is allowed (resolved at execution by the seam).
	ok := postCommandHook()
	ok.Command = "${FORMATTER}"
	if err := ok.Validate(); err != nil {
		t.Fatalf("a well-formed ${VAR} should be allowed: %v", err)
	}
}

func TestHookCommandArgsRequireCommand(t *testing.T) {
	// CommandArgs without a Command is meaningless — there is nothing to execute.
	bad := postCommandHook()
	bad.Command = "   "
	if err := bad.Validate(); err == nil {
		t.Fatal("CommandArgs with an empty Command must be rejected")
	}
}

func TestHookHasCommand(t *testing.T) {
	if !postCommandHook().HasCommand() {
		t.Fatal("a hook with a Command should report HasCommand")
	}
	plain := Hook{ID: "x", Event: HookPostToolUse, Match: HookMatch{ToolKind: "read"}, Action: HookAsk}
	if plain.HasCommand() {
		t.Fatal("a hook with no Command must not report HasCommand")
	}
}

// A pre-G5 forge.json (no command/commandArgs keys) loads unchanged and validates
// — the field is additive and backward-readable (omitempty).
func TestHookBackwardReadableWithoutCommand(t *testing.T) {
	const pre = `{"id":"old","event":"post-tool-use","match":{"toolKind":"write"},"action":"ask","enabled":true}`
	var h Hook
	if err := json.Unmarshal([]byte(pre), &h); err != nil {
		t.Fatalf("pre-G5 hook should unmarshal: %v", err)
	}
	if h.HasCommand() {
		t.Fatal("a pre-G5 hook carries no command")
	}
	if err := h.Validate(); err != nil {
		t.Fatalf("a pre-G5 hook should still validate: %v", err)
	}
}

func TestPostToolUseCommandsSelectsMatching(t *testing.T) {
	match := postCommandHook()
	disabled := postCommandHook()
	disabled.ID, disabled.Enabled = "disabled", false
	noCmd := Hook{ID: "nocmd", Event: HookPostToolUse, Match: HookMatch{ToolKind: "write"}, Action: HookAsk, Enabled: true}
	preCmd := postCommandHook()
	preCmd.ID, preCmd.Event = "pre", HookPreToolUse // a pre hook never runs as a post command
	wrongKind := postCommandHook()
	wrongKind.ID, wrongKind.Match = "wrong", HookMatch{ToolKind: "shell"}

	hooks := []Hook{match, disabled, noCmd, preCmd, wrongKind}
	got := PostToolUseCommands(hooks, "write", "write main.go", "", "")
	if len(got) != 1 || got[0].ID != "fmt-go" {
		t.Fatalf("PostToolUseCommands = %+v, want only fmt-go", got)
	}
	// Mode binding still applies: a hook scoped to autopilot is excluded in plan.
	scoped := postCommandHook()
	scoped.ID, scoped.Modes = "scoped", []string{ModeAutopilot}
	if got := PostToolUseCommands([]Hook{scoped}, "write", "write main.go", "", ModePlan); len(got) != 0 {
		t.Fatalf("a mode-scoped command hook must not match outside its mode: %+v", got)
	}
}
