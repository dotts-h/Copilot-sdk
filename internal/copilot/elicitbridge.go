package copilot

import (
	"strconv"
	"sync"
)

// elicitDecision is the user's answer to a schema-driven elicitation request,
// delivered through the elicitBridge: an Action ("accept"/"decline"/"cancel")
// and, on accept, the submitted form Content keyed by field name.
type elicitDecision struct {
	Action  string
	Content map[string]any
}

// elicitBridge connects the SDK's synchronous OnElicitationRequest callback to
// the asynchronous web UI, mirroring permBridge/inputBridge/planBridge. The
// callback calls begin() for an id + one-shot channel, emits an EvElicitation
// carrying the id, and blocks; RespondElicit() routes the decision back through
// resolve(). It is the schema-driven-form analogue of inputBridge (which carries
// a single answer string); the resolution here carries a whole form's values.
type elicitBridge struct {
	mu      sync.Mutex
	seq     int
	waiters map[string]chan elicitDecision
}

func newElicitBridge() *elicitBridge {
	return &elicitBridge{waiters: make(map[string]chan elicitDecision)}
}

// begin registers a pending elicitation and returns its id and channel.
func (b *elicitBridge) begin() (string, chan elicitDecision) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seq++
	id := "elicit-" + strconv.Itoa(b.seq)
	ch := make(chan elicitDecision, 1)
	b.waiters[id] = ch
	return id, ch
}

// resolve delivers a decision to a waiting callback. It returns false if the id
// is unknown (already answered or never existed).
func (b *elicitBridge) resolve(id string, d elicitDecision) bool {
	b.mu.Lock()
	ch := b.waiters[id]
	delete(b.waiters, id)
	b.mu.Unlock()
	if ch == nil {
		return false
	}
	ch <- d
	return true
}

// pending reports how many elicitations are outstanding (for tests/diagnostics).
func (b *elicitBridge) pending() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.waiters)
}
