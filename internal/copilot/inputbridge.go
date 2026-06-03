package copilot

import (
	"strconv"
	"sync"
)

// inputBridge connects the SDK's synchronous OnUserInputRequest callback (which
// must return an answer inline) to the asynchronous web UI (which answers later
// via RespondInput). It mirrors permBridge, but the resolution carries the
// user's answer string rather than an approve/reject bool. The callback calls
// begin() to get an id + a one-shot channel, emits an EvUserInput carrying the
// id, and blocks on the channel; RespondInput() routes the answer back through
// resolve().
type inputBridge struct {
	mu      sync.Mutex
	seq     int
	waiters map[string]chan string
}

func newInputBridge() *inputBridge {
	return &inputBridge{waiters: make(map[string]chan string)}
}

// begin registers a pending question and returns its id and channel.
func (b *inputBridge) begin() (string, chan string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seq++
	id := "ask-" + strconv.Itoa(b.seq)
	ch := make(chan string, 1)
	b.waiters[id] = ch
	return id, ch
}

// resolve delivers an answer to a waiting callback. It returns false if the id
// is unknown (already answered or never existed).
func (b *inputBridge) resolve(id, answer string) bool {
	b.mu.Lock()
	ch := b.waiters[id]
	delete(b.waiters, id)
	b.mu.Unlock()
	if ch == nil {
		return false
	}
	ch <- answer
	return true
}

// pending reports how many questions are outstanding (for tests/diagnostics).
func (b *inputBridge) pending() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.waiters)
}
