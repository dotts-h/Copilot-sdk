package copilot

import (
	"testing"
	"time"

	sdk "github.com/github/copilot-sdk/go"
)

func TestElicitBridgeBeginResolve(t *testing.T) {
	b := newElicitBridge()
	id, ch := b.begin()
	if id == "" || b.pending() != 1 {
		t.Fatalf("begin: id=%q pending=%d", id, b.pending())
	}
	var got elicitDecision
	done := make(chan struct{})
	go func() { got = <-ch; close(done) }()
	if !b.resolve(id, elicitDecision{Action: "accept", Content: map[string]any{"name": "Mona"}}) {
		t.Fatal("resolve should succeed for a known id")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("waiter never received the decision")
	}
	if got.Action != "accept" || got.Content["name"] != "Mona" {
		t.Fatalf("decision = %+v", got)
	}
	if b.pending() != 0 {
		t.Fatalf("pending should be 0, got %d", b.pending())
	}
}

func TestElicitBridgeResolveUnknown(t *testing.T) {
	if newElicitBridge().resolve("nope", elicitDecision{}) {
		t.Fatal("resolving an unknown id should return false")
	}
}

// TestElicitationHandlerAccept exercises the synchronous elicitation callback:
// it normalizes the schema, emits an EvElicitation, blocks, and is resolved
// (accepted with content) via RespondElicit.
func TestElicitationHandlerAccept(t *testing.T) {
	c := newTestSDKClient()
	h := c.elicitationHandler()
	source := "github-mcp"
	resultCh := make(chan sdk.ElicitationResult, 1)
	go func() {
		res, _ := h(sdk.ElicitationContext{
			Message:           "Provide your details",
			ElicitationSource: &source,
			RequestedSchema: &sdk.ElicitationSchema{
				Properties: map[string]any{
					"name": map[string]any{"type": "string", "title": "Your name", "default": "Mona"},
					"team": map[string]any{"type": "string", "enum": []any{"red", "blue"}},
				},
				Required: []string{"name"},
			},
		})
		resultCh <- res
	}()

	var ev Event
	select {
	case ev = <-c.events:
	case <-time.After(time.Second):
		t.Fatal("no EvElicitation emitted")
	}
	if ev.Type != EvElicitation || ev.Elicit == nil {
		t.Fatalf("expected EvElicitation with Elicit, got %+v", ev)
	}
	if ev.Elicit.Message != "Provide your details" || ev.Elicit.Source != "github-mcp" {
		t.Fatalf("elicit request not normalized: %+v", ev.Elicit)
	}
	if len(ev.Elicit.Fields) != 2 {
		t.Fatalf("want 2 fields, got %+v", ev.Elicit.Fields)
	}
	// Fields are sorted by name: "name" then "team".
	name, team := ev.Elicit.Fields[0], ev.Elicit.Fields[1]
	if name.Name != "name" || name.Label != "Your name" || name.Default != "Mona" || !name.Required {
		t.Fatalf("name field = %+v", name)
	}
	if team.Name != "team" || team.Required || len(team.Enum) != 2 || team.Enum[0] != "red" {
		t.Fatalf("team field = %+v", team)
	}

	if err := c.RespondElicit(ev.Elicit.ID, "accept", map[string]any{"name": "Mona", "team": "blue"}); err != nil {
		t.Fatalf("RespondElicit: %v", err)
	}
	select {
	case r := <-resultCh:
		if r.Action != sdk.ElicitationActionAccept || r.Content["team"] != "blue" {
			t.Fatalf("result = %+v, want accept+content", r)
		}
	case <-time.After(time.Second):
		t.Fatal("handler never returned after RespondElicit")
	}
}

func TestElicitationHandlerDecline(t *testing.T) {
	c := newTestSDKClient()
	h := c.elicitationHandler()
	resultCh := make(chan sdk.ElicitationResult, 1)
	go func() {
		res, _ := h(sdk.ElicitationContext{Message: "Details?"})
		resultCh <- res
	}()
	ev := <-c.events
	if err := c.RespondElicit(ev.Elicit.ID, "decline", nil); err != nil {
		t.Fatal(err)
	}
	r := <-resultCh
	if r.Action != sdk.ElicitationActionDecline {
		t.Fatalf("result = %+v, want decline", r)
	}
}

func TestRespondElicitUnknownID(t *testing.T) {
	c := newTestSDKClient()
	if err := c.RespondElicit("elicit-999", "accept", nil); err == nil {
		t.Fatal("RespondElicit on unknown id should error")
	}
}

func TestNormalizeElicitFieldsNilSchema(t *testing.T) {
	if f := normalizeElicitFields(nil); f != nil {
		t.Fatalf("nil schema should yield no fields, got %+v", f)
	}
}

func TestNormalizeElicitFieldDefaults(t *testing.T) {
	// A boolean field with a bool default and a numeric field with a float
	// default should stringify sensibly; an unknown property degrades to string.
	f := normalizeElicitField("flag", map[string]any{"type": "boolean", "default": true})
	if f.Type != "boolean" || f.Default != "true" {
		t.Fatalf("boolean field = %+v", f)
	}
	n := normalizeElicitField("count", map[string]any{"type": "integer", "default": float64(3)})
	if n.Type != "integer" || n.Default != "3" {
		t.Fatalf("integer field = %+v", n)
	}
	bad := normalizeElicitField("x", "not-an-object")
	if bad.Type != "string" || bad.Label != "x" {
		t.Fatalf("malformed property should degrade to string: %+v", bad)
	}
}
