package pause

import (
	"testing"
	"time"
)

// register a pause and resolve it with continue; the parked caller receives the
// payload exactly once.
func TestRegisterResolveContinue(t *testing.T) {
	l := NewLedger()
	p := l.Register(Pause{AgentID: "sub-1", Kind: KindInput, Message: "which branch?",
		Caps: []Cap{CapContinue, CapCancel}})
	if p.ID == "" {
		t.Fatal("Register did not stamp an ID")
	}
	if got := l.Pending(); len(got) != 1 || got[0].ID != p.ID {
		t.Fatalf("Pending() = %v, want one pause %q", got, p.ID)
	}

	if !l.Resolve(p.ID, Resolution{Action: ActContinue, Payload: "main"}) {
		t.Fatal("Resolve returned false for an open pause")
	}
	res := p.Wait()
	if res.Action != ActContinue || res.Payload != "main" {
		t.Fatalf("Wait() = %+v, want continue/main", res)
	}
	if got := l.Pending(); len(got) != 0 {
		t.Fatalf("Pending() after resolve = %v, want empty", got)
	}
}

// a second Resolve for the same id is a no-op (the duplicate-POST race).
func TestDoubleResolveIsNoOp(t *testing.T) {
	l := NewLedger()
	p := l.Register(Pause{Caps: []Cap{CapContinue, CapCancel}})
	if !l.Resolve(p.ID, Resolution{Action: ActContinue}) {
		t.Fatal("first Resolve should succeed")
	}
	if l.Resolve(p.ID, Resolution{Action: ActCancel}) {
		t.Fatal("second Resolve should be a no-op (already resolved)")
	}
	if res := p.Wait(); res.Action != ActContinue {
		t.Fatalf("the FIRST resolution must win: got %+v", res)
	}
}

// CancelAll settles every open pause exactly once, and a later Resolve on a
// cancelled pause is a no-op — the run-abort-while-pending race.
func TestCancelAllThenResolveIsNoOp(t *testing.T) {
	l := NewLedger()
	a := l.Register(Pause{AgentID: "a", Caps: []Cap{CapContinue, CapCancel}})
	b := l.Register(Pause{AgentID: "b", Caps: []Cap{CapContinue, CapCancel}})

	if n := l.CancelAll("run aborted"); n != 2 {
		t.Fatalf("CancelAll settled %d, want 2", n)
	}
	if l.Resolve(a.ID, Resolution{Action: ActContinue}) {
		t.Fatal("Resolve after CancelAll must be a no-op")
	}
	for _, p := range []*Pause{a, b} {
		res := p.Wait()
		if res.Action != ActCancel || res.Payload != "run aborted" {
			t.Fatalf("CancelAll resolution = %+v, want cancel/aborted", res)
		}
	}
	if got := l.Pending(); len(got) != 0 {
		t.Fatalf("Pending() after CancelAll = %v, want empty", got)
	}
}

// resolving an unknown id is a no-op, not a panic.
func TestResolveUnknownID(t *testing.T) {
	l := NewLedger()
	if l.Resolve("pause-999", Resolution{Action: ActContinue}) {
		t.Fatal("Resolve of an unknown id should return false")
	}
}

// Pending is a stable snapshot in registration order; resolving the first leaves
// the second in place.
func TestPendingOrderAndDrop(t *testing.T) {
	l := NewLedger()
	a := l.Register(Pause{AgentID: "a"})
	b := l.Register(Pause{AgentID: "b"})
	c := l.Register(Pause{AgentID: "c"})
	l.Resolve(b.ID, Resolution{Action: ActCancel})

	got := l.Pending()
	if len(got) != 2 || got[0].ID != a.ID || got[1].ID != c.ID {
		t.Fatalf("Pending() = %v, want [%s %s] in order", got, a.ID, c.ID)
	}
}

// Sweep resolves only the pauses whose deadline has passed, to their per-pause
// default, and leaves SLA-less and not-yet-expired pauses untouched. The clock is
// injected so the timeout is deterministic.
func TestSweepExpiresToDefault(t *testing.T) {
	l := NewLedger()
	base := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)

	expired := l.Register(Pause{AgentID: "slow", Deadline: base.Add(1 * time.Minute),
		OnExpiry: Resolution{Action: ActCancel, Payload: "timed out — cancel"}})
	fresh := l.Register(Pause{AgentID: "soon", Deadline: base.Add(10 * time.Minute),
		OnExpiry: Resolution{Action: ActCancel}})
	noSLA := l.Register(Pause{AgentID: "patient"}) // zero deadline = no SLA

	swept := l.Sweep(base.Add(5 * time.Minute))
	if len(swept) != 1 || swept[0] != expired.ID {
		t.Fatalf("Sweep swept %v, want only %q", swept, expired.ID)
	}
	if res := expired.Wait(); res.Action != ActCancel || res.Payload != "timed out — cancel" {
		t.Fatalf("expired resolution = %+v, want its OnExpiry default", res)
	}
	if got := l.Pending(); len(got) != 2 {
		t.Fatalf("Pending() after sweep = %d, want 2 (fresh + noSLA still open)", len(got))
	}
	// The fresh and SLA-less pauses are still resolvable normally.
	if !l.Resolve(fresh.ID, Resolution{Action: ActContinue}) ||
		!l.Resolve(noSLA.ID, Resolution{Action: ActContinue}) {
		t.Fatal("unswept pauses should still resolve")
	}
}

// Can reports the capability flags a pause was registered with.
func TestCanCapability(t *testing.T) {
	p := Pause{Caps: []Cap{CapContinue, CapRespond}}
	if !p.Can(CapContinue) || !p.Can(CapRespond) {
		t.Fatal("Can should report declared capabilities")
	}
	if p.Can(CapCancel) {
		t.Fatal("Can should be false for an undeclared capability")
	}
}
