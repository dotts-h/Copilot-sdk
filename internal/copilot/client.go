package copilot

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// Client is the high-level interface the TUI depends on. It is deliberately
// small so it can be mocked in tests (see mock.go).
type Client interface {
	// CreateSession starts a session and returns its id.
	CreateSession(ctx context.Context, spec SessionSpec) (string, error)
	// Send submits a prompt to a session. Responses arrive as events.
	Send(ctx context.Context, sessionID, prompt string) error
	// Abort cancels in-flight processing for a session.
	Abort(ctx context.Context, sessionID string) error
	// Events streams sidecar events until the client is closed.
	Events() <-chan Event
	// Close shuts the client down and releases resources.
	Close() error
}

// ErrClosed is returned when operating on a closed client.
var ErrClosed = errors.New("copilot: client closed")

// StdioClient implements Client over a pair of byte streams (typically the
// sidecar process's stdin/stdout). It is transport-agnostic: tests inject
// in-memory pipes, production injects the child process.
type StdioClient struct {
	w   io.Writer
	enc *json.Encoder

	// writeMu serializes encoder writes. It is deliberately separate from mu so
	// a slow/blocked write never stalls response delivery in readLoop.
	writeMu sync.Mutex

	mu      sync.Mutex
	nextID  int
	waiters map[int]chan Response
	closed  bool

	events chan Event
	done   chan struct{}
	closeC sync.Once
	onStop func() // optional hook run on Close (e.g. kill the process)
}

// NewStdioClient builds a client that reads frames from r and writes requests
// to w, then starts the read loop. onStop, if non-nil, runs once on Close.
func NewStdioClient(r io.Reader, w io.Writer, onStop func()) *StdioClient {
	c := &StdioClient{
		w:       w,
		enc:     json.NewEncoder(w),
		nextID:  0,
		waiters: make(map[int]chan Response),
		events:  make(chan Event, 256),
		done:    make(chan struct{}),
		onStop:  onStop,
	}
	go c.readLoop(r)
	return c
}

// readLoop parses one JSON frame per line, routing responses to waiters and
// events to the events channel. It exits on EOF or read error, failing any
// pending waiters so callers never hang.
func (c *StdioClient) readLoop(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var f Frame
		if err := json.Unmarshal(line, &f); err != nil {
			// Malformed line: surface as an error event, keep going.
			c.emit(Event{Type: EventError, Data: mustJSON(ErrorData{Message: "bad frame: " + err.Error()})})
			continue
		}
		if f.isResponse() {
			c.deliver(Response{ID: f.ID, Result: f.Result, Error: f.Error})
			continue
		}
		if f.Event != "" {
			c.emit(Event{Type: f.Event, Data: f.Data})
		}
	}
	// Stream ended: wake everyone.
	c.failAllWaiters(ErrClosed)
	c.shutdown()
}

func (c *StdioClient) emit(e Event) {
	select {
	case c.events <- e:
	case <-c.done:
	}
}

func (c *StdioClient) deliver(resp Response) {
	c.mu.Lock()
	ch := c.waiters[resp.ID]
	delete(c.waiters, resp.ID)
	c.mu.Unlock()
	if ch != nil {
		ch <- resp
	}
}

func (c *StdioClient) failAllWaiters(err error) {
	c.mu.Lock()
	for id, ch := range c.waiters {
		delete(c.waiters, id)
		ch <- Response{ID: id, Error: err.Error()}
	}
	c.mu.Unlock()
}

// call sends a request and waits for its correlated response or ctx/cancel.
func (c *StdioClient) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	// Marshal params outside any lock.
	var raw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("encode params: %w", err)
		}
		raw = b
	}

	// Register the waiter under the state lock only.
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrClosed
	}
	c.nextID++
	id := c.nextID
	ch := make(chan Response, 1)
	c.waiters[id] = ch
	c.mu.Unlock()

	// Write under the dedicated write lock so response delivery is never blocked.
	c.writeMu.Lock()
	err := c.enc.Encode(Request{ID: id, Method: method, Params: raw})
	c.writeMu.Unlock()
	if err != nil {
		c.mu.Lock()
		delete(c.waiters, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("write request: %w", err)
	}

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.waiters, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	case <-c.done:
		return nil, ErrClosed
	case resp := <-ch:
		if resp.Error != "" {
			return nil, fmt.Errorf("sidecar: %s", resp.Error)
		}
		return resp.Result, nil
	}
}

// CreateSession implements Client.
func (c *StdioClient) CreateSession(ctx context.Context, spec SessionSpec) (string, error) {
	raw, err := c.call(ctx, "createSession", spec)
	if err != nil {
		return "", err
	}
	var res createSessionResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", fmt.Errorf("decode createSession result: %w", err)
	}
	if res.SessionID == "" {
		return "", errors.New("sidecar returned empty session id")
	}
	return res.SessionID, nil
}

// Send implements Client.
func (c *StdioClient) Send(ctx context.Context, sessionID, prompt string) error {
	_, err := c.call(ctx, "send", map[string]string{"sessionId": sessionID, "prompt": prompt})
	return err
}

// Abort implements Client.
func (c *StdioClient) Abort(ctx context.Context, sessionID string) error {
	_, err := c.call(ctx, "abort", map[string]string{"sessionId": sessionID})
	return err
}

// Events implements Client.
func (c *StdioClient) Events() <-chan Event { return c.events }

// Close implements Client.
func (c *StdioClient) Close() error {
	c.shutdown()
	return nil
}

// shutdown is idempotent: it marks closed, closes done/events, and runs onStop.
func (c *StdioClient) shutdown() {
	c.closeC.Do(func() {
		c.mu.Lock()
		c.closed = true
		c.mu.Unlock()
		close(c.done)
		close(c.events)
		if c.onStop != nil {
			c.onStop()
		}
	})
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}
