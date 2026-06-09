package copilot

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
)

// This file is the PostToolUse hook EXECUTOR (G5, ADR-0032). When a tool call
// completes the bridge consults the compiled policy for matching PostToolUse
// command hooks (ctxforge.PostToolUseCommands) and runs each hook's resolved
// command — a single bounded local process, no shell — with the workspace as cwd.
// The command's stdout/stderr is treated as UNTRUSTED: it is bounded, never fed
// back to the agent, and surfaced only as an EvHookRun timeline annotation; it can
// NEVER flip a permission/decision (those are resolved pre-tool, before any
// command runs). ${VAR} references resolve here via the same env seam as MCP
// (ADR-0020) — the secret is never stored, only the reference, and a missing
// reference resolves to UNSET (empty), never the literal "${VAR}".

const (
	// defaultHookTimeout bounds a single PostToolUse command — a post-tool hook is
	// telemetry/side-effect, not an interactive step, so it must not stall the turn.
	defaultHookTimeout = 5 * time.Second
	// hookOutputCap bounds the captured output (~2KB) so untrusted output can never
	// flood memory or the timeline. Capture stops once the cap is reached.
	hookOutputCap = 2048
)

// commandRunner executes a single program (no shell) in dir with args under ctx,
// returning the combined, bounded stdout/stderr, the process exit code (-1 if it
// never started or was killed), and any start/exec error. It is a seam so the
// executor is unit-testable without spawning real processes.
type commandRunner func(ctx context.Context, dir, name string, args []string) (output string, exitCode int, err error)

// hookVarRef matches an inline ${VAR} reference (UPPER_SNAKE) — the resolution
// half of the MCP env shape (ADR-0020), applied to a hook command at execution.
var hookVarRef = regexp.MustCompile(`\$\{([A-Z_][A-Z0-9_]*)\}`)

// resolveVarRefs expands every ${VAR} in s via the client's env lookup (default
// os.Getenv). An unset reference resolves to the empty string — never the literal
// "${VAR}" — mirroring resolveEnv (ADR-0020): the secret lives only in the
// environment, and an absent one leaves no trace of the reference.
func (c *SDKClient) resolveVarRefs(s string) string {
	lookup := c.lookupEnv
	if lookup == nil {
		lookup = os.Getenv
	}
	return hookVarRef.ReplaceAllStringFunc(s, func(ref string) string {
		name := ref[2 : len(ref)-1] // strip "${" and "}"
		return lookup(name)
	})
}

// hookTimeoutOrDefault is the per-command deadline (a field lets tests shorten it).
func (c *SDKClient) hookTimeoutOrDefault() time.Duration {
	if c.hookTimeout > 0 {
		return c.hookTimeout
	}
	return defaultHookTimeout
}

// firePostToolHooks runs every matching PostToolUse command hook for a completed
// tool call and emits an EvHookRun per hook. It runs in its own goroutine (off the
// SDK event-handler path) so a hook command's latency never blocks event delivery;
// the resulting annotations land in the timeline asynchronously, which is correct
// for display-only telemetry. Each event is stamped with the session id.
func (c *SDKClient) firePostToolHooks(sid string, hooks []ctxforge.Hook, workspace string) {
	for _, h := range hooks {
		e := c.runPostToolHook(h, workspace)
		e.SessionID = sid
		c.emit(e)
	}
}

// runPostToolHook resolves a hook's ${VAR} references and runs its command, then
// returns the result as an EvHookRun event. It never returns a permission/decision
// — a post-tool command is telemetry, not a gate — so its (untrusted) output
// cannot influence whether any tool runs. Output is bounded to hookOutputCap.
func (c *SDKClient) runPostToolHook(h ctxforge.Hook, workspace string) Event {
	name := c.resolveVarRefs(strings.TrimSpace(h.Command))
	args := make([]string, len(h.CommandArgs))
	for i, a := range h.CommandArgs {
		args[i] = c.resolveVarRefs(a)
	}

	runner := c.runCmd
	if runner == nil {
		runner = execCommand
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.hookTimeoutOrDefault())
	defer cancel()
	out, code, err := runner(ctx, workspace, name, args)
	timedOut := ctx.Err() == context.DeadlineExceeded

	// Surface a START failure (e.g. program not found) the user would otherwise
	// see only as a bare "exited -1": when the command never produced output and
	// failed without timing out, show the error so a typo'd command is diagnosable.
	// The error text is our own/OS text, escaped like all output (ADR-0001).
	if out == "" && err != nil && !timedOut {
		out = err.Error()
	}

	return Event{Type: EvHookRun, HookRun: &HookRun{
		HookID:   h.ID,
		Command:  commandLine(name, args),
		Output:   capOutput(out),
		ExitCode: code,
		TimedOut: timedOut,
		Failed:   err != nil || timedOut || code != 0,
	}}
}

// execCommand is the production commandRunner: it execs the program DIRECTLY (no
// shell — no pipes/redirects/chaining), captures stdout+stderr into one bounded
// buffer, and runs with dir as cwd. The exit code is -1 when the process never
// started or was killed (e.g. on timeout). A missing program name yields a start
// error, surfaced as a failed run rather than a panic.
func execCommand(ctx context.Context, dir, name string, args []string) (string, int, error) {
	if strings.TrimSpace(name) == "" {
		return "", -1, exec.ErrNotFound
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	buf := &capWriter{cap: hookOutputCap}
	cmd.Stdout = buf
	cmd.Stderr = buf
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = -1
		}
	}
	return buf.String(), code, err
}

// capWriter is an io.Writer that accumulates at most cap bytes and silently drops
// the rest, so an unbounded/streaming command can never exhaust memory. — ADR-0032.
type capWriter struct {
	buf bytes.Buffer
	cap int
}

func (w *capWriter) Write(p []byte) (int, error) {
	if remaining := w.cap - w.buf.Len(); remaining > 0 {
		if len(p) > remaining {
			w.buf.Write(p[:remaining])
		} else {
			w.buf.Write(p)
		}
	}
	// Always report a full write so the child process is never blocked by our cap.
	return len(p), nil
}

func (w *capWriter) String() string { return w.buf.String() }

// capOutput bounds a captured output string to hookOutputCap bytes and trims
// trailing whitespace, then drops any trailing INCOMPLETE UTF-8 rune so the
// snippet is always valid text. The capWriter already caps the production runner
// at exactly hookOutputCap (and can split a multi-byte rune at that byte
// boundary), so the rune-trim must run on a string that is *not* over-cap too;
// it also defends against a runner that doesn't cap itself (e.g. a test fake).
func capOutput(s string) string {
	s = strings.TrimRight(s, " \t\r\n")
	if len(s) > hookOutputCap {
		s = s[:hookOutputCap]
	}
	// Drop a trailing partial rune (continuation bytes with no complete leader).
	b := []byte(s)
	for len(b) > 0 && !utf8.ValidString(string(b)) {
		b = b[:len(b)-1]
	}
	return strings.TrimRight(string(b), " \t\r\n")
}

// commandLine renders a resolved command + args as one display line for the
// timeline annotation. Args are space-joined; this is display-only (the process
// was already run from the argv directly, not from this string).
func commandLine(name string, args []string) string {
	if len(args) == 0 {
		return name
	}
	return name + " " + strings.Join(args, " ")
}
