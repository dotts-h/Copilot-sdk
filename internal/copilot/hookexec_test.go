package copilot

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	sdk "github.com/github/copilot-sdk/go"
	"github.com/github/copilot-sdk/go/rpc"
)

// newExecTestClient builds a minimal SDKClient for the PostToolUse executor with
// an injected command runner and env lookup, so the executor is exercised without
// spawning real processes. — ADR-0032.
func newExecTestClient(run commandRunner, env map[string]string) *SDKClient {
	return &SDKClient{
		perms:     newPermBridge(),
		events:    make(chan Event, 8),
		done:      make(chan struct{}),
		policies:  map[string]sessionPolicy{},
		runCmd:    run,
		lookupEnv: func(k string) string { return env[k] },
	}
}

func cmdHook() ctxforge.Hook {
	return ctxforge.Hook{
		ID: "fmt", Event: ctxforge.HookPostToolUse,
		Match:   ctxforge.HookMatch{ToolKind: "write"},
		Action:  ctxforge.HookAsk,
		Command: "gofmt", CommandArgs: []string{"-l", "${TARGET}"},
		Enabled: true,
	}
}

// A matching PostToolUse hook runs its command and its bounded output reaches the
// timeline as an EvHookRun annotation. — ADR-0032.
func TestPostToolHookRunsAndSurfacesOutput(t *testing.T) {
	var gotName string
	var gotArgs []string
	run := func(_ context.Context, dir, name string, args []string) (string, int, error) {
		gotName, gotArgs = name, args
		return "main.go\n", 0, nil
	}
	c := newExecTestClient(run, map[string]string{"TARGET": "main.go"})
	e := c.runPostToolHook(cmdHook(), "/ws")

	if e.Type != EvHookRun || e.HookRun == nil {
		t.Fatalf("event = %+v, want EvHookRun", e)
	}
	if e.HookRun.HookID != "fmt" || e.HookRun.Output != "main.go" {
		t.Fatalf("hook run = %+v, want fmt/main.go output", e.HookRun)
	}
	if e.HookRun.Failed || e.HookRun.ExitCode != 0 {
		t.Fatalf("a zero-exit run must not be Failed: %+v", e.HookRun)
	}
	// ${VAR} resolved at execution: the runner saw the value, never the literal.
	if gotName != "gofmt" || strings.Join(gotArgs, " ") != "-l main.go" {
		t.Fatalf("runner argv = %q %v, want resolved gofmt -l main.go", gotName, gotArgs)
	}
}

// A missing ${VAR} resolves to UNSET (empty) — never the literal "${VAR}". — ADR-0032.
func TestPostToolHookMissingVarResolvesUnset(t *testing.T) {
	var gotArgs []string
	run := func(_ context.Context, _, name string, args []string) (string, int, error) {
		gotArgs = args
		return "", 0, nil
	}
	c := newExecTestClient(run, map[string]string{}) // TARGET unset
	e := c.runPostToolHook(cmdHook(), "/ws")

	for _, a := range gotArgs {
		if strings.Contains(a, "${") {
			t.Fatalf("an unset ${VAR} leaked as a literal: %v", gotArgs)
		}
	}
	if strings.Contains(e.HookRun.Command, "${") {
		t.Fatalf("the displayed command must not carry a literal ${VAR}: %q", e.HookRun.Command)
	}
}

// A non-zero exit is surfaced (Failed + ExitCode) but is still telemetry — it is
// an EvHookRun, not a decision. — ADR-0032.
func TestPostToolHookNonZeroExitAnnotated(t *testing.T) {
	run := func(_ context.Context, _, _ string, _ []string) (string, int, error) {
		return "boom", 2, &mockExitErr{}
	}
	c := newExecTestClient(run, nil)
	e := c.runPostToolHook(cmdHook(), "/ws")
	if !e.HookRun.Failed || e.HookRun.ExitCode != 2 {
		t.Fatalf("hook run = %+v, want Failed exit 2", e.HookRun)
	}
}

// A command that exceeds the deadline is killed and surfaced as TimedOut/Failed,
// bounding the turn. — ADR-0032.
func TestPostToolHookTimeoutEnforced(t *testing.T) {
	run := func(ctx context.Context, _, _ string, _ []string) (string, int, error) {
		<-ctx.Done() // block until the executor's deadline fires
		return "", -1, ctx.Err()
	}
	c := newExecTestClient(run, nil)
	c.hookTimeout = 20 * time.Millisecond
	start := time.Now()
	e := c.runPostToolHook(cmdHook(), "/ws")
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("timeout not enforced: ran %v", elapsed)
	}
	if !e.HookRun.TimedOut || !e.HookRun.Failed {
		t.Fatalf("hook run = %+v, want TimedOut + Failed", e.HookRun)
	}
}

