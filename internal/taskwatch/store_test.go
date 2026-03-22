package taskwatch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStorePutAndGet(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)

	entry := WatchEntry{
		InvocationID:  "inv-test-001",
		InvocationDir: filepath.Join(workspace, "invocations", "inv-test-001"),
		CallerProfile: "caller",
		TargetProfile: "target",
		CheckInterval: Duration(60 * time.Second),
		Timeout:       Duration(3600 * time.Second),
		CreatedAt:     time.Now().UTC().Truncate(time.Millisecond),
		LastCheckedAt: time.Now().UTC().Truncate(time.Millisecond),
		Status:        WatchStatusActive,
	}

	if err := store.Put(entry); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	got, err := store.Get("inv-test-001")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil {
		t.Fatal("Get() returned nil")
	}
	if got.InvocationID != "inv-test-001" {
		t.Fatalf("InvocationID = %q, want inv-test-001", got.InvocationID)
	}
	if got.Status != WatchStatusActive {
		t.Fatalf("Status = %q, want active", got.Status)
	}
}

func TestStoreGetNonExistent(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)

	got, err := store.Get("does-not-exist")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != nil {
		t.Fatalf("Get() = %v, want nil", got)
	}
}

func TestStoreListActive(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)

	active := WatchEntry{
		InvocationID:  "inv-active",
		InvocationDir: "/tmp/a",
		CallerProfile: "caller",
		Status:        WatchStatusActive,
		CheckInterval: Duration(60 * time.Second),
		Timeout:       Duration(3600 * time.Second),
		CreatedAt:     time.Now().UTC(),
		LastCheckedAt: time.Now().UTC(),
	}
	completed := WatchEntry{
		InvocationID:  "inv-completed",
		InvocationDir: "/tmp/b",
		CallerProfile: "caller",
		Status:        WatchStatusCompleted,
		CheckInterval: Duration(60 * time.Second),
		Timeout:       Duration(3600 * time.Second),
		CreatedAt:     time.Now().UTC(),
		LastCheckedAt: time.Now().UTC(),
	}

	if err := store.Put(active); err != nil {
		t.Fatalf("Put(active) error = %v", err)
	}
	if err := store.Put(completed); err != nil {
		t.Fatalf("Put(completed) error = %v", err)
	}

	entries, err := store.ListActive()
	if err != nil {
		t.Fatalf("ListActive() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("ListActive() returned %d entries, want 1", len(entries))
	}
	if entries[0].InvocationID != "inv-active" {
		t.Fatalf("entry ID = %q, want inv-active", entries[0].InvocationID)
	}
}

func TestStoreDelete(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)

	entry := WatchEntry{
		InvocationID:  "inv-delete",
		InvocationDir: "/tmp/d",
		CallerProfile: "caller",
		Status:        WatchStatusActive,
		CheckInterval: Duration(60 * time.Second),
		Timeout:       Duration(3600 * time.Second),
		CreatedAt:     time.Now().UTC(),
		LastCheckedAt: time.Now().UTC(),
	}
	if err := store.Put(entry); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if err := store.Delete("inv-delete"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	got, err := store.Get("inv-delete")
	if err != nil {
		t.Fatalf("Get() after delete error = %v", err)
	}
	if got != nil {
		t.Fatal("Get() after delete returned non-nil")
	}
}

func TestStoreListActiveEmptyDir(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)

	entries, err := store.ListActive()
	if err != nil {
		t.Fatalf("ListActive() error = %v", err)
	}
	if entries != nil {
		t.Fatalf("ListActive() = %v, want nil for missing dir", entries)
	}
}

func TestDurationJSONRoundTrip(t *testing.T) {
	d := Duration(90 * time.Second)
	data, err := d.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}
	var d2 Duration
	if err := d2.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}
	if time.Duration(d2) != 90*time.Second {
		t.Fatalf("round-trip = %v, want 90s", time.Duration(d2))
	}
}

func TestStoreIgnoresNonJSONFiles(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)

	if err := store.ensureDir(); err != nil {
		t.Fatalf("ensureDir() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(store.dir, "readme.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	entries, err := store.ListAll()
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("ListAll() = %d entries, want 0", len(entries))
	}
}
