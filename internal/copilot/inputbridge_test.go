package copilot

import (
	"testing"
	"time"

	sdk "github.com/github/copilot-sdk/go"
)

func TestInputBridgeBeginResolve(t *testing.T) {
	b := newInputBridge()
	id, ch := b.begin()
	if id == "" {
		t.Fatal("empty id")
	}
	if b.pending() != 1 {
		t.Fatalf("pending = %d, want 1", b.pending())
	}

	var got string
	done := make(chan struct{})
	go func() { got = <-ch; close(done) }()

	if !b.resolve(id, "yes") {
		t.Fatal("resolve should succeed for a known id")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("waiter never received the answer")
	}
	if got != "yes" {
		t.Fatalf("answer = %q, want yes", got)
	}
	if b.pending() != 0 {
		t.Fatalf("pending should be 0 after resolve, got %d", b.pending())
	}
}

func TestInputBridgeResolveUnknown(t *testing.T) {
	b := newInputBridge()
	if b.resolve("nope", "x") {
		t.Fatal("resolving an unknown id should return false")
	}
}

// TestUserInputHandlerEmitsAndResolves exercises the synchronous ask_user
// callback end to end: it emits an EvUserInput, blocks, and is resolved by
// RespondInput. The answer matching a choice is reported as non-freeform.
func TestUserInputHandlerEmitsAndResolves(t *testing.T) {
	c := newTestSDKClient()
	h := c.userInputHandler()
	allow := true

	type res struct {
		resp sdk.UserInputResponse
		err  error
	}
	resultCh := make(chan res, 1)
	go func() {
		resp, err := h(sdk.UserInputRequest{
			Question: "Pick one", Choices: []string{"a", "b"}, AllowFreeform: &allow,
		}, sdk.UserInputInvocation{})
		resultCh <- res{resp, err}
	}()

	var ev Event
	select {
	case ev = <-c.events:
	case <-time.After(time.Second):
		t.Fatal("no EvUserInput emitted")
	}
	if ev.Type != EvUserInput || ev.Input == nil {
		t.Fatalf("expected EvUserInput with Input, got %+v", ev)
	}
	if ev.Input.Question != "Pick one" || len(ev.Input.Choices) != 2 || !ev.Input.AllowFreeform {
		t.Fatalf("input request not normalized: %+v", ev.Input)
	}

	if err := c.RespondInput(ev.Input.ID, "a"); err != nil {
		t.Fatalf("RespondInput: %v", err)
	}
	select {
	case r := <-resultCh:
		if r.err != nil {
			t.Fatalf("handler err: %v", r.err)
		}
		if r.resp.Answer != "a" || r.resp.WasFreeform {
			t.Fatalf("response = %+v, want answer=a, freeform=false", r.resp)
		}
	case <-time.After(time.Second):
		t.Fatal("handler never returned after RespondInput")
	}
}

func TestUserInputHandlerFreeformAnswer(t *testing.T) {
	c := newTestSDKClient()
	h := c.userInputHandler()
	resultCh := make(chan sdk.UserInputResponse, 1)
	go func() {
		resp, _ := h(sdk.UserInputRequest{Question: "Name?", Choices: []string{"a"}}, sdk.UserInputInvocation{})
		resultCh <- resp
	}()
	ev := <-c.events
	if err := c.RespondInput(ev.Input.ID, "custom"); err != nil {
		t.Fatal(err)
	}
	resp := <-resultCh
	if resp.Answer != "custom" || !resp.WasFreeform {
		t.Fatalf("response = %+v, want answer=custom, freeform=true", resp)
	}
}

func TestRespondInputUnknownID(t *testing.T) {
	c := newTestSDKClient()
	if err := c.RespondInput("ask-999", "x"); err == nil {
		t.Fatal("RespondInput on unknown id should error")
	}
}
