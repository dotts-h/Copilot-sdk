package copilot

import (
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
// governance policy (ADR-0029): a PreToolUse decision of allow auto-approves the
// call (no EvPermission emitted), deny rejects it with the hook's reason fed back
// to the agent, and ask falls through to the existing behavior — emit an
// EvPermission and block until Respond() (or shutdown). This generalizes the
// flat AutoApproveTools from an all-or-nothing switch to a per-tool ruleset.
func (c *SDKClient) permissionHandler() sdk.PermissionHandlerFunc {
	return func(req sdk.PermissionRequest, inv sdk.PermissionInvocation) (rpc.PermissionDecision, error) {
		c.mu.Lock()
		hooks := c.hooks[inv.SessionID]
		c.mu.Unlock()
		switch d := ctxforge.Evaluate(hooks, ctxforge.HookPreToolUse, string(req.Kind()), permCommand(req)); d.Action {
		case ctxforge.HookAllow:
			return &rpc.PermissionDecisionApproveOnce{}, nil
		case ctxforge.HookDeny:
			fb := d.Reason
			if fb == "" {
				fb = "Denied by hook policy"
			}
			return &rpc.PermissionDecisionReject{Feedback: &fb}, nil
		}
		// ask (or no matching hook): fall through to the interactive gate.
		id, ch := c.perms.begin()
		file, intention, diff := permWriteFields(req)
		c.emit(Event{Type: EvPermission, SessionID: inv.SessionID, Permission: &PermissionRequest{
			ID: id, Kind: string(req.Kind()), Detail: describePermission(req),
			FileName: file, Intention: intention, Diff: diff,
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
