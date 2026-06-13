// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package copilot

import (
	"strings"

	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	sdk "github.com/github/copilot-sdk/go"
	"github.com/github/copilot-sdk/go/rpc"
)

// This file holds the sync↔async bridge adapters: the SDK invokes these
// permission/input/plan/elicitation callbacks synchronously and expects a
// decision, while the web UI answers asynchronously over HTTP. Each adapter
// emits a normalized Event, then blocks on its bridge channel until the matching
// Respond* call resolves it (or the client shuts down via c.done). See the
// "Interactive permissions (sync ↔ async bridge)" section in ARCHITECTURE.md.

// permissionHandler bridges the SDK's synchronous permission callback to the
// async web UI. Before the interactive gate it consults the session's compiled
// governance policy (ADR-0029/0030): a PreToolUse decision of allow auto-approves
// the call (no EvPermission emitted), deny rejects it with the hook's reason fed
// back to the agent, and ask falls through to the existing behavior — emit an
// EvPermission and block until Respond() (or shutdown).
//
// This is the ONLY permission handler (the SDK's blanket ApproveAll is never
// wired): the mandatory dangerous ruleset (G2) must run even under
// AutoApproveTools. A mandatory deny still rejects and a mandatory ask still
// gates regardless of auto-approve; only the non-mandatory remainder (an ordinary
// ask, or a no-match default-ask) is blanket-approved when the session set
// AutoApproveTools — so config alone cannot bypass the dangerous policy. — ADR-0030.
func (c *SDKClient) permissionHandler() sdk.PermissionHandlerFunc {
	return func(req sdk.PermissionRequest, inv sdk.PermissionInvocation) (rpc.PermissionDecision, error) {
		c.mu.Lock()
		pol := c.policies[inv.SessionID]
		c.mu.Unlock()
		kind, cmd := string(req.Kind()), permCommand(req)
		d := ctxforge.Evaluate(pol.hooks, ctxforge.HookPreToolUse, kind, cmd, pol.workspace, pol.mode)
		switch d.Action {
		case ctxforge.HookAllow:
			// A user hook auto-approved this call — surface "auto-approved by X" in
			// the timeline (ADR-0031). The built-in safe-read default (builtin-* id)
			// stays silent so the expected baseline doesn't flood the transcript.
			if d.HookID != "" && !isBuiltinHook(d.HookID) {
				c.emitDecision(inv.SessionID, HookAllow, d, req)
			}
			return &rpc.PermissionDecisionApproveOnce{}, nil
		case ctxforge.HookDeny:
			// A hard-deny never runs the tool (no tool card), so the decision event
			// is the ONLY way the user learns why the call was blocked. — ADR-0031.
			c.emitDecision(inv.SessionID, HookDeny, d, req)
			fb := d.Reason
			if fb == "" {
				fb = "Denied by hook policy"
			}
			return &rpc.PermissionDecisionReject{Feedback: &fb}, nil
		}
		// ask: a non-mandatory decision under the mode's auto-approve baseline is
		// approved; a mandatory ask (dangerous-action gate) is never auto-approved.
		// The baseline is mode-bound — autopilot on, interactive off, else the
		// session's AutoApproveTools config (ADR-0031). The blanket auto-approve of
		// the non-mandatory remainder is intentionally silent (it is the expected
		// baseline, and the active mode already shows in the statusline).
		if ctxforge.EffectiveAutoApprove(pol.mode, pol.autoApprove) && !d.Mandatory {
			return &rpc.PermissionDecisionApproveOnce{}, nil
		}
		// Otherwise fall through to the interactive gate, carrying the hook's reason
		// so the human sees WHY they are being asked (ADR-0031).
		id, ch := c.perms.begin()
		file, intention, diff := permWriteFields(req)
		c.emit(Event{Type: EvPermission, SessionID: inv.SessionID, Permission: &PermissionRequest{
			ID: id, Kind: kind, Detail: describePermission(req),
			FileName: file, Intention: intention, Diff: diff, Reason: d.Reason,
		}})
		select {
		case approve := <-ch:
			if approve {
				return &rpc.PermissionDecisionApproveOnce{}, nil
			}
			fb := "Rejected by user"
			return &rpc.PermissionDecisionReject{Feedback: &fb}, nil
		case <-c.done:
			return &rpc.PermissionDecisionUserNotAvailable{}, nil
		}
	}
}

// HookAllow / HookDeny re-export the ctxforge action constants so handlers.go
// reads without an extra import qualifier at the call sites that classify a
// surfaced decision. — ADR-0031.
const (
	HookAllow = ctxforge.HookAllow
	HookDeny  = ctxforge.HookDeny
)

