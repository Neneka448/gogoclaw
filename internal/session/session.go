package session

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/Neneka448/gogoclaw/internal/utils"
	openai "github.com/sashabaranov/go-openai"
)

var sessionNow = time.Now

const (
	// ArchiveDirName preserves the current on-disk archive directory spelling for compatibility.
	ArchiveDirName = "achrive"
	// ArchiveFileSuffixToken preserves the current on-disk archive filename token for compatibility.
	ArchiveFileSuffixToken = "_achrive_"
)

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
	UpdateMetadata(channel string, sessionType string) error
	AppendMessage(message openai.ChatCompletionMessage) error
	AppendMessages(messages []openai.ChatCompletionMessage) error
	MarkMemoryIngested(digest string) error
	ArchiveAndReset() error
}

type SessionManager interface {
	GetOrCreateSession(sessionID string, senderID string) (Session, error)
	ListSessionIDs() ([]string, error)
	Close() error
}

type SessionFile struct {
	Revision uint64                         `json:"revision,omitempty"`
	Meta     SessionMeta                    `json:"meta"`
	Messages []openai.ChatCompletionMessage `json:"messages"`
}

type SessionMeta struct {
	SessionKey     string `json:"session_key"`
	SenderID       string `json:"sender_id"`
	Channel        string `json:"channel,omitempty"`
	Type           string `json:"type,omitempty"`
	IngestedDigest string `json:"ingested_digest,omitempty"`
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

func inferSessionChannel(sessionID string) string {
	normalized := strings.TrimSpace(sessionID)
	if normalized == "" {
		return ""
	}
	channel, _, found := strings.Cut(normalized, ":")
	if !found {
		return ""
	}
	return strings.TrimSpace(channel)
}

func MessagesDigest(messages []openai.ChatCompletionMessage) string {
	encoded, err := json.Marshal(utils.CloneMessages(messages))
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(encoded)
	return fmt.Sprintf("%x", sum)
}

func MakeSessionID(channelID string, chatID string) string {
	return channelID + ":" + chatID
}
