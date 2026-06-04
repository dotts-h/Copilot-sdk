package copilot

import (
	"context"
	"sync"
)

// MockClient is an in-memory Client for tests and offline use. It records calls
// and lets the test drive the event stream.
type MockClient struct {
	mu              sync.Mutex
	events          chan Event
	sessions        int
	Sent            []string
	SentModes       []string // agent mode passed alongside each Sent prompt
	LastAttach      []string
	Aborted         []string
	Responded       []PermissionDecision
	RespondedInput  []InputDecision
	RespondedPlan   []PlanDecision
	RespondedElicit []ElicitDecision
	closed          bool

	CreateErr error
	SendErr   error
	// Models is returned by ListModels; tests set it to drive the model picker.
	Models        []ModelInfo
	ListModelsErr error

	// Session-management state, settable by tests to drive the session picker.
	Sessions        []SessionMeta      // returned by ListSessions
	Histories       map[string][]Event // returned by SessionHistory, keyed by id
	ListSessionsErr error
	ResumeErr       error
	HistoryErr      error
	DeleteErr       error
	Resumed         []string // session ids passed to ResumeSession
	Deleted         []string // session ids passed to DeleteSession
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
	return "mock-session", nil
}

// Send implements Client.
func (m *MockClient) Send(_ context.Context, _, prompt string, attachments []string, agentMode string) error {
	if m.SendErr != nil {
		return m.SendErr
	}
	m.mu.Lock()
	m.Sent = append(m.Sent, prompt)
	m.SentModes = append(m.SentModes, agentMode)
	m.LastAttach = attachments
	m.mu.Unlock()
	return nil
}

// SentModeAt returns the agent mode passed with the i-th sent prompt, safe for
// concurrent reads.
func (m *MockClient) SentModeAt(i int) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.SentModes[i]
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

// InputDecision records a RespondInput call for assertions.
type InputDecision struct {
	ID     string
	Answer string
}

// RespondInput implements Client.
func (m *MockClient) RespondInput(id, answer string) error {
	m.mu.Lock()
	m.RespondedInput = append(m.RespondedInput, InputDecision{ID: id, Answer: answer})
	m.mu.Unlock()
	return nil
}

// PlanDecision records a RespondPlan call for assertions.
type PlanDecision struct {
	ID       string
	Approved bool
	Action   string
	Feedback string
}

// RespondPlan implements Client.
func (m *MockClient) RespondPlan(id string, approved bool, action, feedback string) error {
	m.mu.Lock()
	m.RespondedPlan = append(m.RespondedPlan, PlanDecision{ID: id, Approved: approved, Action: action, Feedback: feedback})
	m.mu.Unlock()
	return nil
}

// ElicitDecision records a RespondElicit call for assertions.
type ElicitDecision struct {
	ID      string
	Action  string
	Content map[string]any
}

// RespondElicit implements Client.
func (m *MockClient) RespondElicit(id, action string, content map[string]any) error {
	m.mu.Lock()
	m.RespondedElicit = append(m.RespondedElicit, ElicitDecision{ID: id, Action: action, Content: content})
	m.mu.Unlock()
	return nil
}

// SentCount returns the number of prompts sent so far, safe for concurrent reads.
func (m *MockClient) SentCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Sent)
}

// SentAt returns the i-th sent prompt, safe for concurrent reads.
func (m *MockClient) SentAt(i int) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Sent[i]
}

// ListModels implements Client.
func (m *MockClient) ListModels(context.Context) ([]ModelInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Models, m.ListModelsErr
}

// ListSessions implements Client.
func (m *MockClient) ListSessions(context.Context) ([]SessionMeta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Sessions, m.ListSessionsErr
}

// ResumeSession implements Client.
func (m *MockClient) ResumeSession(_ context.Context, sessionID string, _ SessionSpec) (string, error) {
	if m.ResumeErr != nil {
		return "", m.ResumeErr
	}
	m.mu.Lock()
	m.Resumed = append(m.Resumed, sessionID)
	m.mu.Unlock()
	return sessionID, nil
}

// SessionHistory implements Client.
func (m *MockClient) SessionHistory(_ context.Context, sessionID string) ([]Event, error) {
	if m.HistoryErr != nil {
		return nil, m.HistoryErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Histories[sessionID], nil
}

// DeleteSession implements Client.
func (m *MockClient) DeleteSession(_ context.Context, sessionID string) error {
	if m.DeleteErr != nil {
		return m.DeleteErr
	}
	m.mu.Lock()
	m.Deleted = append(m.Deleted, sessionID)
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
