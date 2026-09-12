package chat

import (
	"sync"
	"time"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa"
)

const (
	DefaultHistoryMaxMessages = 20
	DefaultHistoryTTL         = 30 * time.Minute
	maxSessions               = 5000
)

type session struct {
	lastUsed time.Time
	messages []nexxa.Message
}

// Memory holds per-session chat history in process memory. The store is
// bounded: each session keeps at most maxMessages turns and entries expire
// after ttl without activity.
type Memory struct {
	mu       sync.Mutex
	sessions map[string]*session
	maxMsg   int
	ttl      time.Duration
}

func NewMemory(maxMsg int, ttl time.Duration) *Memory {
	if maxMsg <= 0 {
		maxMsg = DefaultHistoryMaxMessages
	}
	if ttl <= 0 {
		ttl = DefaultHistoryTTL
	}
	return &Memory{
		sessions: make(map[string]*session),
		maxMsg:   maxMsg,
		ttl:      ttl,
	}
}

// Messages returns the stored history for a session, oldest first. The slice
// is a copy; callers may reuse it freely.
func (m *Memory) Messages(sessionID string) []nexxa.Message {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[sessionID]
	if !ok {
		return nil
	}
	if time.Since(s.lastUsed) > m.ttl {
		delete(m.sessions, sessionID)
		return nil
	}
	out := make([]nexxa.Message, len(s.messages))
	copy(out, s.messages)
	return out
}

// Append records a user turn and its assistant reply for a session.
func (m *Memory) Append(sessionID, userMsg, assistantMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sweepLocked()

	s, ok := m.sessions[sessionID]
	if !ok || time.Since(s.lastUsed) > m.ttl {
		s = &session{messages: make([]nexxa.Message, 0, m.maxMsg)}
		m.sessions[sessionID] = s
	}
	s.lastUsed = time.Now()
	s.messages = append(s.messages,
		nexxa.Message{Role: "user", Content: userMsg},
		nexxa.Message{Role: "assistant", Content: assistantMsg},
	)
	if excess := len(s.messages) - m.maxMsg; excess > 0 {
		if excess%2 == 1 {
			excess++
		}
		s.messages = s.messages[excess:]
	}
}

func (m *Memory) sweepLocked() {
	if len(m.sessions) < maxSessions {
		return
	}
	now := time.Now()
	for id, s := range m.sessions {
		if now.Sub(s.lastUsed) > m.ttl {
			delete(m.sessions, id)
		}
	}
}