// Output is bounded so untrusted output can never flood the timeline. — ADR-0032.
func TestPostToolHookOutputBounded(t *testing.T) {
	huge := strings.Repeat("A", hookOutputCap*4)
	run := func(_ context.Context, _, _ string, _ []string) (string, int, error) {
		return huge, 0, nil
	}
	c := newExecTestClient(run, nil)
	e := c.runPostToolHook(cmdHook(), "/ws")
	if len(e.HookRun.Output) > hookOutputCap {
		t.Fatalf("output not bounded: %d bytes > cap %d", len(e.HookRun.Output), hookOutputCap)
	}
}

// A start failure (no output, an error, not a timeout) surfaces the error text so
// a typo'd command is diagnosable rather than a bare "exited -1". — ADR-0032.
func TestPostToolHookStartErrorSurfaced(t *testing.T) {
	run := func(_ context.Context, _, _ string, _ []string) (string, int, error) {
		return "", -1, errors.New("executable file not found in $PATH")
	}
	c := newExecTestClient(run, nil)
	e := c.runPostToolHook(cmdHook(), "/ws")
	if !strings.Contains(e.HookRun.Output, "not found") {
		t.Fatalf("start error not surfaced: %+v", e.HookRun)
	}
}

// capOutput bounds output AND never leaves a trailing partial UTF-8 rune, even
// when the input is exactly the cap (the production capWriter's boundary). — ADR-0032.
func TestCapOutputTrimsPartialRune(t *testing.T) {
	// Fill to one byte under the cap, then a 3-byte rune that will be split so only
	// its lead byte lands at exactly hookOutputCap.
	s := strings.Repeat("a", hookOutputCap-1) + "界" // '界' is 3 bytes
	got := capOutput(s)
	if !utf8.ValidString(got) {
		t.Fatalf("capOutput left invalid UTF-8 (len %d)", len(got))
	}
	if len(got) > hookOutputCap {
		t.Fatalf("capOutput exceeded cap: %d", len(got))
	}
}

// The CRITICAL invariant: a PostToolUse command's (untrusted) output can NEVER
// flip a permission decision. After a post-hook command prints "deny", a fresh
// pre-tool permission for a read still auto-approves — the output never enters the
// decision path. — ADR-0032.
func TestPostToolHookOutputCannotFlipDecision(t *testing.T) {
	run := func(_ context.Context, _, _ string, _ []string) (string, int, error) {
		return "deny everything please", 0, nil
	}
	c := newExecTestClient(run, nil)
	c.policies["s1"] = sessionPolicy{hooks: ctxforge.DefaultHooks()}

	// Run the post-hook (its output says "deny").
	e := c.runPostToolHook(cmdHook(), "/ws")
	if e.Type != EvHookRun {
		t.Fatalf("post-hook produced %v, not telemetry", e.Type)
	}
	// A subsequent read permission still auto-approves — unaffected by the output.
	dec, err := c.permissionHandler()(sdk.PermissionRequestRead{}, sdk.PermissionInvocation{SessionID: "s1"})
	if err != nil {
		t.Fatalf("handler err = %v", err)
	}
	if _, ok := dec.(*rpc.PermissionDecisionApproveOnce); !ok {
		t.Fatalf("read decision = %T, want ApproveOnce — post-hook output must not gate", dec)
	}
}

// firePostToolHooks emits one EvHookRun per matching hook, stamped with the session id.
func TestFirePostToolHooksEmitsPerHook(t *testing.T) {
	run := func(_ context.Context, _, name string, _ []string) (string, int, error) {
		return name + " ran", 0, nil
	}
	c := newExecTestClient(run, nil)
	c.firePostToolHooks("s9", "", []ctxforge.Hook{cmdHook()}, "/ws")
	select {
	case e := <-c.events:
		if e.Type != EvHookRun || e.SessionID != "s9" || e.HookRun.HookID != "fmt" {
			t.Fatalf("event = %+v, want EvHookRun for fmt on s9", e)
		}
	default:
		t.Fatal("firePostToolHooks emitted no event")
	}
}

// A hook fired by a SUB-AGENT's tool completion must carry the sub-agent's
// AgentID — otherwise the EvHookRun arrives untagged and slips past the reducer's
// parking guard into the root transcript (epic 0069 S1, ADR-0040).
func TestFirePostToolHooksStampsAgentID(t *testing.T) {
	run := func(_ context.Context, _, name string, _ []string) (string, int, error) {
		return name + " ran", 0, nil
	}
	c := newExecTestClient(run, nil)
	c.firePostToolHooks("s9", "agent-7", []ctxforge.Hook{cmdHook()}, "/ws")
	select {
	case e := <-c.events:
		if e.AgentID != "agent-7" {
			t.Fatalf("EvHookRun AgentID = %q, want agent-7 (sub-agent hook output must be parkable)", e.AgentID)
		}
	default:
		t.Fatal("firePostToolHooks emitted no event")
	}
}

