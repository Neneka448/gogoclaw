package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

var sessionNow = time.Now

func SessionNowForTest(now func() time.Time) func() {
	previous := sessionNow
	sessionNow = now
	return func() {
		sessionNow = previous
	}
}

type Session interface {
	Close() error
	GetSessionID() string
	GetMessages(memoryWindow int) []openai.ChatCompletionMessage
	AppendMessage(message openai.ChatCompletionMessage) error
	AppendMessages(messages []openai.ChatCompletionMessage) error
	ReadSessionFile() error
	WriteSessionFile() error
	GetSessionFilePath() string
	ArchiveAndReset() (string, error)
}

type SessionManager interface {
	GetOrCreateSession(sessionID string, senderID string) (Session, error)
	Close() error
}

type SessionFile struct {
	Meta     SessionMeta                    `json:"meta"`
	Messages []openai.ChatCompletionMessage `json:"messages"`
}

type SessionMeta struct {
	SessionKey string `json:"session_key"`
	SenderID   string `json:"sender_id"`
}

type fileSession struct {
	id             string
	senderID       string
	filePath       string
	mu             sync.Mutex
	loaded         bool
	data           SessionFile
	flushRunning   bool
	flushRequested bool
	lastWriteErr   error
}

type sessionManager struct {
	workspacePath string
	mu            sync.Mutex
	sessions      map[string]*fileSession
}

func NewSessionManager(workspacePath string) SessionManager {
	return &sessionManager{
		workspacePath: workspacePath,
		sessions:      make(map[string]*fileSession),
	}
}

func (manager *sessionManager) GetOrCreateSession(sessionID string, senderID string) (Session, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	if existing, ok := manager.sessions[sessionID]; ok {
		if existing.senderID == "" && senderID != "" {
			existing.senderID = senderID
		}
		return existing, nil
	}

	session := &fileSession{
		id:       sessionID,
		senderID: senderID,
		filePath: filepath.Join(manager.workspacePath, "sessions", sessionID+".json"),
		data: SessionFile{
			Meta: SessionMeta{
				SessionKey: sessionID,
				SenderID:   senderID,
			},
			Messages: []openai.ChatCompletionMessage{},
		},
	}

	if err := session.ReadSessionFile(); err != nil {
		return nil, err
	}

	if session.data.Meta.SenderID == "" && senderID != "" {
		session.data.Meta.SenderID = senderID
		if err := session.WriteSessionFile(); err != nil {
			return nil, err
		}
	}

	manager.sessions[sessionID] = session
	return session, nil
}

func (manager *sessionManager) Close() error {
	manager.mu.Lock()
	sessions := make([]*fileSession, 0, len(manager.sessions))
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

func (session *fileSession) Close() error {
	return session.WriteSessionFile()
}

func (session *fileSession) GetSessionID() string {
	return session.id
}

func (session *fileSession) GetSessionFilePath() string {
	return session.filePath
}

func (session *fileSession) ArchiveAndReset() (string, error) {
	session.mu.Lock()
	defer session.mu.Unlock()

	if err := session.ensureLoadedLocked(); err != nil {
		return "", err
	}

	snapshot := session.snapshotLocked()
	archivePath := ""
	if len(snapshot.Messages) > 0 {
		var err error
		archivePath, err = session.archiveSnapshotLocked(snapshot)
		if err != nil {
			return "", err
		}
	}

	session.data.Messages = []openai.ChatCompletionMessage{}
	session.flushRequested = false
	session.lastWriteErr = nil
	if err := session.writeSessionFileLocked(); err != nil {
		return "", err
	}

	return archivePath, nil
}

func (session *fileSession) GetMessages(memoryWindow int) []openai.ChatCompletionMessage {
	session.mu.Lock()
	defer session.mu.Unlock()

	if err := session.ensureLoadedLocked(); err != nil {
		return nil
	}

	start := 0
	if memoryWindow > 0 && len(session.data.Messages) > memoryWindow {
		start = len(session.data.Messages) - memoryWindow
	}

	return cloneMessages(session.data.Messages[start:])
}

func (session *fileSession) AppendMessage(message openai.ChatCompletionMessage) error {
	return session.AppendMessages([]openai.ChatCompletionMessage{message})
}

func (session *fileSession) AppendMessages(messages []openai.ChatCompletionMessage) error {
	session.mu.Lock()
	defer session.mu.Unlock()

	if err := session.ensureLoadedLocked(); err != nil {
		return err
	}

	session.data.Messages = append(session.data.Messages, cloneMessages(messages)...)
	session.scheduleFlushLocked()
	return nil
}

func (session *fileSession) ReadSessionFile() error {
	session.mu.Lock()
	defer session.mu.Unlock()

	return session.readSessionFileLocked()
}

func (session *fileSession) WriteSessionFile() error {
	session.mu.Lock()
	if err := session.ensureLoadedLocked(); err != nil {
		session.mu.Unlock()
		return err
	}
	snapshot := session.snapshotLocked()
	session.flushRequested = false
	session.mu.Unlock()

	if err := session.writeSnapshot(snapshot); err != nil {
		session.mu.Lock()
		session.lastWriteErr = err
		session.mu.Unlock()
		return err
	}

	return nil
}

func (session *fileSession) ensureLoadedLocked() error {
	if session.loaded {
		return nil
	}

	return session.readSessionFileLocked()
}

func (session *fileSession) readSessionFileLocked() error {
	if err := os.MkdirAll(filepath.Dir(session.filePath), 0755); err != nil {
		return fmt.Errorf("create sessions directory: %w", err)
	}

	if _, err := os.Stat(session.filePath); os.IsNotExist(err) {
		session.data = SessionFile{
			Meta: SessionMeta{
				SessionKey: session.id,
				SenderID:   session.senderID,
			},
			Messages: []openai.ChatCompletionMessage{},
		}
		session.loaded = true
		return session.writeSessionFileLocked()
	} else if err != nil {
		return fmt.Errorf("stat session file: %w", err)
	}

	content, err := os.ReadFile(session.filePath)
	if err != nil {
		return fmt.Errorf("read session file: %w", err)
	}

	var data SessionFile
	if err := json.Unmarshal(content, &data); err != nil {
		return fmt.Errorf("decode session file: %w", err)
	}
	if data.Meta.SessionKey == "" {
		data.Meta.SessionKey = session.id
	}
	if data.Meta.SenderID == "" {
		data.Meta.SenderID = session.senderID
	}
	if data.Messages == nil {
		data.Messages = []openai.ChatCompletionMessage{}
	}

	session.data = data
	session.loaded = true
	return nil
}

func (session *fileSession) writeSessionFileLocked() error {
	return session.writeSnapshot(session.snapshotLocked())
}

func (session *fileSession) archiveSnapshotLocked(snapshot SessionFile) (string, error) {
	archiveDir := filepath.Join(filepath.Dir(session.filePath), "achrive")
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		return "", fmt.Errorf("create achrive directory: %w", err)
	}
	archivePath := filepath.Join(
		archiveDir,
		filepath.Base(session.filePath)+"_achrive_"+strconv.FormatInt(sessionNow().Unix(), 10),
	)
	if err := session.writeSnapshotToPath(snapshot, archivePath); err != nil {
		return "", err
	}
	return archivePath, nil
}

