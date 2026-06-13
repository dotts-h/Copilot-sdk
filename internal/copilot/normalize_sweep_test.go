// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package copilot

import (
	"testing"

	sdk "github.com/github/copilot-sdk/go"
)

// A tool that starts but never completes (a mid-tool SDK error with no matching
// ToolExecutionComplete) orphans its toolNames/toolMeta entry. Issue 0089:
// session teardown must reclaim it. DeleteSession and Close both delegate to
// sweepToolMaps, so we drive a start-without-complete then sweep and assert the
// orphan is gone — while a sibling session's in-flight tool survives.
func TestSweepToolMapsReclaimsOrphanedToolEntries(t *testing.T) {
	c := newTestSDKClient()

	// s1 starts two tools; t1 completes, t2 orphans (no complete event).
	h1 := c.makeHandler("s1")
	h1(sdk.SessionEvent{Data: &sdk.ToolExecutionStartData{ToolCallID: "t1", ToolName: "bash"}})
	h1(sdk.SessionEvent{Data: &sdk.ToolExecutionStartData{ToolCallID: "t2", ToolName: "read"}})
	h1(sdk.SessionEvent{Data: &sdk.ToolExecutionCompleteData{ToolCallID: "t1"}})

	// A second session keeps an in-flight tool that must outlive s1's teardown.
	h2 := c.makeHandler("s2")
	h2(sdk.SessionEvent{Data: &sdk.ToolExecutionStartData{ToolCallID: "t3", ToolName: "edit"}})

	c.mu.Lock()
	_, t1Pre := c.toolNames["t1"]
	_, t2Pre := c.toolNames["t2"]
	c.mu.Unlock()
	if t1Pre {
		t.Fatalf("t1 completed normally — should already be reclaimed before any sweep")
	}
	if !t2Pre {
		t.Fatalf("t2 orphan should still be present before the sweep")
	}

	c.mu.Lock()
	c.sweepToolMaps("s1")
	c.mu.Unlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.toolNames["t2"]; ok {
		t.Errorf("toolNames still holds s1 orphan t2 after sweep")
	}
	if _, ok := c.toolMeta["t2"]; ok {
		t.Errorf("toolMeta still holds s1 orphan t2 after sweep")
	}
	if _, ok := c.toolSession["t2"]; ok {
		t.Errorf("toolSession still holds s1 orphan t2 after sweep")
	}
	if _, ok := c.toolNames["t3"]; !ok {
		t.Errorf("sweep of s1 wrongly reclaimed s2's in-flight tool t3")
	}
	if _, ok := c.toolSession["t3"]; !ok {
		t.Errorf("sweep of s1 wrongly dropped s2's toolSession entry for t3")
	}
}
