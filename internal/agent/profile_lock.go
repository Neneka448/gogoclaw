package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

const profileLockDirName = ".gogoclaw/profile_locks"

type profileRuntimeLock struct {
	file        *os.File
	path        string
	profileName string
	workspace   string
	releaseOnce sync.Once
}

func acquireProfileRuntimeLock(workspace string, profileName string) (*profileRuntimeLock, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return nil, fmt.Errorf("workspace path is required for profile %q", profileName)
	}

	lockDir := filepath.Join(workspace, profileLockDirName)
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return nil, fmt.Errorf("create profile lock directory: %w", err)
	}

	lockPath := filepath.Join(lockDir, fmt.Sprintf("%s.lock", normalizeProfileLockName(profileName)))
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open profile lock file: %w", err)
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, fmt.Errorf("profile %q is already active in workspace %q", profileName, workspace)
		}
		return nil, fmt.Errorf("acquire profile lock: %w", err)
	}

	return &profileRuntimeLock{
		file:        file,
		path:        lockPath,
		profileName: profileName,
		workspace:   workspace,
	}, nil
}

func (lock *profileRuntimeLock) release() error {
	if lock == nil || lock.file == nil {
		return nil
	}

	var releaseErr error
	lock.releaseOnce.Do(func() {
		if err := unix.Flock(int(lock.file.Fd()), unix.LOCK_UN); err != nil {
			releaseErr = fmt.Errorf("unlock profile %q in workspace %q: %w", lock.profileName, lock.workspace, err)
		}
		if err := lock.file.Close(); err != nil && releaseErr == nil {
			releaseErr = fmt.Errorf("close profile lock %s: %w", lock.path, err)
		}
		lock.file = nil
	})
	return releaseErr
}

func normalizeProfileLockName(profileName string) string {
	runes := make([]rune, 0, len(profileName))
	for _, r := range profileName {
		switch {
		case r >= 'a' && r <= 'z':
			runes = append(runes, r)
		case r >= 'A' && r <= 'Z':
			runes = append(runes, r)
		case r >= '0' && r <= '9':
			runes = append(runes, r)
		case r == '.', r == '_', r == '-':
			runes = append(runes, r)
		default:
			runes = append(runes, '_')
		}
	}
	if len(runes) == 0 {
		return defaultAgentProfileName
	}
	return string(runes)
}
