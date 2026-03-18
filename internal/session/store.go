package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Neneka448/gogoclaw/internal/utils"
	openai "github.com/sashabaranov/go-openai"
)

type sessionStore interface {
	Load(sessionID string, senderID string) (SessionFile, error)
	Save(sessionID string, snapshot SessionFile) error
	AppendMessages(sessionID string, revision uint64, messages []openai.ChatCompletionMessage) error
	SaveMetadata(sessionID string, revision uint64, meta SessionMeta) error
	Archive(sessionID string, snapshot SessionFile) error
	ListSessionIDs() ([]string, error)
}

const sessionWALFileSuffix = ".wal"

const (
	sessionLogSetMetadata   = "set_metadata"
	sessionLogAppendMessage = "append_messages"
)

type sessionLogEntry struct {
	Revision uint64                         `json:"revision"`
	Op       string                         `json:"op"`
	Meta     *SessionMeta                   `json:"meta,omitempty"`
	Messages []openai.ChatCompletionMessage `json:"messages,omitempty"`
}

type fileSessionStore struct {
	workspacePath string
}

// fileSessionStore keeps a readable snapshot on disk while routing high-frequency
// session mutations through an append-only WAL to reduce write amplification.
func newFileSessionStore(workspacePath string) sessionStore {
	return &fileSessionStore{workspacePath: workspacePath}
}

func (store *fileSessionStore) Load(sessionID string, senderID string) (SessionFile, error) {
	if err := ensureDir(store.sessionsDir()); err != nil {
		return SessionFile{}, fmt.Errorf("create sessions directory: %w", err)
	}

	targetPath := store.sessionFilePath(sessionID)
	snapshot, exists, err := readSnapshotFromPath(targetPath)
	if err != nil {
		return SessionFile{}, err
	}
	if !exists {
		snapshot = newSessionState(sessionID, senderID).snapshot()
	}

	if err := store.ensureWAL(sessionID); err != nil {
		return SessionFile{}, err
	}
	if err := store.replayWAL(sessionID, &snapshot); err != nil {
		return SessionFile{}, err
	}
	snapshot = newSessionStateFromSnapshot(sessionID, senderID, snapshot).snapshot()
	if !exists {
		if err := store.Save(sessionID, snapshot); err != nil {
			return SessionFile{}, err
		}
	}
	return snapshot, nil
}

func (store *fileSessionStore) Save(sessionID string, snapshot SessionFile) error {
	if err := writeSnapshotToPath(snapshot, store.sessionFilePath(sessionID)); err != nil {
		return err
	}
	store.truncateWAL(sessionID)
	return nil
}

func (store *fileSessionStore) AppendMessages(sessionID string, revision uint64, messages []openai.ChatCompletionMessage) error {
	if len(messages) == 0 {
		return nil
	}
	return store.appendWALEntry(sessionID, sessionLogEntry{
		Revision: revision,
		Op:       sessionLogAppendMessage,
		Messages: messages,
	})
}

func (store *fileSessionStore) SaveMetadata(sessionID string, revision uint64, meta SessionMeta) error {
	metaCopy := meta
	return store.appendWALEntry(sessionID, sessionLogEntry{
		Revision: revision,
		Op:       sessionLogSetMetadata,
		Meta:     &metaCopy,
	})
}

func (store *fileSessionStore) Archive(sessionID string, snapshot SessionFile) error {
	archiveDir := filepath.Join(store.sessionsDir(), ArchiveDirName)
	if err := ensureDir(archiveDir); err != nil {
		return fmt.Errorf("create %s directory: %w", ArchiveDirName, err)
	}

	archivePath := filepath.Join(
		archiveDir,
		sessionID+".json"+ArchiveFileSuffixToken+strconv.FormatInt(sessionNow().Unix(), 10),
	)
	return writeSnapshotToPath(snapshot, archivePath)
}

