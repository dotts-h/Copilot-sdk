// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package web

import (
	"strings"
	"testing"

	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// usageEvent folds one assistant turn's token usage in through the real EvUsage
// reducer, exercising the same path the runtime drives (account + session meter).
func usageEvent(model string, input int64) copilot.Event {
	return copilot.Event{Type: copilot.EvUsage, Usage: copilot.UsageData{Model: model, InputTokens: input}}
}

// TestStatlineScopesTotalsToThisSession guards item 3.2 / TECH_DEBT #2: the
// statusline credits/tokens reflect *this* conversation, not the process-global
// meter that aggregates every cookie-keyed session.
func TestStatlineScopesTotalsToThisSession(t *testing.T) {
	s, _ := newTestServer()
	s.spec.Model = "gpt-5"

	// Another conversation's spend folds into the shared account-wide meter only
	// (e.g. a second browser session). It must not leak into this statusline.
	s.meter.Record(telemetry.Usage{Model: "gpt-5", InputTokens: 800_000}) // 100 cr account-wide

	got := renderStatline(s)
	if !strings.Contains(got, "0.00 cr") {
		t.Errorf("a session with no turns of its own must show zero credits: %q", got)
	}
	if strings.Contains(got, "100.00 cr") {
		t.Errorf("the account-wide total must not leak into the per-session statusline: %q", got)
	}
	if !strings.Contains(got, "↑0 ") {
		t.Errorf("a session with no turns must show zero input tokens: %q", got)
	}

	// This conversation records a turn through the real reducer.
	_ = s.handleEvent(usageEvent("gpt-5", 80_000)) // 10 cr this session

	got = renderStatline(s)
	if !strings.Contains(got, "10.00 cr") {
		t.Errorf("the statusline must reflect this session's own spend (10 cr): %q", got)
	}
	if !strings.Contains(got, "↑80.0k") {
		t.Errorf("the statusline must reflect this session's input tokens: %q", got)
	}
	if strings.Contains(got, "110.00 cr") {
		t.Errorf("the statusline must not fold in the account-wide total: %q", got)
	}
}
