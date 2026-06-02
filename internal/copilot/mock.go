package copilot

import (
	"context"
	"sync"
)

// MockClient is an in-memory Client for tests and offline use. It records calls
// and lets the test drive the event stream.
type MockClient struct {
	mu          sync.Mutex
	events      chan Event
	sessions    int
	Sent        []string
	LastAttach  []string
	Aborted     []string
	Resumed     []string
	Responded   []PermissionDecision
	lastSession string
	closed      bool

	CreateErr error
	SendErr   error
}

// NewMockClient returns a ready mock with a buffered event channel.
func NewMockClient() *MockClient {
	return &MockClient{events: make(chan Event, 256)}
}

// CreateSession implements Client.
func (m *MockClient) CreateSession(context.Context, SessionSpec) (string, error) {
	if m.CreateErr != nil {
		return "", m.CreateErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions++
	m.lastSession = "mock-session"
	return "mock-session", nil
}

// ResumeSession implements Client.
func (m *MockClient) ResumeSession(_ context.Context, sessionID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Resumed = append(m.Resumed, sessionID)
	m.lastSession = sessionID
	return sessionID, nil
}

// LastSessionID implements Client.
func (m *MockClient) LastSessionID(_ context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastSession, nil
}

// Send implements Client.
func (m *MockClient) Send(_ context.Context, _, prompt string, attachments []string) error {
	if m.SendErr != nil {
		return m.SendErr
	}
	m.mu.Lock()
	m.Sent = append(m.Sent, prompt)
	m.LastAttach = attachments
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

// PermissionDecision records a Respond call for assertions.
type PermissionDecision struct {
	ID      string
	Approve bool
}

// Respond implements Client.
func (m *MockClient) Respond(id string, approve bool) error {
	m.mu.Lock()
	m.Responded = append(m.Responded, PermissionDecision{ID: id, Approve: approve})
	m.mu.Unlock()
	return nil
}

// Events implements Client.
func (m *MockClient) Events() <-chan Event { return m.events }

// Emit pushes an event onto the stream.
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
