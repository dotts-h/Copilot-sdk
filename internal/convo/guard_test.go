// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package convo

// guard_test.go — additional guard tests for items 8–10.

import "testing"

// ── Item 8: AddDecision commits pending text ─────────────────────────────────

// TestAddDecisionCommitsPendingText guards that AddDecision flushes the
// in-flight message buffer before appending its own turn, so the committed
// transcript is [RoleAgent("partial"), RoleDecision] in that order.
func TestAddDecisionCommitsPendingText(t *testing.T) {
	var c State
	c.AppendDelta("partial")
	c.AddDecision(DecisionView{Kind: "allow", HookID: "h1", Detail: "bash ls"})

	committed := c.Committed()
	if len(committed) != 2 {
		t.Fatalf("want 2 committed turns, got %d: %+v", len(committed), committed)
	}
	if committed[0].Role != RoleAgent || committed[0].Text != "partial" {
		t.Fatalf("first turn should be agent 'partial', got %+v", committed[0])
	}
	if committed[1].Role != RoleDecision || committed[1].Decision == nil {
		t.Fatalf("second turn should be decision, got %+v", committed[1])
	}
	if committed[1].Decision.Kind != "allow" || committed[1].Decision.HookID != "h1" {
		t.Fatalf("decision payload wrong: %+v", committed[1].Decision)
	}
	// Message buffer must be empty after the flush.
	if _, txt := c.Pending(); txt != "" {
		t.Fatalf("message buffer should be empty after AddDecision, got %q", txt)
	}
}

// ── Item 9: AddDecision commits pending reasoning ────────────────────────────

// TestAddDecisionCommitsPendingReasoning guards that AddDecision flushes an
// in-flight reasoning buffer before appending its turn, so reasoning precedes
// the decision in the transcript.
func TestAddDecisionCommitsPendingReasoning(t *testing.T) {
	var c State
	c.AppendReasoning("thinking")
	c.AddDecision(DecisionView{Kind: "deny", Reason: "blocked by policy"})

	committed := c.Committed()
	if len(committed) != 2 {
		t.Fatalf("want 2 committed turns, got %d: %+v", len(committed), committed)
	}
	if committed[0].Role != RoleReasoning || committed[0].Text != "thinking" {
		t.Fatalf("first turn should be reasoning 'thinking', got %+v", committed[0])
	}
	if committed[1].Role != RoleDecision || committed[1].Decision == nil {
		t.Fatalf("second turn should be decision, got %+v", committed[1])
	}
	if committed[1].Decision.Kind != "deny" {
		t.Fatalf("decision kind wrong: %+v", committed[1].Decision)
	}
}

// ── Item 10: reasoning → tool interleave ordering ───────────────────────────
//
// NOTE: TestReasoningIsSeparate and TestToolInterleavesWithText already cover
// reasoning-then-message and text-before-tool ordering respectively.
// This test specifically guards the reasoning→tool→message interleave sequence
// — a distinct path not covered by any existing test.

// TestReasoningThenToolInterleaveOrder asserts that AppendReasoning → ToolStart
// → Finish produces [RoleReasoning, RoleTool, RoleAgent] in that order.
func TestReasoningThenToolInterleaveOrder(t *testing.T) {
	var c State
	c.AppendReasoning("let me plan") // reasoning in-flight
	c.ToolStart("t1", "bash", "ls")  // commits reasoning, adds tool turn
	c.AppendDelta("done")            // text after the tool
	c.Finish("")

	committed := c.Committed()
	if len(committed) != 3 {
		t.Fatalf("want 3 turns [reasoning, tool, agent], got %d: %+v", len(committed), committed)
	}
	if committed[0].Role != RoleReasoning {
		t.Fatalf("turn 0 should be reasoning, got %v", committed[0].Role)
	}
	if committed[1].Role != RoleTool {
		t.Fatalf("turn 1 should be tool, got %v", committed[1].Role)
	}
	if committed[2].Role != RoleAgent || committed[2].Text != "done" {
		t.Fatalf("turn 2 should be agent 'done', got %+v", committed[2])
	}
}