func (session *fileSession) snapshotLocked() SessionFile {
	snapshot := SessionFile{
		Meta: SessionMeta{
			SessionKey: session.data.Meta.SessionKey,
			SenderID:   session.data.Meta.SenderID,
		},
		Messages: cloneMessages(session.data.Messages),
	}
	if snapshot.Meta.SessionKey == "" {
		snapshot.Meta.SessionKey = session.id
	}
	if snapshot.Meta.SenderID == "" {
		snapshot.Meta.SenderID = session.senderID
	}
	if snapshot.Messages == nil {
		snapshot.Messages = []openai.ChatCompletionMessage{}
	}

	return snapshot
}

func (session *fileSession) writeSnapshot(snapshot SessionFile) error {
	return session.writeSnapshotToPath(snapshot, session.filePath)
}

func (session *fileSession) writeSnapshotToPath(snapshot SessionFile, targetPath string) error {
	encoded, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("encode session file: %w", err)
	}
	if err := os.WriteFile(targetPath, encoded, 0644); err != nil {
		return fmt.Errorf("write session file: %w", err)
	}

	return nil
}

func (session *fileSession) scheduleFlushLocked() {
	session.flushRequested = true
	if session.flushRunning {
		return
	}

	session.flushRunning = true
	go session.flushLoop()
}

func (session *fileSession) flushLoop() {
	for {
		session.mu.Lock()
		if !session.flushRequested {
			session.flushRunning = false
			session.mu.Unlock()
			return
		}
		snapshot := session.snapshotLocked()
		session.flushRequested = false
		session.mu.Unlock()

		err := session.writeSnapshot(snapshot)

		session.mu.Lock()
		session.lastWriteErr = err
		session.mu.Unlock()
	}
}

func cloneMessages(messages []openai.ChatCompletionMessage) []openai.ChatCompletionMessage {
	clonedMessages := make([]openai.ChatCompletionMessage, 0, len(messages))
	for _, message := range messages {
		var functionCall *openai.FunctionCall
		if message.FunctionCall != nil {
			copied := *message.FunctionCall
			functionCall = &copied
		}

		toolCalls := make([]openai.ToolCall, 0, len(message.ToolCalls))
		for _, toolCall := range message.ToolCalls {
			toolCalls = append(toolCalls, toolCall)
		}

		clonedMessages = append(clonedMessages, openai.ChatCompletionMessage{
			Role:         message.Role,
			Content:      message.Content,
			Name:         message.Name,
			Refusal:      message.Refusal,
			FunctionCall: functionCall,
			ToolCalls:    toolCalls,
			ToolCallID:   message.ToolCallID,
		})
	}

	return clonedMessages
}

func MakeSessionID(channelID string, chatID string) string {
	return channelID + ":" + chatID
}
