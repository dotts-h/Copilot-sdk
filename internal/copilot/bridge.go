package copilot

import (
	"strconv"
	"sync"
)

// bridge connects a synchronous SDK callback (which must return a decision
// inline) to the asynchronous web UI (which decides later via a Respond* call).
// The callback calls begin() for an id + a one-shot channel, emits an event
// carrying the id, and blocks on the channel; the matching Respond* routes the
// decision back through resolve(). The four request kinds — tool permission,
// ask_user, exit-plan-mode, and elicitation — differ only in the decision type
// carried, so they share this one generic implementation, parameterized by the
// payload type T and an id prefix.
type bridge[T any] struct {
	mu      sync.Mutex
	seq     int
	prefix  string
	waiters map[string]chan T
}

func newBridge[T any](prefix string) *bridge[T] {
	return &bridge[T]{prefix: prefix, waiters: make(map[string]chan T)}
}

// begin registers a pending request and returns its id and one-shot channel.
func (b *bridge[T]) begin() (string, chan T) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seq++
	id := b.prefix + "-" + strconv.Itoa(b.seq)
	ch := make(chan T, 1)
	b.waiters[id] = ch
	return id, ch
}

// resolve delivers a decision to a waiting callback. It returns false if the id
// is unknown (already answered or never existed).
func (b *bridge[T]) resolve(id string, v T) bool {
	b.mu.Lock()
	ch := b.waiters[id]
	delete(b.waiters, id)
	b.mu.Unlock()
	if ch == nil {
		return false
	}
	ch <- v
	return true
}

// pending reports how many requests are outstanding (for tests/diagnostics).
func (b *bridge[T]) pending() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.waiters)
}

// The four concrete bridges. The constructors fix the payload type and id prefix
// (the prefixes are part of the wire ids the UI posts back to, so they must not
// change). permissions carry an approve/reject bool; ask_user carries the
// answer string; the other two carry a decision struct.
func newPermBridge() *bridge[bool]             { return newBridge[bool]("perm") }
func newInputBridge() *bridge[string]          { return newBridge[string]("ask") }
func newPlanBridge() *bridge[planDecision]     { return newBridge[planDecision]("plan") }
func newElicitBridge() *bridge[elicitDecision] { return newBridge[elicitDecision]("elicit") }

// planDecision is the user's answer to an exit-plan-mode request: approve and
// proceed with Action, or decline with Feedback requesting changes.
type planDecision struct {
	Approved bool
	Action   string
	Feedback string
}

// elicitDecision is the user's answer to a schema-driven elicitation request: an
// Action ("accept"/"decline"/"cancel") and, on accept, the submitted form
// Content keyed by field name.
type elicitDecision struct {
	Action  string
	Content map[string]any
}
