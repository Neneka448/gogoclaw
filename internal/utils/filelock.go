package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

var ErrFileLockHeld = errors.New("file lock is already held")

type FileLockOptions struct {
	Path     string
	Resource string
	Metadata map[string]string
	Now      func() time.Time
}

type FileLockInfo struct {
	PID        int               `json:"pid"`
	Hostname   string            `json:"hostname,omitempty"`
	AcquiredAt string            `json:"acquired_at,omitempty"`
	Resource   string            `json:"resource,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type FileLockHeldError struct {
	Path string
	Info *FileLockInfo
}

func (err *FileLockHeldError) Error() string {
	if err == nil {
		return ErrFileLockHeld.Error()
	}
	if details := FormatFileLockInfo(err.Info); details != "" {
		return fmt.Sprintf("%s: %s%s", ErrFileLockHeld.Error(), err.Path, details)
	}
	return fmt.Sprintf("%s: %s", ErrFileLockHeld.Error(), err.Path)
}

func (err *FileLockHeldError) Unwrap() error {
	return ErrFileLockHeld
}

type FileLock struct {
	file        *os.File
	path        string
	info        FileLockInfo
	releaseOnce sync.Once
}

func AcquireFileLock(options FileLockOptions) (*FileLock, error) {
	lockPath := strings.TrimSpace(options.Path)
	if lockPath == "" {
		return nil, fmt.Errorf("file lock path is required")
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, fmt.Errorf("create lock directory: %w", err)
	}

	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			info, _ := ReadFileLockInfo(lockPath)
			return nil, &FileLockHeldError{Path: lockPath, Info: info}
		}
		return nil, fmt.Errorf("acquire file lock %s: %w", lockPath, err)
	}

	now := time.Now
	if options.Now != nil {
		now = options.Now
	}
	hostname, hostnameErr := os.Hostname()
	if hostnameErr != nil {
		hostname = ""
	}
	info := FileLockInfo{
		PID:        os.Getpid(),
		Hostname:   strings.TrimSpace(hostname),
		AcquiredAt: now().UTC().Format(time.RFC3339),
		Resource:   strings.TrimSpace(options.Resource),
		Metadata:   cloneStringMap(options.Metadata),
	}
	if err := writeFileLockInfo(file, info); err != nil {
		_ = unix.Flock(int(file.Fd()), unix.LOCK_UN)
		_ = file.Close()
		return nil, fmt.Errorf("write lock metadata %s: %w", lockPath, err)
	}

	return &FileLock{file: file, path: lockPath, info: info}, nil
}

func (lock *FileLock) Release() error {
	if lock == nil || lock.file == nil {
		return nil
	}

	var releaseErr error
	lock.releaseOnce.Do(func() {
		if err := unix.Flock(int(lock.file.Fd()), unix.LOCK_UN); err != nil {
			releaseErr = fmt.Errorf("unlock file lock %s: %w", lock.path, err)
		}
		if err := lock.file.Close(); err != nil && releaseErr == nil {
			releaseErr = fmt.Errorf("close file lock %s: %w", lock.path, err)
		}
		lock.file = nil
	})
	return releaseErr
}

func (lock *FileLock) Info() FileLockInfo {
	if lock == nil {
		return FileLockInfo{}
	}
	return lock.info
}

func ReadFileLockInfo(path string) (*FileLockInfo, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	content = []byte(strings.TrimSpace(string(content)))
	if len(content) == 0 {
		return nil, nil
	}
	var info FileLockInfo
	if err := json.Unmarshal(content, &info); err != nil {
		return nil, err
	}
	if len(info.Metadata) == 0 {
		info.Metadata = nil
	}
	return &info, nil
}

func FormatFileLockInfo(info *FileLockInfo) string {
	if info == nil {
		return ""
	}
	parts := make([]string, 0, 4)
	if info.PID > 0 {
		parts = append(parts, fmt.Sprintf("pid=%d", info.PID))
	}
	if info.Hostname != "" {
		parts = append(parts, fmt.Sprintf("host=%s", info.Hostname))
	}
	if info.AcquiredAt != "" {
		parts = append(parts, fmt.Sprintf("acquired_at=%s", info.AcquiredAt))
	}
	if info.Resource != "" {
		parts = append(parts, fmt.Sprintf("resource=%s", info.Resource))
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

func writeFileLockInfo(file *os.File, info FileLockInfo) error {
	if file == nil {
		return fmt.Errorf("lock file is nil")
	}
	payload, err := json.Marshal(info)
	if err != nil {
		return err
	}
	if err := file.Truncate(0); err != nil {
		return err
	}
	if _, err := file.Seek(0, 0); err != nil {
		return err
	}
	if _, err := file.Write(payload); err != nil {
		return err
	}
	return file.Sync()
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}