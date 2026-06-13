// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package copilot

import (
	"context"
	"errors"
	"testing"
)

// MockClient is the offline/demo seam and the double every web/convo test drives,
// so its recording and error-injection behavior is worth guarding directly.

func TestMockRecordsSendAndAccessors(t *testing.T) {
	m := NewMockClient()
	defer m.Close()
	ctx := context.Background()

	if _, err := m.CreateSession(ctx, SessionSpec{}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := m.Send(ctx, "s", "hello", nil, "plan"); err != nil {
		t.Fatalf("send: %v", err)
	}
	if err := m.Send(ctx, "s", "again", []string{"a.txt"}, ""); err != nil {
		t.Fatalf("send2: %v", err)
	}
	if got := m.SentCount(); got != 2 {
		t.Fatalf("SentCount = %d, want 2", got)
	}
	if got := m.SentAt(0); got != "hello" {
		t.Fatalf("SentAt(0) = %q", got)
	}
	if got := m.SentModeAt(0); got != "plan" {
		t.Fatalf("SentModeAt(0) = %q, want plan", got)
	}
	if got := m.SentModeAt(1); got != "" {
		t.Fatalf("SentModeAt(1) = %q, want empty", got)
	}
	if len(m.LastAttach) != 1 || m.LastAttach[0] != "a.txt" {
		t.Fatalf("LastAttach = %v", m.LastAttach)
	}
}

func TestMockErrorInjection(t *testing.T) {
	sentinel := errors.New("boom")
	t.Run("create", func(t *testing.T) {
		m := NewMockClient()
		m.CreateErr = sentinel
		if _, err := m.CreateSession(context.Background(), SessionSpec{}); !errors.Is(err, sentinel) {
			t.Fatalf("CreateSession err = %v", err)
		}
	})
	t.Run("send", func(t *testing.T) {
		m := NewMockClient()
		m.SendErr = sentinel
		if err := m.Send(context.Background(), "s", "p", nil, ""); !errors.Is(err, sentinel) {
			t.Fatalf("Send err = %v", err)
		}
		if m.SentCount() != 0 {
			t.Fatal("a failed send must not be recorded")
		}
	})
}

func TestMockRecordsDecisions(t *testing.T) {
	m := NewMockClient()
	if err := m.Respond("p1", true); err != nil {
		t.Fatal(err)
	}
	if err := m.RespondInput("i1", "answer"); err != nil {
		t.Fatal(err)
	}
	if err := m.RespondPlan("pl1", true, "approve", "lgtm"); err != nil {
		t.Fatal(err)
	}
	if err := m.RespondElicit("e1", "accept", map[string]any{"k": "v"}); err != nil {
		t.Fatal(err)
	}
	if len(m.Responded) != 1 || m.Responded[0] != (PermissionDecision{ID: "p1", Approve: true}) {
		t.Fatalf("Responded = %v", m.Responded)
	}
	if len(m.RespondedInput) != 1 || m.RespondedInput[0] != (InputDecision{ID: "i1", Answer: "answer"}) {
		t.Fatalf("RespondedInput = %v", m.RespondedInput)
	}
	if len(m.RespondedPlan) != 1 || m.RespondedPlan[0].Action != "approve" || !m.RespondedPlan[0].Approved {
		t.Fatalf("RespondedPlan = %v", m.RespondedPlan)
	}
	if len(m.RespondedElicit) != 1 || m.RespondedElicit[0].Content["k"] != "v" {
		t.Fatalf("RespondedElicit = %v", m.RespondedElicit)
	}
}

func TestMockAbortAndListModels(t *testing.T) {
	m := NewMockClient()
	if err := m.Abort(context.Background(), "sess"); err != nil {
		t.Fatal(err)
	}
	if len(m.Aborted) != 1 || m.Aborted[0] != "sess" {
		t.Fatalf("Aborted = %v", m.Aborted)
	}
	m.Models = []ModelInfo{{ID: "gpt-5", Name: "GPT-5"}}
	got, err := m.ListModels(context.Background())
	if err != nil || len(got) != 1 || got[0].ID != "gpt-5" {
		t.Fatalf("ListModels = %v, %v", got, err)
	}
}

func TestMockSessionManagement(t *testing.T) {
	m := NewMockClient()
	ctx := context.Background()
	m.Sessions = []SessionMeta{{ID: "a"}, {ID: "b"}}
	m.Histories = map[string][]Event{"a": {{Type: EvMessage, Text: "hi"}}}

	metas, err := m.ListSessions(ctx)
	if err != nil || len(metas) != 2 {
		t.Fatalf("ListSessions = %v, %v", metas, err)
	}
	id, err := m.ResumeSession(ctx, "a", SessionSpec{})
	if err != nil || id != "a" {
		t.Fatalf("ResumeSession = %q, %v", id, err)
	}
	if len(m.Resumed) != 1 || m.Resumed[0] != "a" {
		t.Fatalf("Resumed = %v", m.Resumed)
	}
	hist, err := m.SessionHistory(ctx, "a")
	if err != nil || len(hist) != 1 || hist[0].Text != "hi" {
		t.Fatalf("SessionHistory = %v, %v", hist, err)
	}
	if err := m.DeleteSession(ctx, "b"); err != nil {
		t.Fatal(err)
	}
	if len(m.Deleted) != 1 || m.Deleted[0] != "b" {
		t.Fatalf("Deleted = %v", m.Deleted)
	}
}

func TestMockSessionManagementErrors(t *testing.T) {
	sentinel := errors.New("nope")
	ctx := context.Background()
	m := NewMockClient()
	m.ListSessionsErr, m.ResumeErr, m.HistoryErr, m.DeleteErr = sentinel, sentinel, sentinel, sentinel
	if _, err := m.ListSessions(ctx); !errors.Is(err, sentinel) {
		t.Fatal("ListSessions should surface the error")
	}
	if _, err := m.ResumeSession(ctx, "a", SessionSpec{}); !errors.Is(err, sentinel) {
		t.Fatal("ResumeSession should surface the error")
	}
	if _, err := m.SessionHistory(ctx, "a"); !errors.Is(err, sentinel) {
		t.Fatal("SessionHistory should surface the error")
	}
	if err := m.DeleteSession(ctx, "a"); !errors.Is(err, sentinel) {
		t.Fatal("DeleteSession should surface the error")
	}
}

func TestMockEmitAndCloseIdempotent(t *testing.T) {
	m := NewMockClient()
	m.Emit(Event{Type: EvMessage, Text: "x"})
	if e := <-m.Events(); e.Text != "x" {
		t.Fatalf("Events delivered %q", e.Text)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal("Close must be idempotent")
	}
	// Emit after close must not panic (send-on-closed-channel guard).
	m.Emit(Event{Type: EvMessage, Text: "dropped"})
}
