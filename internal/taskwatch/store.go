package taskwatch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	watchDirName = ".gogoclaw/task_watches"
	watchFileExt = ".json"
)

// WatchStatus tracks the lifecycle state of a task watch.
type WatchStatus string

const (
	WatchStatusActive    WatchStatus = "active"
	WatchStatusCompleted WatchStatus = "completed"
	WatchStatusTimeout   WatchStatus = "timeout"
)

// WatchEntry is a persistent record that tells the TaskWatch scanner
// to monitor a specific invocation directory for completion.
type WatchEntry struct {
	InvocationID  string            `json:"invocation_id"`
	InvocationDir string            `json:"invocation_dir"`
	CallerProfile string            `json:"caller_profile"`
	TargetProfile string            `json:"target_profile"`
	TaskCronID    string            `json:"task_cron_id,omitempty"`
	CheckInterval Duration          `json:"check_interval"`
	Timeout       Duration          `json:"timeout"`
	CreatedAt     time.Time         `json:"created_at"`
	LastCheckedAt time.Time         `json:"last_checked_at"`
	Status        WatchStatus       `json:"status"`
	ReturnRouting map[string]string `json:"return_routing,omitempty"`
}

// Duration wraps time.Duration for clean JSON marshalling as seconds.
type Duration time.Duration

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).Seconds())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var seconds float64
	if err := json.Unmarshal(b, &seconds); err != nil {
		return err
	}
	*d = Duration(time.Duration(seconds * float64(time.Second)))
	return nil
}

// Store handles persistent read/write of watch entries on disk.
type Store struct {
	dir string
}

func NewStore(workspace string) *Store {
	return &Store{dir: filepath.Join(workspace, watchDirName)}
}

func (s *Store) ensureDir() error {
	return os.MkdirAll(s.dir, 0o755)
}

// Put writes or overwrites a watch entry.
func (s *Store) Put(entry WatchEntry) error {
	if err := s.ensureDir(); err != nil {
		return fmt.Errorf("create task watch directory: %w", err)
	}
	path := s.pathFor(entry.InvocationID)
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal watch entry %s: %w", entry.InvocationID, err)
	}
	return os.WriteFile(path, data, 0o644)
}

// Get reads a single watch entry by invocation ID.
func (s *Store) Get(invocationID string) (*WatchEntry, error) {
	path := s.pathFor(invocationID)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var entry WatchEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("unmarshal watch entry %s: %w", invocationID, err)
	}
	return &entry, nil
}

// Delete removes a watch entry file.
func (s *Store) Delete(invocationID string) error {
	path := s.pathFor(invocationID)
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ListActive returns all watch entries with Status == WatchStatusActive.
func (s *Store) ListActive() ([]WatchEntry, error) {
	return s.list(func(e WatchEntry) bool { return e.Status == WatchStatusActive })
}

// ListAll returns every watch entry on disk.
func (s *Store) ListAll() ([]WatchEntry, error) {
	return s.list(func(WatchEntry) bool { return true })
}

func (s *Store) list(predicate func(WatchEntry) bool) ([]WatchEntry, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var result []WatchEntry
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), watchFileExt) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			continue
		}
		var w WatchEntry
		if err := json.Unmarshal(data, &w); err != nil {
			continue
		}
		if predicate(w) {
			result = append(result, w)
		}
	}
	return result, nil
}

func (s *Store) pathFor(invocationID string) string {
	return filepath.Join(s.dir, invocationID+watchFileExt)
}