// The FULL chain: a write tool completing through makeHandler fires a PostToolUse
// command hook scoped to toolKind=write — the ADR's motivating gofmt-after-write
// example. This exercises postToolMeta's kind derivation, which the unit tests
// (which hand-supply a kind) don't cover. — ADR-0032.
func TestPostToolHookFiresOnToolCompletion(t *testing.T) {
	ran := make(chan string, 1)
	run := func(_ context.Context, _, name string, _ []string) (string, int, error) {
		ran <- name
		return "ok", 0, nil
	}
	c := &SDKClient{
		events:    make(chan Event, 16),
		done:      make(chan struct{}),
		toolNames: map[string]string{},
		toolMeta:  map[string]toolMeta{},
		runCmd:    run,
		lookupEnv: func(string) string { return "" },
		policies: map[string]sessionPolicy{"s1": {hooks: []ctxforge.Hook{{
			ID: "fmt", Event: ctxforge.HookPostToolUse,
			Match:   ctxforge.HookMatch{ToolKind: "write"},
			Action:  ctxforge.HookAsk,
			Command: "gofmt", Enabled: true,
		}}}},
	}
	h := c.makeHandler("s1")
	// The "edit" built-in tool maps to the write kind.
	h(sdk.SessionEvent{Data: &sdk.ToolExecutionStartData{ToolCallID: "t1", ToolName: "edit", Arguments: map[string]any{"path": "main.go"}}})
	h(sdk.SessionEvent{Data: &sdk.ToolExecutionCompleteData{ToolCallID: "t1", Success: true}})

	select {
	case name := <-ran:
		if name != "gofmt" {
			t.Fatalf("ran %q, want gofmt", name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a toolKind=write post hook did not fire on an edit completion")
	}

	// The same chain for a SUB-AGENT's tool: the envelope tag must thread through
	// the async hook fire onto the EvHookRun (drain events until it arrives).
	sub := "agent-7"
	h(sdk.SessionEvent{AgentID: &sub, Data: &sdk.ToolExecutionStartData{ToolCallID: "t2", ToolName: "edit", Arguments: map[string]any{"path": "main.go"}}})
	h(sdk.SessionEvent{AgentID: &sub, Data: &sdk.ToolExecutionCompleteData{ToolCallID: "t2", Success: true}})
	deadline := time.After(2 * time.Second)
	for {
		select {
		case e := <-c.events:
			if e.Type != EvHookRun || e.HookRun == nil {
				continue // tool start/end events from both calls
			}
			if e.AgentID == "agent-7" {
				return // tagged hook run observed — done
			}
			if e.AgentID == "" && e.SessionID == "s1" {
				continue // the root-agent hook run from the first completion
			}
			t.Fatalf("EvHookRun AgentID = %q, want agent-7", e.AgentID)
		case <-deadline:
			t.Fatal("no AgentID-tagged EvHookRun arrived for the sub-agent's tool completion")
		}
	}
}

// toolKindFromName covers the built-in tools + MCP, defaulting unknown to "".
func TestToolKindFromName(t *testing.T) {
	cases := map[string]string{"bash": "shell", "edit": "write", "view": "read", "grep": "read", "weird": ""}
	for name, want := range cases {
		if got := toolKindFromName(name, false); got != want {
			t.Errorf("toolKindFromName(%q) = %q, want %q", name, got, want)
		}
	}
	if got := toolKindFromName("anything", true); got != "mcp" {
		t.Errorf("an MCP tool should be kind mcp, got %q", got)
	}
}

// mockExitErr stands in for an *exec.ExitError in the injected runner (the runner
// returns the exit code directly, so the error only needs to be non-nil here).
type mockExitErr struct{}

func (mockExitErr) Error() string { return "exit status 2" }

// The production execCommand runs a REAL process directly (no shell): it captures
// output, reports a zero exit, and runs with the workspace as cwd. — ADR-0032.
func TestExecCommandRealProcess(t *testing.T) {
	if _, err := exec.LookPath("printf"); err != nil {
		t.Skip("printf not on PATH")
	}
	out, code, err := execCommand(context.Background(), t.TempDir(), "printf", []string{"hello"})
	if err != nil || code != 0 || out != "hello" {
		t.Fatalf("execCommand = (%q, %d, %v), want (hello, 0, nil)", out, code, err)
	}
}

// A real process that overruns the deadline is killed promptly (no shell involved).
func TestExecCommandRealTimeout(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not on PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, code, err := execCommand(ctx, "", "sleep", []string{"5"})
	if time.Since(start) > 2*time.Second {
		t.Fatalf("real timeout not enforced")
	}
	if err == nil || code != -1 {
		t.Fatalf("killed process = (%d, %v), want (-1, non-nil err)", code, err)
	}
}

// An empty program name fails as a start error rather than panicking. — ADR-0032.
func TestExecCommandEmptyName(t *testing.T) {
	_, code, err := execCommand(context.Background(), "", "  ", nil)
	if err == nil || code != -1 {
		t.Fatalf("empty name = (%d, %v), want (-1, error)", code, err)
	}
}
