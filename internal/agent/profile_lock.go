package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Neneka448/gogoclaw/internal/utils"
)

const profileLockDirName = ".gogoclaw/profile_locks"

type profileRuntimeLock struct {
	lock        *utils.FileLock
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
	lock, err := utils.AcquireFileLock(utils.FileLockOptions{
		Path:     lockPath,
		Resource: fmt.Sprintf("profile:%s", profileName),
		Metadata: map[string]string{
			"profile":   profileName,
			"workspace": workspace,
		},
	})
	if err != nil {
		var heldErr *utils.FileLockHeldError
		if errors.As(err, &heldErr) {
			return nil, fmt.Errorf("profile %q is already active in workspace %q%s", profileName, workspace, utils.FormatFileLockInfo(heldErr.Info))
		}
		return nil, fmt.Errorf("acquire profile lock: %w", err)
	}

	return &profileRuntimeLock{
		lock:        lock,
		path:        lockPath,
		profileName: profileName,
		workspace:   workspace,
	}, nil
}

func (lock *profileRuntimeLock) release() error {
	if lock == nil || lock.lock == nil {
		return nil
	}

	var releaseErr error
	lock.releaseOnce.Do(func() {
		if err := lock.lock.Release(); err != nil {
			releaseErr = fmt.Errorf("release profile %q lock in workspace %q: %w", lock.profileName, lock.workspace, err)
		}
		lock.lock = nil
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
