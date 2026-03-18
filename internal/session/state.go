package session

import (
	"strings"

	"github.com/Neneka448/gogoclaw/internal/utils"
	openai "github.com/sashabaranov/go-openai"
)

type sessionState struct {
	id       string
	senderID string
	data     SessionFile
}

func newSessionState(sessionID string, senderID string) sessionState {
	return newSessionStateFromSnapshot(sessionID, senderID, SessionFile{})
}

func newSessionStateFromSnapshot(sessionID string, senderID string, snapshot SessionFile) sessionState {
	state := sessionState{
		id:       sessionID,
		senderID: senderID,
		data: SessionFile{
			Revision: snapshot.Revision,
			Meta: SessionMeta{
				SessionKey:     snapshot.Meta.SessionKey,
				SenderID:       snapshot.Meta.SenderID,
				Channel:        snapshot.Meta.Channel,
				Type:           snapshot.Meta.Type,
				IngestedDigest: snapshot.Meta.IngestedDigest,
			},
			Messages: utils.CloneMessages(snapshot.Messages),
		},
	}
	if state.data.Meta.SessionKey == "" {
		state.data.Meta.SessionKey = sessionID
	}
	if state.data.Meta.SenderID == "" {
		state.data.Meta.SenderID = senderID
	}
	if state.data.Meta.Channel == "" {
		state.data.Meta.Channel = inferSessionChannel(state.data.Meta.SessionKey)
	}
	if state.data.Messages == nil {
		state.data.Messages = []openai.ChatCompletionMessage{}
	}
	if state.senderID == "" {
		state.senderID = state.data.Meta.SenderID
	}
	return state
}

func (state sessionState) clone() sessionState {
	return newSessionStateFromSnapshot(state.id, state.senderID, state.snapshot())
}

func (state sessionState) snapshot() SessionFile {
	return SessionFile{
		Revision: state.data.Revision,
		Meta: SessionMeta{
			SessionKey:     state.data.Meta.SessionKey,
			SenderID:       state.data.Meta.SenderID,
			Channel:        state.data.Meta.Channel,
			Type:           state.data.Meta.Type,
			IngestedDigest: state.data.Meta.IngestedDigest,
		},
		Messages: utils.CloneMessages(state.data.Messages),
	}
}

func (state sessionState) getMessages(memoryWindow int) []openai.ChatCompletionMessage {
	start := 0
	if memoryWindow > 0 && len(state.data.Messages) > memoryWindow {
		start = len(state.data.Messages) - memoryWindow
	}
	start = normalizeWindowStart(state.data.Messages, start)
	return utils.CloneMessages(state.data.Messages[start:])
}

func (state sessionState) getMemoryIngestedDigest() string {
	return state.data.Meta.IngestedDigest
}

func (state sessionState) withSenderID(senderID string) (SessionMeta, bool) {
	senderID = strings.TrimSpace(senderID)
	if senderID == "" || state.data.Meta.SenderID != "" {
		return SessionMeta{}, false
	}
	meta := state.data.Meta
	meta.SenderID = senderID
	return meta, true
}

func (state sessionState) withUpdatedMetadata(channel string, sessionType string) (SessionMeta, bool) {
	meta := state.data.Meta
	updated := false
	channel = strings.TrimSpace(channel)
	if channel == "" {
		channel = inferSessionChannel(state.id)
	}
	if channel != "" && meta.Channel != channel {
		meta.Channel = channel
		updated = true
	}
	sessionType = strings.TrimSpace(sessionType)
	if sessionType != "" && meta.Type != sessionType {
		meta.Type = sessionType
		updated = true
	}
	return meta, updated
}

func (state *sessionState) appendMessages(messages []openai.ChatCompletionMessage, revision uint64) bool {
	if len(messages) == 0 {
		return false
	}
	state.data.Messages = append(state.data.Messages, messages...)
	state.data.Revision = revision
	return true
}

func (state sessionState) withMemoryIngestedDigest(digest string) (SessionMeta, bool) {
	if state.data.Meta.IngestedDigest == digest {
		return SessionMeta{}, false
	}
	meta := state.data.Meta
	meta.IngestedDigest = digest
	return meta, true
}

func (state sessionState) nextRevision() uint64 {
	return state.data.Revision + 1
}

func (state *sessionState) replaceMetadata(meta SessionMeta, revision uint64) {
	if meta.SenderID != "" {
		state.senderID = meta.SenderID
	}
	state.data.Meta = meta
	state.data.Revision = revision
}

func (state *sessionState) reset() {
	state.data.Messages = []openai.ChatCompletionMessage{}
	state.data.Meta.IngestedDigest = ""
}
