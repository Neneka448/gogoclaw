package session

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

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
	GetMemoryIngestedDigest() string
	AppendMessage(message openai.ChatCompletionMessage) error
	AppendMessages(messages []openai.ChatCompletionMessage) error
	MarkMemoryIngested(digest string) error
	ReadSessionFile() error
	WriteSessionFile() error
	GetSessionFilePath() string
	ArchiveAndReset() (string, error)
}

type SessionManager interface {
	GetOrCreateSession(sessionID string, senderID string) (Session, error)
	ListSessionIDs() ([]string, error)
	Close() error
}

type SessionFile struct {
	Meta     SessionMeta                    `json:"meta"`
	Messages []openai.ChatCompletionMessage `json:"messages"`
}

type SessionMeta struct {
	SessionKey     string `json:"session_key"`
	SenderID       string `json:"sender_id"`
	IngestedDigest string `json:"ingested_digest,omitempty"`
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

func ValidateSessionID(sessionID string) error {
	normalized := strings.TrimSpace(sessionID)
	if normalized == "" {
		return fmt.Errorf("session id cannot be empty")
	}
	if strings.ContainsAny(normalized, `/\`) {
		return fmt.Errorf("session id cannot contain path separators")
	}
	if normalized == "." || normalized == ".." {
		return fmt.Errorf("session id cannot be dot path segments")
	}
	for _, r := range normalized {
		if r == 0 || unicode.IsControl(r) {
			return fmt.Errorf("session id cannot contain control characters")
		}
	}
	return nil
}

func (manager *sessionManager) GetOrCreateSession(sessionID string, senderID string) (Session, error) {
	sessionID = strings.TrimSpace(sessionID)
	if err := ValidateSessionID(sessionID); err != nil {
		return nil, err
	}

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

func (manager *sessionManager) ListSessionIDs() ([]string, error) {
	sessionsDir := filepath.Join(manager.workspacePath, "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		return nil, fmt.Errorf("create sessions directory: %w", err)
	}

	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil, fmt.Errorf("read sessions directory: %w", err)
	}

	sessionIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		sessionIDs = append(sessionIDs, strings.TrimSuffix(name, ".json"))
	}
	sort.Strings(sessionIDs)
	return sessionIDs, nil
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

func (session *fileSession) GetMemoryIngestedDigest() string {
	session.mu.Lock()
	defer session.mu.Unlock()

	if err := session.ensureLoadedLocked(); err != nil {
		return ""
	}

	return session.data.Meta.IngestedDigest
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
	session.data.Meta.IngestedDigest = ""
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
	start = normalizeWindowStart(session.data.Messages, start)

	return cloneMessages(session.data.Messages[start:])
}

func normalizeWindowStart(messages []openai.ChatCompletionMessage, start int) int {
	if start <= 0 || start >= len(messages) {
		return start
	}
	if !isToolMessage(messages[start]) {
		return start
	}

	probe := start
	for probe > 0 && isToolMessage(messages[probe]) {
		probe--
	}
	if isAssistantToolCallMessage(messages[probe]) {
		return probe
	}

	return start
}

func isToolMessage(message openai.ChatCompletionMessage) bool {
	return message.Role == openai.ChatMessageRoleTool
}

func isAssistantToolCallMessage(message openai.ChatCompletionMessage) bool {
	if message.Role != openai.ChatMessageRoleAssistant {
		return false
	}
	return len(message.ToolCalls) > 0 || message.FunctionCall != nil
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

func (session *fileSession) MarkMemoryIngested(digest string) error {
	session.mu.Lock()
	defer session.mu.Unlock()

	if err := session.ensureLoadedLocked(); err != nil {
		return err
	}
	if session.data.Meta.IngestedDigest == digest {
		return nil
	}
	session.data.Meta.IngestedDigest = digest
	return session.writeSessionFileLocked()
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
			SessionKey:     session.data.Meta.SessionKey,
			SenderID:       session.data.Meta.SenderID,
			IngestedDigest: session.data.Meta.IngestedDigest,
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
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("create session file directory: %w", err)
	}
	tempFile, err := os.CreateTemp(filepath.Dir(targetPath), filepath.Base(targetPath)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp session file: %w", err)
	}
	tempPath := tempFile.Name()
	cleanup := func() {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
	}
	if _, err := tempFile.Write(encoded); err != nil {
		cleanup()
		return fmt.Errorf("write temp session file: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync temp session file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("close temp session file: %w", err)
	}
	if err := os.Chmod(tempPath, 0644); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("chmod temp session file: %w", err)
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("replace session file: %w", err)
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

func MessagesDigest(messages []openai.ChatCompletionMessage) string {
	encoded, err := json.Marshal(cloneMessages(messages))
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(encoded)
	return fmt.Sprintf("%x", sum)
}

func MakeSessionID(channelID string, chatID string) string {
	return channelID + ":" + chatID
}
