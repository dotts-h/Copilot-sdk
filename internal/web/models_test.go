package web

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dotts-h/copilot-sdk/internal/copilot"
)

var errFake = errors.New("list failed")

func TestModelsPageListsAndMarksCurrent(t *testing.T) {
	s, mock := newTestServer()
	mock.Models = []copilot.ModelInfo{
		{ID: "gpt-5", Name: "GPT-5"},
		{ID: "claude-sonnet-4-6", Name: "Claude Sonnet 4.6"},
	}
	s.spec.Model = "claude-sonnet-4-6"

	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := get(t, srv, "/page/models")
	if !strings.Contains(body, "GPT-5") || !strings.Contains(body, "Claude Sonnet 4.6") {
		t.Errorf("models page should list every model: %q", body)
	}
	// The current model row is marked active.
	if !strings.Contains(body, `row on`) {
		t.Errorf("current model should be marked active: %q", body)
	}
	// Non-current models offer a use action.
	if !strings.Contains(body, `/models/gpt-5/select`) {
		t.Errorf("each model should be selectable: %q", body)
	}
}

func TestModelSelectSwitchesAndRestarts(t *testing.T) {
	s, mock := newTestServer()
	mock.Models = []copilot.ModelInfo{{ID: "gpt-5", Name: "GPT-5"}, {ID: "o4", Name: "o4"}}
	s.spec.Model = "gpt-5"
	// Pretend a session is already open so we can confirm it restarts.
	s.sessionID = "live-session"

	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/models/o4/select", "application/x-www-form-urlencoded", nil)
	if err != nil {
		t.Fatal(err)
	}
	body := readBody(t, resp)
	if s.spec.Model != "o4" {
		t.Errorf("select should switch the spec model, got %q", s.spec.Model)
	}
	if s.sessionID != "" {
		t.Errorf("select should restart the session, got %q", s.sessionID)
	}
	if s.config.DefaultModel != "o4" {
		t.Errorf("select should persist the default model, got %q", s.config.DefaultModel)
	}
	// Response re-renders the models page with the new current marked.
	if !strings.Contains(body, "o4") || !strings.Contains(body, "row on") {
		t.Errorf("select response should re-render the models page: %q", body)
	}
}

func TestModelsPageHandlesListError(t *testing.T) {
	s, mock := newTestServer()
	mock.ListModelsErr = errFake
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	body := get(t, srv, "/page/models")
	if !strings.Contains(body, "unavailable") {
		t.Errorf("models page should degrade gracefully on list error: %q", body)
	}
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}
