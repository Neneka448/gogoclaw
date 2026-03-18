package session

import (
	"sync"

	"github.com/Neneka448/gogoclaw/internal/utils"
	openai "github.com/sashabaranov/go-openai"
)

type managedSession struct {
	mu    sync.Mutex
	state sessionState
	store sessionStore
}

func newManagedSession(store sessionStore, state sessionState) *managedSession {
	return &managedSession{
		state: state,
		store: store,
	}
}

func (session *managedSession) Close() error {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.store.Save(session.state.id, session.state.snapshot())
}

func (session *managedSession) GetSessionID() string {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.state.id
}

func (session *managedSession) GetMessages(memoryWindow int) []openai.ChatCompletionMessage {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.state.getMessages(memoryWindow)
}

func (session *managedSession) GetMemoryIngestedDigest() string {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.state.getMemoryIngestedDigest()
}

func (session *managedSession) UpdateMetadata(channel string, sessionType string) error {
	session.mu.Lock()
	defer session.mu.Unlock()

	meta, updated := session.state.withUpdatedMetadata(channel, sessionType)
	if !updated {
		return nil
	}
	revision := session.state.nextRevision()
	if err := session.store.SaveMetadata(session.state.id, revision, meta); err != nil {
		return err
	}
	session.state.replaceMetadata(meta, revision)
	return nil
}

func (session *managedSession) AppendMessage(message openai.ChatCompletionMessage) error {
	return session.AppendMessages([]openai.ChatCompletionMessage{message})
}

func (session *managedSession) AppendMessages(messages []openai.ChatCompletionMessage) error {
	session.mu.Lock()
	defer session.mu.Unlock()

	if len(messages) == 0 {
		return nil
	}
	clonedMessages := utils.CloneMessages(messages)
	revision := session.state.nextRevision()
	if err := session.store.AppendMessages(session.state.id, revision, clonedMessages); err != nil {
		return err
	}
	session.state.appendMessages(clonedMessages, revision)
	return nil
}

func (session *managedSession) MarkMemoryIngested(digest string) error {
	session.mu.Lock()
	defer session.mu.Unlock()

	meta, updated := session.state.withMemoryIngestedDigest(digest)
	if !updated {
		return nil
	}
	revision := session.state.nextRevision()
	if err := session.store.SaveMetadata(session.state.id, revision, meta); err != nil {
		return err
	}
	session.state.replaceMetadata(meta, revision)
	return nil
}

func (session *managedSession) ArchiveAndReset() error {
	session.mu.Lock()
	defer session.mu.Unlock()

	current := session.state.snapshot()
	if len(current.Messages) > 0 {
		if err := session.store.Archive(session.state.id, current); err != nil {
			return err
		}
	}

	next := session.state.snapshot()
	next.Messages = []openai.ChatCompletionMessage{}
	next.Meta.IngestedDigest = ""
	if err := session.store.Save(session.state.id, next); err != nil {
		return err
	}
	session.state.reset()
	return nil
}

func (session *managedSession) ensureSenderID(senderID string) error {
	session.mu.Lock()
	defer session.mu.Unlock()

	meta, updated := session.state.withSenderID(senderID)
	if !updated {
		return nil
	}
	revision := session.state.nextRevision()
	if err := session.store.SaveMetadata(session.state.id, revision, meta); err != nil {
		return err
	}
	session.state.replaceMetadata(meta, revision)
	return nil
}
