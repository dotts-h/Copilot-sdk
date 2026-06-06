package copilot

import (
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
// async web UI: it emits an EvPermission and blocks until Respond() (or shutdown).
func (c *SDKClient) permissionHandler() sdk.PermissionHandlerFunc {
	return func(req sdk.PermissionRequest, inv sdk.PermissionInvocation) (rpc.PermissionDecision, error) {
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
