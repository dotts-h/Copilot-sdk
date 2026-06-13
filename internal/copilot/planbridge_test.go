// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package copilot

import (
	"testing"
	"time"

	sdk "github.com/github/copilot-sdk/go"
)

func TestPlanBridgeBeginResolve(t *testing.T) {
	b := newPlanBridge()
	id, ch := b.begin()
	if id == "" || b.pending() != 1 {
		t.Fatalf("begin: id=%q pending=%d", id, b.pending())
	}
	var got planDecision
	done := make(chan struct{})
	go func() { got = <-ch; close(done) }()
	if !b.resolve(id, planDecision{Approved: true, Action: "build"}) {
		t.Fatal("resolve should succeed for a known id")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("waiter never received the decision")
	}
	if !got.Approved || got.Action != "build" {
		t.Fatalf("decision = %+v", got)
	}
	if b.pending() != 0 {
		t.Fatalf("pending should be 0, got %d", b.pending())
	}
}

func TestPlanBridgeResolveUnknown(t *testing.T) {
	if newPlanBridge().resolve("nope", planDecision{}) {
		t.Fatal("resolving an unknown id should return false")
	}
}

// TestExitPlanModeHandlerApprove exercises the synchronous exit-plan-mode
// callback: it emits an EvPlanReview, blocks, and is resolved (approved) via
// RespondPlan.
func TestExitPlanModeHandlerApprove(t *testing.T) {
	c := newTestSDKClient()
	h := c.exitPlanModeHandler()
	resultCh := make(chan sdk.ExitPlanModeResult, 1)
	go func() {
		resp, _ := h(sdk.ExitPlanModeRequest{
			Summary: "Do X then Y", PlanContent: "1. X\n2. Y",
			Actions: []string{"proceed", "edit"}, RecommendedAction: "proceed",
		}, sdk.ExitPlanModeInvocation{})
		resultCh <- resp
	}()

	var ev Event
	select {
	case ev = <-c.events:
	case <-time.After(time.Second):
		t.Fatal("no EvPlanReview emitted")
	}
	if ev.Type != EvPlanReview || ev.Plan == nil {
		t.Fatalf("expected EvPlanReview with Plan, got %+v", ev)
	}
	if ev.Plan.Summary != "Do X then Y" || ev.Plan.Recommended != "proceed" || len(ev.Plan.Actions) != 2 {
		t.Fatalf("plan request not normalized: %+v", ev.Plan)
	}
	if err := c.RespondPlan(ev.Plan.ID, true, "proceed", ""); err != nil {
		t.Fatalf("RespondPlan: %v", err)
	}
	select {
	case r := <-resultCh:
		if !r.Approved || r.SelectedAction != "proceed" {
			t.Fatalf("result = %+v, want approved+proceed", r)
		}
	case <-time.After(time.Second):
		t.Fatal("handler never returned after RespondPlan")
	}
}

func TestExitPlanModeHandlerDeclineWithFeedback(t *testing.T) {
	c := newTestSDKClient()
	h := c.exitPlanModeHandler()
	resultCh := make(chan sdk.ExitPlanModeResult, 1)
	go func() {
		resp, _ := h(sdk.ExitPlanModeRequest{Summary: "Plan"}, sdk.ExitPlanModeInvocation{})
		resultCh <- resp
	}()
	ev := <-c.events
	if err := c.RespondPlan(ev.Plan.ID, false, "", "add tests first"); err != nil {
		t.Fatal(err)
	}
	r := <-resultCh
	if r.Approved || r.Feedback != "add tests first" {
		t.Fatalf("result = %+v, want declined+feedback", r)
	}
}

func TestRespondPlanUnknownID(t *testing.T) {
	c := newTestSDKClient()
	if err := c.RespondPlan("plan-999", true, "x", ""); err == nil {
		t.Fatal("RespondPlan on unknown id should error")
	}
}

func TestHandlerMapsPlanChanged(t *testing.T) {
	c := newTestSDKClient()
	h := c.makeHandler("")
	h(sdk.SessionEvent{Data: &sdk.SessionPlanChangedData{Operation: sdk.PlanChangedOperationCreate}})
	ev := <-c.events
	if ev.Type != EvPlanChanged || ev.Text != "plan created" {
		t.Fatalf("plan-changed event = %+v", ev)
	}
}
