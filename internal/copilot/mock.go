package copilot

import (
	"context"
	"sync"
)

// MockClient is an in-memory Client for tests and offline demos. It records
// calls and lets the test drive the event stream.
type MockClient struct {
	mu       sync.Mutex
	events   chan Event
	sessions int
	Sent     []string // prompts seen by Send
	Aborted  []string // session ids passed to Abort
	closed   bool

	// CreateErr / SendErr, if set, are returned by the respective methods.
	CreateErr error
	SendErr   error
}

// NewMockClient returns a ready mock with a buffered event channel.
func NewMockClient() *MockClient {
	return &MockClient{events: make(chan Event, 256)}
}

// CreateSession implements Client.
func (m *MockClient) CreateSession(_ context.Context, _ SessionSpec) (string, error) {
	if m.CreateErr != nil {
		return "", m.CreateErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions++
	return "mock-session", nil
}

// Send implements Client.
func (m *MockClient) Send(_ context.Context, sessionID, prompt string) error {
	if m.SendErr != nil {
		return m.SendErr
	}
	m.mu.Lock()
	m.Sent = append(m.Sent, prompt)
	m.mu.Unlock()
	return nil
}

// Abort implements Client.
func (m *MockClient) Abort(_ context.Context, sessionID string) error {
	m.mu.Lock()
	m.Aborted = append(m.Aborted, sessionID)
	m.mu.Unlock()
	return nil
}

// Events implements Client.
func (m *MockClient) Events() <-chan Event { return m.events }

// Emit pushes an event onto the stream for the TUI to consume.
func (m *MockClient) Emit(e Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closed {
		m.events <- e
	}
}

// Close implements Client.
func (m *MockClient) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closed {
		m.closed = true
		close(m.events)
	}
	return nil
}
