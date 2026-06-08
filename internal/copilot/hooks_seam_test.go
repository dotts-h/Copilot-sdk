package copilot

import (
	"testing"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	sdk "github.com/github/copilot-sdk/go"
	"github.com/github/copilot-sdk/go/rpc"
)

// newPolicyTestClient builds a minimal SDKClient exercising only the permission
// bridge + hook policy, without a live runtime, so the seam's allow/deny/ask
// behavior is unit-testable like the other handler-level tests in this package.
func newPolicyTestClient(hooks []ctxforge.Hook) *SDKClient {
	return &SDKClient{
		perms:  newPermBridge(),
		events: make(chan Event, 8),
		done:   make(chan struct{}),
		hooks:  map[string][]ctxforge.Hook{"s1": hooks},
	}
}

func TestPermissionHandlerAllowAutoApprovesWithoutGate(t *testing.T) {
	c := newPolicyTestClient([]ctxforge.Hook{
		{ID: "a", Event: ctxforge.HookPreToolUse, Match: ctxforge.HookMatch{ToolKind: "read"}, Action: ctxforge.HookAllow, Enabled: true},
	})
	h := c.permissionHandler()
	dec, err := h(sdk.PermissionRequestRead{}, sdk.PermissionInvocation{SessionID: "s1"})
	if err != nil {
		t.Fatalf("handler err = %v", err)
	}
	if _, ok := dec.(*rpc.PermissionDecisionApproveOnce); !ok {
		t.Fatalf("decision = %T, want ApproveOnce", dec)
	}
	// An allowed call emits NO permission gate and leaves no pending request.
	if c.perms.pending() != 0 {
		t.Fatalf("pending perms = %d, want 0", c.perms.pending())
	}
	select {
	case e := <-c.events:
		t.Fatalf("unexpected event emitted on allow: %+v", e)
	default:
	}
}

func TestPermissionHandlerDenyRejectsWithReason(t *testing.T) {
	c := newPolicyTestClient([]ctxforge.Hook{
		{ID: "d", Event: ctxforge.HookPreToolUse, Match: ctxforge.HookMatch{ToolKind: "shell", Pattern: "rm -rf *"}, Action: ctxforge.HookDeny, Reason: "destructive command blocked", Enabled: true},
	})
	h := c.permissionHandler()
	dec, err := h(sdk.PermissionRequestShell{FullCommandText: "rm -rf /tmp/x"}, sdk.PermissionInvocation{SessionID: "s1"})
	if err != nil {
		t.Fatalf("handler err = %v", err)
	}
	rej, ok := dec.(*rpc.PermissionDecisionReject)
	if !ok {
		t.Fatalf("decision = %T, want Reject", dec)
	}
	if rej.Feedback == nil || *rej.Feedback != "destructive command blocked" {
		t.Fatalf("reject feedback = %v, want the hook reason", rej.Feedback)
	}
	if c.perms.pending() != 0 {
		t.Fatalf("pending perms = %d, want 0 (no gate on deny)", c.perms.pending())
	}
}

func TestPermissionHandlerDenyMatchesWriteByPath(t *testing.T) {
	// A pattern hook governs a write by its target path (permCommand feeds the
	// write FileName), so an outside-workspace / sensitive-path deny fires.
	c := newPolicyTestClient([]ctxforge.Hook{
		{ID: "d", Event: ctxforge.HookPreToolUse, Match: ctxforge.HookMatch{ToolKind: "write", Pattern: "*/.ssh/*"}, Action: ctxforge.HookDeny, Reason: "no writes to .ssh", Enabled: true},
	})
	h := c.permissionHandler()
	dec, err := h(sdk.PermissionRequestWrite{FileName: "/home/u/.ssh/authorized_keys"}, sdk.PermissionInvocation{SessionID: "s1"})
	if err != nil {
		t.Fatalf("handler err = %v", err)
	}
	rej, ok := dec.(*rpc.PermissionDecisionReject)
	if !ok {
		t.Fatalf("decision = %T, want Reject", dec)
	}
	if rej.Feedback == nil || *rej.Feedback != "no writes to .ssh" {
		t.Fatalf("reject feedback = %v, want the hook reason", rej.Feedback)
	}
}

func TestPermissionHandlerAskFallsThroughToGate(t *testing.T) {
	// No matching hook for a write → the default ask falls through to the
	// interactive gate: an EvPermission is emitted and the handler blocks until
	// Respond resolves it.
	c := newPolicyTestClient([]ctxforge.Hook{
		{ID: "a", Event: ctxforge.HookPreToolUse, Match: ctxforge.HookMatch{ToolKind: "read"}, Action: ctxforge.HookAllow, Enabled: true},
	})
	h := c.permissionHandler()

	type result struct {
		dec rpc.PermissionDecision
		err error
	}
	done := make(chan result, 1)
	go func() {
		dec, err := h(sdk.PermissionRequestWrite{FileName: "main.go"}, sdk.PermissionInvocation{SessionID: "s1"})
		done <- result{dec, err}
	}()

	var ev Event
	select {
	case ev = <-c.events:
	case <-time.After(time.Second):
		t.Fatal("no EvPermission emitted; ask did not reach the gate")
	}
	if ev.Type != EvPermission || ev.Permission == nil {
		t.Fatalf("emitted event = %+v, want EvPermission with payload", ev)
	}
	if ev.Permission.Kind != "write" || ev.Permission.FileName != "main.go" {
		t.Fatalf("perm payload = %+v, want write/main.go", ev.Permission)
	}
	// Approve the gated request; the handler returns ApproveOnce.
	if err := c.Respond(ev.Permission.ID, true); err != nil {
		t.Fatalf("Respond: %v", err)
	}
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("handler err = %v", r.err)
		}
		if _, ok := r.dec.(*rpc.PermissionDecisionApproveOnce); !ok {
			t.Fatalf("decision = %T, want ApproveOnce", r.dec)
		}
	case <-time.After(time.Second):
		t.Fatal("handler did not return after Respond")
	}
}
