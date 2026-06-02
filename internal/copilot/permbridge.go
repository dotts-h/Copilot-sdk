package copilot

import (
	"strconv"
	"sync"
)

// permBridge connects the SDK's synchronous OnPermissionRequest callback (which
// must return a decision inline) to the asynchronous TUI (which answers later
// via Respond). The callback calls begin() to get an id + a one-shot channel,
// emits an EvPermission carrying the id, and blocks on the channel; the TUI's
// Respond() routes the user's decision back through resolve().
type permBridge struct {
	mu      sync.Mutex
	seq     int
	waiters map[string]chan bool
}

func newPermBridge() *permBridge {
	return &permBridge{waiters: make(map[string]chan bool)}
}

// begin registers a pending decision and returns its id and channel.
func (p *permBridge) begin() (string, chan bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.seq++
	id := "perm-" + strconv.Itoa(p.seq)
	ch := make(chan bool, 1)
	p.waiters[id] = ch
	return id, ch
}

// resolve delivers a decision to a waiting callback. It returns false if the id
// is unknown (already answered or never existed).
func (p *permBridge) resolve(id string, approve bool) bool {
	p.mu.Lock()
	ch := p.waiters[id]
	delete(p.waiters, id)
	p.mu.Unlock()
	if ch == nil {
		return false
	}
	ch <- approve
	return true
}

// pending reports how many decisions are outstanding (for tests/diagnostics).
func (p *permBridge) pending() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.waiters)
}