// isBuiltinHook reports whether a hook id names a shipped built-in (the
// safe-read defaults and the mandatory dangerous ruleset all use the builtin-*
// id prefix). A user hook never carries it (Validate's slug rule does not
// reserve the prefix, but the seeded set owns it by convention). — ADR-0031.
func isBuiltinHook(id string) bool { return strings.HasPrefix(id, "builtin-") }

// emitDecision normalizes a non-gated governance decision into an EvToolDecision
// event so the timeline can explain why a call was auto-approved or denied
// without re-introducing a gate (ADR-0031). The reason already lives in the pure
// ctxforge.Decision; no SDK type crosses into ctxforge.
func (c *SDKClient) emitDecision(sessionID, kind string, d ctxforge.Decision, req sdk.PermissionRequest) {
	c.emit(Event{Type: EvToolDecision, SessionID: sessionID, Decision: &ToolDecision{
		Kind: kind, HookID: d.HookID, Reason: d.Reason, Detail: describePermission(req),
	}})
}

// permCommand returns the string a hook Pattern matches against for a request:
// a shell request's full command, or a write request's target file name (so a
// pattern can govern writes by path, e.g. outside-workspace denies). Empty for
// kinds with no meaningful string target (read/mcp in this SDK), where only a
// kind match applies.
func permCommand(req sdk.PermissionRequest) string {
	switch r := req.(type) {
	case sdk.PermissionRequestShell:
		return r.FullCommandText
	case sdk.PermissionRequestWrite:
		return r.FileName
	}
	return ""
}

// userInputHandler bridges the SDK's synchronous ask_user callback to the async
// web UI, mirroring permissionHandler: it emits an EvUserInput and blocks until
// RespondInput() (or shutdown). WasFreeform is derived by checking whether the
// answer matches one of the offered choices.
func (c *SDKClient) userInputHandler() sdk.UserInputHandler {
	return func(req sdk.UserInputRequest, inv sdk.UserInputInvocation) (sdk.UserInputResponse, error) {
		id, ch := c.inputs.begin()
		allow := req.AllowFreeform != nil && *req.AllowFreeform
		c.emit(Event{Type: EvUserInput, SessionID: inv.SessionID, Input: &InputRequest{
			ID: id, Question: req.Question, Choices: req.Choices, AllowFreeform: allow,
		}})
		select {
		case answer := <-ch:
			wasFreeform := true
			for _, choice := range req.Choices {
				if choice == answer {
					wasFreeform = false
					break
				}
			}
			return sdk.UserInputResponse{Answer: answer, WasFreeform: wasFreeform}, nil
		case <-c.done:
			return sdk.UserInputResponse{}, ErrClosed
		}
	}
}

// exitPlanModeHandler bridges the SDK's synchronous exit-plan-mode callback to
// the async web UI, mirroring userInputHandler: it emits an EvPlanReview and
// blocks until RespondPlan() (or shutdown). An approval proceeds with the chosen
// (or recommended) action; a decline returns the user's feedback.
func (c *SDKClient) exitPlanModeHandler() sdk.ExitPlanModeRequestHandler {
	return func(req sdk.ExitPlanModeRequest, inv sdk.ExitPlanModeInvocation) (sdk.ExitPlanModeResult, error) {
		id, ch := c.plans.begin()
		c.emit(Event{Type: EvPlanReview, SessionID: inv.SessionID, Plan: &PlanRequest{
			ID: id, Summary: req.Summary, Plan: req.PlanContent,
			Actions: req.Actions, Recommended: req.RecommendedAction,
		}})
		select {
		case d := <-ch:
			return sdk.ExitPlanModeResult{
				Approved: d.Approved, SelectedAction: d.Action, Feedback: d.Feedback,
			}, nil
		case <-c.done:
			return sdk.ExitPlanModeResult{}, ErrClosed
		}
	}
}

// elicitationHandler bridges the SDK's synchronous OnElicitationRequest callback
// to the async web UI, mirroring exitPlanModeHandler: it normalizes the schema
// into displayable fields, emits an EvElicitation, and blocks until
// RespondElicit() (or shutdown). On shutdown it cancels the request so the MCP
// server is not left waiting.
func (c *SDKClient) elicitationHandler() sdk.ElicitationHandler {
	return func(ec sdk.ElicitationContext) (sdk.ElicitationResult, error) {
		id, ch := c.elicits.begin()
		c.emit(Event{Type: EvElicitation, SessionID: ec.SessionID, Elicit: &ElicitRequest{
			ID: id, Message: ec.Message, Source: derefStr(ec.ElicitationSource),
			Fields: normalizeElicitFields(ec.RequestedSchema),
		}})
		select {
		case d := <-ch:
			return sdk.ElicitationResult{
				Action:  sdk.ElicitationAction(d.Action),
				Content: d.Content,
			}, nil
		case <-c.done:
			return sdk.ElicitationResult{Action: sdk.ElicitationActionCancel}, nil
		}
	}
}
