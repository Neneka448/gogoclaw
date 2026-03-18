package session

import (
	"strings"
	"sync"
)

type sessionManager struct {
	store    sessionStore
	mu       sync.Mutex
	sessions map[string]*managedSession
}

func NewSessionManager(workspacePath string) SessionManager {
	return &sessionManager{
		store:    newFileSessionStore(workspacePath),
		sessions: make(map[string]*managedSession),
	}
}

func (manager *sessionManager) GetOrCreateSession(sessionID string, senderID string) (Session, error) {
	sessionID = strings.TrimSpace(sessionID)
	if err := ValidateSessionID(sessionID); err != nil {
		return nil, err
	}

	manager.mu.Lock()
	if existing, ok := manager.sessions[sessionID]; ok {
		manager.mu.Unlock()
		if err := existing.ensureSenderID(senderID); err != nil {
			return nil, err
		}
		return existing, nil
	}

	snapshot, err := manager.store.Load(sessionID, senderID)
	if err != nil {
		manager.mu.Unlock()
		return nil, err
	}
	currentSession := newManagedSession(manager.store, newSessionStateFromSnapshot(sessionID, senderID, snapshot))
	manager.sessions[sessionID] = currentSession
	manager.mu.Unlock()

	if err := currentSession.ensureSenderID(senderID); err != nil {
		return nil, err
	}
	return currentSession, nil
}

func (manager *sessionManager) ListSessionIDs() ([]string, error) {
	return manager.store.ListSessionIDs()
}

func (manager *sessionManager) Close() error {
	manager.mu.Lock()
	sessions := make([]*managedSession, 0, len(manager.sessions))
	for _, currentSession := range manager.sessions {
		sessions = append(sessions, currentSession)
	}
	manager.mu.Unlock()

	for _, currentSession := range sessions {
		if err := currentSession.Close(); err != nil {
			return err
		}
	}

	return nil
}
