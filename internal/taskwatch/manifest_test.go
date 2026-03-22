package taskwatch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParseInvocationCronID(t *testing.T) {
	tests := []struct {
		cronID string
		want   string
	}{
		{"inv-20260322-abc123-task", "inv-20260322-abc123"},
		{"inv-20260322-abc123-heartbeat", ""},
		{"some-other-cron", ""},
		{"inv-task", "inv"},
		{"", ""},
		{"  inv-20260322-abc123-task  ", "inv-20260322-abc123"},
	}
	for _, tt := range tests {
		got := ParseInvocationCronID(tt.cronID)
		if got != tt.want {
			t.Errorf("ParseInvocationCronID(%q) = %q, want %q", tt.cronID, got, tt.want)
		}
	}
}

func TestResolveInvocationDir(t *testing.T) {
	workspace := t.TempDir()
	invID := "inv-20260322-abc123"
	dir := filepath.Join(workspace, "invocations", invID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Without manifest.json, should fail.
	if _, err := ResolveInvocationDir(workspace, invID); err == nil {
		t.Fatal("expected error without manifest.json")
	}

	// With manifest.json, should succeed.
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveInvocationDir(workspace, invID)
	if err != nil {
		t.Fatalf("ResolveInvocationDir() error = %v", err)
	}
	if got != dir {
		t.Fatalf("got %q, want %q", got, dir)
	}
}

func TestReadManifest(t *testing.T) {
	dir := t.TempDir()
	m := InvocationManifest{
		InvocationID:    "inv-20260322-abc123",
		CallerProfile:   "main",
		TargetProfile:   "worker",
		TaskCronID:      "inv-20260322-abc123-task",
		HeartbeatCronID: "inv-20260322-abc123-heartbeat",
		ReturnChannelID: "feishu",
		ReturnChatID:    "chat-999",
		ReturnSessionID: "feishu:chat-999",
	}
	data, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadManifest(dir)
	if err != nil {
		t.Fatalf("ReadManifest() error = %v", err)
	}
	if got.InvocationID != "inv-20260322-abc123" {
		t.Fatalf("InvocationID = %q", got.InvocationID)
	}
	if got.CallerProfile != "main" {
		t.Fatalf("CallerProfile = %q", got.CallerProfile)
	}
	if got.ReturnChannelID != "feishu" {
		t.Fatalf("ReturnChannelID = %q", got.ReturnChannelID)
	}
}

func TestWatchEntryFromManifest(t *testing.T) {
	m := &InvocationManifest{
		InvocationID:    "inv-001",
		CallerProfile:   "caller",
		TargetProfile:   "target",
		TaskCronID:      "inv-001-task",
		ReturnChannelID: "feishu",
		ReturnChatID:    "chat-1",
		ReturnSenderID:  "",
		ReturnSessionID: "feishu:chat-1",
	}
	entry := WatchEntryFromManifest(m, "/workspace/invocations/inv-001")

	if entry.InvocationID != "inv-001" {
		t.Fatalf("InvocationID = %q", entry.InvocationID)
	}
	if entry.CallerProfile != "caller" {
		t.Fatalf("CallerProfile = %q", entry.CallerProfile)
	}
	if entry.TaskCronID != "inv-001-task" {
		t.Fatalf("TaskCronID = %q", entry.TaskCronID)
	}
	if entry.ReturnRouting["channel_id"] != "feishu" {
		t.Fatalf("ReturnRouting[channel_id] = %q", entry.ReturnRouting["channel_id"])
	}
	if _, ok := entry.ReturnRouting["sender_id"]; ok {
		t.Fatal("empty sender_id should not be in ReturnRouting")
	}
	if entry.ReturnRouting["session_id"] != "feishu:chat-1" {
		t.Fatalf("ReturnRouting[session_id] = %q", entry.ReturnRouting["session_id"])
	}
}

func TestReadManifestMissingFile(t *testing.T) {
	_, err := ReadManifest(t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing manifest.json")
	}
}
