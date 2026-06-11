// Package pause is the human-in-the-loop pause ledger: the typed records a
// sub-agent parks on when it needs the human, and their idempotent resolution.
//
// It is the generalization of the permission bridge (internal/copilot/bridge.go):
// a blocking caller registers a Pause and waits on its one-shot channel while the
// asynchronous UI resolves it via Resolve. Unlike that bridge, a Pause carries a
// typed Kind (input | issue | budget | permission) and a set of capability flags
// (continue | respond | cancel) so the surface can render only the buttons the
// caller declared (the Agent Inbox model), and an optional SLA Deadline + default.
//
// The package is pure and dependency-free so the orchestrator's pause semantics —
// capability flags, continue/cancel, idempotent resolution, and the SLA timeout —
// are table-tested with no HTTP and no client. The orchestrator (internal/web)
// owns one Ledger per session and is the only writer. — ADR-0043, epic 0069 S4.
package pause

import (
	"strconv"
	"sync"
	"time"
)

// Kind classifies why a sub-agent parked, so the surface can label the wait.
type Kind string

const (
	KindInput      Kind = "input"      // the agent needs a free-form instruction/answer
	KindIssue      Kind = "issue"      // the agent hit a blocker and escalated
	KindBudget     Kind = "budget"     // a budget/leash gate parked the turn
	KindPermission Kind = "permission" // a native permission ask routed into the pause surface
)

// Cap is a capability flag: an action the surface may offer for a pause. A pause
// declares its caps at registration so a render shows only the relevant buttons.
type Cap string

const (
	CapContinue Cap = "continue" // resume the agent, delivering a payload as the tool result
	CapRespond  Cap = "respond"  // submit structured input (a form), then continue
	CapCancel   Cap = "cancel"   // cooperatively cancel — the agent's turn ends cleanly
)

// Action is the verb a Resolution carries.
type Action string

const (
	ActContinue Action = "continue" // resume with Payload
	ActCancel   Action = "cancel"   // cooperative cancel — "wrap up"
	ActTimeout  Action = "timeout"  // the SLA elapsed; the surface applied a default
)

// Resolution is the outcome delivered to the parked (blocked) caller: the verb and
// the human's instruction/answer (on continue) or the default note (on timeout).
type Resolution struct {
	Action  Action
	Payload string
}

// Pause is a typed human-in-the-loop park: a sub-agent hit something it cannot
// decide alone and is waiting on the human. Its fields are immutable after
// Register; the resolution is delivered on ch exactly once (buffered, so a Resolve
// never blocks even if no one is yet waiting).
type Pause struct {
	ID      string
	AgentID string // the sub-agent/lane instance that parked; "" for a root-level pause
	Kind    Kind
	Message string
	Caps    []Cap
	Created time.Time
	// Deadline is the SLA timeout instant; zero disables it (the default in
	// interactive mode — only autopilot arms a timer). On expiry Sweep resolves the
	// pause to OnExpiry.
	Deadline time.Time
	OnExpiry Resolution

	ch chan Resolution
}

// Can reports whether the pause was registered with capability c.
func (p *Pause) Can(c Cap) bool {
	for _, have := range p.Caps {
		if have == c {
			return true
		}
	}
	return false
}

// Wait blocks until the pause is resolved and returns its resolution. It is the
// parked caller's side of the one-shot channel; Resolve / CancelAll / Sweep are
// the resolving sides.
func (p *Pause) Wait() Resolution { return <-p.ch }

// Ledger is the orchestrator-owned set of open pauses. All methods are safe for
// concurrent use; the orchestrator registers from a tool-handler goroutine and
// resolves from an HTTP handler goroutine.
type Ledger struct {
	mu     sync.Mutex
	seq    int
	prefix string
	open   map[string]*Pause
	order  []string // registration order, for a stable Pending() snapshot
}

// NewLedger returns an empty ledger. The id prefix ("pause") is part of the wire
// ids the UI posts back to, so it is fixed here, mirroring the copilot bridges.
func NewLedger() *Ledger {
	return &Ledger{prefix: "pause", open: make(map[string]*Pause)}
}

// Register stores a copy of p — stamping its ID and one-shot channel — and returns
// the stored pointer, whose Wait the caller blocks on. Created defaults to now when
// unset so the timeline always has an instant to render.
func (l *Ledger) Register(p Pause) *Pause {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seq++
	p.ID = l.prefix + "-" + strconv.Itoa(l.seq)
	if p.Created.IsZero() {
		p.Created = time.Now()
	}
	p.ch = make(chan Resolution, 1)
	stored := &p
	l.open[p.ID] = stored
	l.order = append(l.order, p.ID)
	return stored
}

// Resolve delivers res to the pause's parked caller exactly once and removes it
// from the ledger. It is idempotent: a second Resolve (or one after CancelAll /
// Sweep already settled it, or for an unknown id) returns false and changes
// nothing — the duplicate-POST and abort-while-pending races collapse here.
func (l *Ledger) Resolve(id string, res Resolution) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.deliver(id, res)
}

// CancelAll cooperatively cancels every open pause (a run abort, ADR-0024),
// delivering a cancel carrying payload to each exactly once. Returns how many it
// settled.
func (l *Ledger) CancelAll(payload string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	ids := append([]string(nil), l.order...)
	n := 0
	for _, id := range ids {
		if l.deliver(id, Resolution{Action: ActCancel, Payload: payload}) {
			n++
		}
	}
	return n
}

// Sweep resolves every open pause whose Deadline has passed (non-zero and at or
// before now) to its OnExpiry default, exactly once, and returns the settled ids in
// registration order. The clock is the caller's, so the SLA timeout is
// deterministic in tests; autopilot drives it from a ticker.
func (l *Ledger) Sweep(now time.Time) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var swept []string
	for _, id := range append([]string(nil), l.order...) {
		p := l.open[id]
		if p == nil || p.Deadline.IsZero() || p.Deadline.After(now) {
			continue
		}
		if l.deliver(id, p.OnExpiry) {
			swept = append(swept, id)
		}
	}
	return swept
}

// Get returns a snapshot copy of the open pause with id.
func (l *Ledger) Get(id string) (Pause, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	p := l.open[id]
	if p == nil {
		return Pause{}, false
	}
	return p.snapshot(), true
}

// Pending returns snapshot copies of the open pauses in registration order, for
// rendering. The channel is not exposed.
func (l *Ledger) Pending() []Pause {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Pause, 0, len(l.order))
	for _, id := range l.order {
		if p := l.open[id]; p != nil {
			out = append(out, p.snapshot())
		}
	}
	return out
}

// deliver settles one pause and drops it from the ledger. Caller holds l.mu.
// Reports whether a pause with that id was open (false = already settled/unknown),
// which is the single idempotency point every resolution path routes through.
func (l *Ledger) deliver(id string, res Resolution) bool {
	p := l.open[id]
	if p == nil {
		return false
	}
	delete(l.open, id)
	for i, oid := range l.order {
		if oid == id {
			l.order = append(l.order[:i], l.order[i+1:]...)
			break
		}
	}
	p.ch <- res // buffered cap 1: a single delivery never blocks
	return true
}

// snapshot returns a copy of the pause without its channel, for read-only callers.
func (p *Pause) snapshot() Pause {
	cp := *p
	cp.ch = nil
	return cp
}
