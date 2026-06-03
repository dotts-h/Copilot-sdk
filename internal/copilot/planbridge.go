package copilot

import (
	"strconv"
	"sync"
)

// planDecision is the user's answer to an exit-plan-mode request, delivered
// through the planBridge: approve and proceed with Action, or decline with
// Feedback requesting changes.
type planDecision struct {
	Approved bool
	Action   string
	Feedback string
}

// planBridge connects the SDK's synchronous OnExitPlanModeRequest callback to
// the asynchronous web UI, mirroring permBridge/inputBridge. The resolution
// carries a planDecision (approve+action or decline+feedback) rather than a bare
// bool/string. The callback calls begin() for an id + one-shot channel, emits an
// EvPlanReview carrying the id, and blocks; RespondPlan() routes the decision
// back through resolve().
type planBridge struct {
	mu      sync.Mutex
	seq     int
	waiters map[string]chan planDecision
}

func newPlanBridge() *planBridge {
	return &planBridge{waiters: make(map[string]chan planDecision)}
}

// begin registers a pending plan review and returns its id and channel.
func (b *planBridge) begin() (string, chan planDecision) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seq++
	id := "plan-" + strconv.Itoa(b.seq)
	ch := make(chan planDecision, 1)
	b.waiters[id] = ch
	return id, ch
}

// resolve delivers a decision to a waiting callback. It returns false if the id
// is unknown (already answered or never existed).
func (b *planBridge) resolve(id string, d planDecision) bool {
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

// pending reports how many reviews are outstanding (for tests/diagnostics).
func (b *planBridge) pending() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.waiters)
}
