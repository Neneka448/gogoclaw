package utils

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAcquireFileLockWritesMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".lock")
	lock, err := AcquireFileLock(FileLockOptions{
		Path:     path,
		Resource: "profile:default",
		Metadata: map[string]string{"profile": "default"},
		Now: func() time.Time {
			return time.Date(2026, 3, 22, 1, 2, 3, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("AcquireFileLock() error = %v", err)
	}
	defer func() {
		if err := lock.Release(); err != nil {
			t.Fatalf("Release() error = %v", err)
		}
	}()

	info, err := ReadFileLockInfo(path)
	if err != nil {
		t.Fatalf("ReadFileLockInfo() error = %v", err)
	}
	if info == nil {
		t.Fatal("ReadFileLockInfo() = nil, want info")
	}
	if info.PID == 0 {
		t.Fatalf("info.PID = %d, want non-zero", info.PID)
	}
	if info.Resource != "profile:default" {
		t.Fatalf("info.Resource = %q, want profile:default", info.Resource)
	}
	if info.Metadata["profile"] != "default" {
		t.Fatalf("info.Metadata[profile] = %q, want default", info.Metadata["profile"])
	}
	if info.AcquiredAt != "2026-03-22T01:02:03Z" {
		t.Fatalf("info.AcquiredAt = %q, want 2026-03-22T01:02:03Z", info.AcquiredAt)
	}
}

func TestAcquireFileLockReturnsHolderInfoWhenHeld(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".lock")
	first, err := AcquireFileLock(FileLockOptions{
		Path:     path,
		Resource: "cron:demo",
		Metadata: map[string]string{"cron_id": "demo"},
	})
	if err != nil {
		t.Fatalf("first AcquireFileLock() error = %v", err)
	}
	defer func() {
		if err := first.Release(); err != nil {
			t.Fatalf("first Release() error = %v", err)
		}
	}()

	_, err = AcquireFileLock(FileLockOptions{Path: path, Resource: "cron:demo"})
	if err == nil {
		t.Fatal("second AcquireFileLock() error = nil, want lock held")
	}
	if !errors.Is(err, ErrFileLockHeld) {
		t.Fatalf("errors.Is(err, ErrFileLockHeld) = false, err = %v", err)
	}
	var heldErr *FileLockHeldError
	if !errors.As(err, &heldErr) {
		t.Fatalf("errors.As(err, *FileLockHeldError) = false, err = %T", err)
	}
	if heldErr.Info == nil || heldErr.Info.PID == 0 {
		t.Fatalf("heldErr.Info = %#v, want pid metadata", heldErr.Info)
	}
	if !strings.Contains(heldErr.Error(), "pid=") {
		t.Fatalf("heldErr.Error() = %q, want pid details", heldErr.Error())
	}
}