func (store *fileSessionStore) ListSessionIDs() ([]string, error) {
	if err := ensureDir(store.sessionsDir()); err != nil {
		return nil, fmt.Errorf("create sessions directory: %w", err)
	}

	entries, err := os.ReadDir(store.sessionsDir())
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

func (store *fileSessionStore) sessionsDir() string {
	return filepath.Join(store.workspacePath, "sessions")
}

func (store *fileSessionStore) sessionFilePath(sessionID string) string {
	return filepath.Join(store.sessionsDir(), sessionID+".json")
}

func (store *fileSessionStore) walFilePath(sessionID string) string {
	return store.sessionFilePath(sessionID) + sessionWALFileSuffix
}

func (store *fileSessionStore) ensureWAL(sessionID string) error {
	walPath := store.walFilePath(sessionID)
	if _, err := os.Stat(walPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat session wal file: %w", err)
	}

	file, err := os.OpenFile(walPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("create session wal file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close session wal file: %w", err)
	}
	if err := syncDir(filepath.Dir(walPath)); err != nil {
		return fmt.Errorf("sync session wal directory: %w", err)
	}
	return nil
}

func (store *fileSessionStore) replayWAL(sessionID string, snapshot *SessionFile) error {
	file, err := os.Open(store.walFilePath(sessionID))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open session wal file: %w", err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	for {
		line, readErr := reader.ReadBytes('\n')
		if readErr != nil && readErr != io.EOF {
			return fmt.Errorf("read session wal file: %w", readErr)
		}
		trimmed := strings.TrimSpace(string(line))
		if trimmed != "" {
			var entry sessionLogEntry
			if err := json.Unmarshal([]byte(trimmed), &entry); err != nil {
				if readErr == io.EOF {
					break
				}
				return fmt.Errorf("decode session wal entry: %w", err)
			}
			if err := applyWALEntry(snapshot, entry); err != nil {
				return err
			}
		}
		if readErr == io.EOF {
			break
		}
	}

	return nil
}

func (store *fileSessionStore) appendWALEntry(sessionID string, entry sessionLogEntry) error {
	if err := store.ensureWAL(sessionID); err != nil {
		return err
	}

	encoded, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encode session wal entry: %w", err)
	}
	encoded = append(encoded, '\n')

	file, err := os.OpenFile(store.walFilePath(sessionID), os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open session wal file: %w", err)
	}
	if _, err := file.Write(encoded); err != nil {
		_ = file.Close()
		return fmt.Errorf("write session wal entry: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync session wal file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close session wal file: %w", err)
	}
	return nil
}

func (store *fileSessionStore) truncateWAL(sessionID string) {
	file, err := os.OpenFile(store.walFilePath(sessionID), os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return
	}
	_ = file.Close()
}

func applyWALEntry(snapshot *SessionFile, entry sessionLogEntry) error {
	if entry.Revision == 0 || entry.Revision <= snapshot.Revision {
		return nil
	}

	switch entry.Op {
	case sessionLogSetMetadata:
		if entry.Meta == nil {
			return fmt.Errorf("decode session wal entry: missing metadata payload")
		}
		snapshot.Meta = *entry.Meta
	case sessionLogAppendMessage:
		snapshot.Messages = append(snapshot.Messages, utils.CloneMessages(entry.Messages)...)
	default:
		return fmt.Errorf("decode session wal entry: unknown operation %q", entry.Op)
	}
	snapshot.Revision = entry.Revision
	return nil
}

func readSnapshotFromPath(targetPath string) (SessionFile, bool, error) {
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		return SessionFile{}, false, nil
	} else if err != nil {
		return SessionFile{}, false, fmt.Errorf("stat session file: %w", err)
	}

	content, err := os.ReadFile(targetPath)
	if err != nil {
		return SessionFile{}, false, fmt.Errorf("read session file: %w", err)
	}

	var snapshot SessionFile
	if err := json.Unmarshal(content, &snapshot); err != nil {
		return SessionFile{}, false, fmt.Errorf("decode session file: %w", err)
	}

	return snapshot, true, nil
}

func writeSnapshotToPath(snapshot SessionFile, targetPath string) error {
	encoded, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("encode session file: %w", err)
	}
	if err := ensureDir(filepath.Dir(targetPath)); err != nil {
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
	if err := syncDir(filepath.Dir(targetPath)); err != nil {
		return fmt.Errorf("sync session file directory: %w", err)
	}

	return nil
}

func ensureDir(path string) error {
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("%s is not a directory", path)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
