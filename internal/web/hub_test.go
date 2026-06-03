package web

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dotts-h/copilot-sdk/internal/config"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

func newTestHub() (*Hub, *copilot.MockClient) {
	mock := copilot.NewMockClient()
	hub := New(Options{
		Client: mock,
		Forge:  &ctxforge.Forge{},
		Config: &config.Config{DefaultModel: "gpt-5"},
		Meter:  telemetry.NewMeter(telemetry.DefaultPriceBook()),
		Logger: log.New(io.Discard, "", 0),
	})
	return hub, mock
}

func TestSessionsAreIsolated(t *testing.T) {
	hub, _ := newTestHub()
	a := hub.newSession("a")
	b := hub.newSession("b")
	if a == b {
		t.Fatal("distinct cookies must get distinct sessions")
	}
	a.state.AddUser("hi from A")
	if len(b.state.Committed()) != 0 {
		t.Fatalf("session B should not see A's transcript: %+v", b.state.Committed())
	}
	a.spec.Model = "model-a"
	b.spec.Model = "model-b"
	if a.spec.Model == b.spec.Model {
		t.Fatal("per-session spec should be independent")
	}
}

func TestRouteByCopilotID(t *testing.T) {
	hub, _ := newTestHub()
	a := hub.newSession("a")
	b := hub.newSession("b")
	hub.bind("cop-a", a)
	hub.bind("cop-b", b)
	if hub.route("cop-a") != a || hub.route("cop-b") != b {
		t.Fatal("events should route to the bound session")
	}
	// With more than one session, an unknown copilot id is unroutable (no guess).
	if hub.route("unknown") != nil {
		t.Fatal("ambiguous route should be nil")
	}
}

func TestRouteFallbackToLoneSession(t *testing.T) {
	hub, _ := newTestHub()
	only := hub.newSession("only")
	// A MockClient event carries no session id; with a single session it routes there.
	if hub.route("") != only {
		t.Fatal("empty session id should route to the lone session")
	}
}

func TestCookielessRequestJoinsLoneSession(t *testing.T) {
	s, _ := newTestServer() // hub with the single "test" session
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got string
	for _, c := range resp.Cookies() {
		if c.Name == cookieName {
			got = c.Value
		}
	}
	if got != "test" {
		t.Fatalf("cookieless request should join the lone session, cookie = %q", got)
	}
}
