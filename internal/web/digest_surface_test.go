// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package web

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// seedDigestData wires a spend ledger + run store with in-window activity so the
// Telemetry digest card has something to summarize.
func seedDigestData(s *Server) {
	rs := withRunStore(s)
	s.spend, _ = telemetry.LoadSpendStore("") // ephemeral ledger
	now := time.Now()
	for _, id := range []string{"r1", "r2", "r3"} {
		_ = rs.Append(telemetry.RunRecord{
			ID: id, WorkflowID: "wf", Name: "Nightly", StartedAt: now, Outcome: "finished",
			Lanes: []telemetry.RunLane{{Index: 0, Credits: 1}},
		})
	}
	_ = s.spend.Append(telemetry.SpendRecord{At: now, USD: 2.50, WorkflowID: "wf", AgentID: "ag1"})
}

// The Telemetry page renders the Period digest card with totals and a download link
// when the window has activity.
func TestTelemetryPageRendersDigest(t *testing.T) {
	s, _ := newTestServer()
	seedDigestData(s)

	html := s.telemetryPartial(defaultSpendWindow)
	if !strings.Contains(html, "Period digest") {
		t.Fatal("the Telemetry page should render the Period digest card when the window has activity")
	}
	if !strings.Contains(html, "/telemetry/digest.md?window=") {
		t.Error("the digest card should carry a windowed export link")
	}
}

// The digest export streams a markdown artifact over the requested window.
func TestDigestExportStreamsMarkdown(t *testing.T) {
	s, _ := newTestServer()
	seedDigestData(s)

	req := httptest.NewRequest("GET", "/telemetry/digest.md?window=30", nil)
	rec := httptest.NewRecorder()
	s.handleDigestExport(rec, req)

	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/markdown") {
		t.Errorf("digest export should be markdown, got %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "my-orchestra-digest.md") {
		t.Errorf("digest export should be an attachment, got %q", cd)
	}
	if body := rec.Body.String(); !strings.Contains(body, "# Spend digest") {
		t.Errorf("digest body should be the rendered artifact, got: %s", body)
	}
}
