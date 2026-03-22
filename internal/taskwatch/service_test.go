package taskwatch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
)

func TestServiceRegisterAndList(t *testing.T) {
	workspace := t.TempDir()
	bus := messagebus.NewMessageBus()
	defer bus.Close()

	svc := NewService(Options{
		Workspace:  workspace,
		MessageBus: bus,
	})

	entry := WatchEntry{
		InvocationID:  "inv-reg-001",
		InvocationDir: filepath.Join(workspace, "invocations", "inv-reg-001"),
		CallerProfile: "caller",
		TargetProfile: "target",
	}

	if err := svc.Register(entry); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	entries, err := svc.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("List() = %d entries, want 1", len(entries))
	}
	if entries[0].Status != WatchStatusActive {
		t.Fatalf("Status = %q, want active", entries[0].Status)
	}
	if time.Duration(entries[0].CheckInterval) != DefaultCheckInterval {
		t.Fatalf("CheckInterval = %v, want %v", time.Duration(entries[0].CheckInterval), DefaultCheckInterval)
	}
}

func TestServiceRegisterValidation(t *testing.T) {
	workspace := t.TempDir()
	bus := messagebus.NewMessageBus()
	defer bus.Close()

	svc := NewService(Options{Workspace: workspace, MessageBus: bus})

	tests := []struct {
		name  string
		entry WatchEntry
	}{
		{"missing invocation_id", WatchEntry{InvocationDir: "/x", CallerProfile: "c"}},
		{"missing invocation_dir", WatchEntry{InvocationID: "x", CallerProfile: "c"}},
		{"missing caller_profile", WatchEntry{InvocationID: "x", InvocationDir: "/x"}},
	}
	for _, tt := range tests {
		if err := svc.Register(tt.entry); err == nil {
			t.Fatalf("Register(%s) error = nil, want validation error", tt.name)
		}
	}
}

func TestServiceUnregister(t *testing.T) {
	workspace := t.TempDir()
	bus := messagebus.NewMessageBus()
	defer bus.Close()

	svc := NewService(Options{Workspace: workspace, MessageBus: bus})

	entry := WatchEntry{
		InvocationID:  "inv-unreg-001",
		InvocationDir: "/tmp/x",
		CallerProfile: "caller",
		TargetProfile: "target",
	}
	if err := svc.Register(entry); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := svc.Unregister("inv-unreg-001"); err != nil {
		t.Fatalf("Unregister() error = %v", err)
	}

	store := NewStore(workspace)
	got, _ := store.Get("inv-unreg-001")
	if got == nil || got.Status != WatchStatusCompleted {
		t.Fatalf("after Unregister: status = %v, want completed", got)
	}
}

func TestServiceScanDetectsCompletion(t *testing.T) {
	workspace := t.TempDir()
	bus := messagebus.NewMessageBus()
	defer bus.Close()

	invocationDir := filepath.Join(workspace, "invocations", "inv-scan-001")
	if err := os.MkdirAll(invocationDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	statusData, _ := json.Marshal(InvocationStatus{Status: "succeeded"})
	if err := os.WriteFile(filepath.Join(invocationDir, "status.json"), statusData, 0o644); err != nil {
		t.Fatalf("WriteFile(status.json) error = %v", err)
	}

	svc := NewService(Options{Workspace: workspace, MessageBus: bus})

	entry := WatchEntry{
		InvocationID:  "inv-scan-001",
		InvocationDir: invocationDir,
		CallerProfile: "caller",
		TargetProfile: "target",
		CheckInterval: Duration(1 * time.Millisecond),
		CreatedAt:     time.Now().UTC().Add(-time.Minute),
		LastCheckedAt: time.Now().UTC().Add(-time.Minute),
	}
	if err := svc.Register(entry); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Run scan directly.
	impl := svc.(*service)
	impl.scan()

	// Verify a completion message was injected.
	inbound, err := bus.Get(messagebus.InboundQueue)
	if err != nil {
		t.Fatalf("Get(InboundQueue) error = %v", err)
	}

	select {
	case msg := <-inbound:
		if msg.Metadata["message_kind"] != "task_completion" {
			t.Fatalf("message_kind = %q, want task_completion", msg.Metadata["message_kind"])
		}
		if msg.Metadata["invocation_id"] != "inv-scan-001" {
			t.Fatalf("invocation_id = %q, want inv-scan-001", msg.Metadata["invocation_id"])
		}
		if msg.Metadata["agent_profile"] != "caller" {
			t.Fatalf("agent_profile = %q, want caller", msg.Metadata["agent_profile"])
		}
	case <-time.After(time.Second):
		t.Fatal("no completion message within 1s")
	}

	// Verify watch entry was marked completed.
	store := NewStore(workspace)
	got, _ := store.Get("inv-scan-001")
	if got == nil || got.Status != WatchStatusCompleted {
		t.Fatalf("entry status = %v, want completed", got)
	}
}

func TestServiceScanDetectsTimeout(t *testing.T) {
	workspace := t.TempDir()
	bus := messagebus.NewMessageBus()
	defer bus.Close()

	invocationDir := filepath.Join(workspace, "invocations", "inv-timeout-001")
	if err := os.MkdirAll(invocationDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	statusData, _ := json.Marshal(InvocationStatus{Status: "running"})
	if err := os.WriteFile(filepath.Join(invocationDir, "status.json"), statusData, 0o644); err != nil {
		t.Fatalf("WriteFile(status.json) error = %v", err)
	}

	svc := NewService(Options{Workspace: workspace, MessageBus: bus})

	entry := WatchEntry{
		InvocationID:  "inv-timeout-001",
		InvocationDir: invocationDir,
		CallerProfile: "caller",
		TargetProfile: "target",
		CheckInterval: Duration(1 * time.Millisecond),
		Timeout:       Duration(1 * time.Millisecond),
		CreatedAt:     time.Now().UTC().Add(-time.Hour),
		LastCheckedAt: time.Now().UTC().Add(-time.Minute),
	}
	if err := svc.Register(entry); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	impl := svc.(*service)
	impl.scan()

	inbound, err := bus.Get(messagebus.InboundQueue)
	if err != nil {
		t.Fatalf("Get(InboundQueue) error = %v", err)
	}

	select {
	case msg := <-inbound:
		if msg.Metadata["message_kind"] != "task_timeout" {
			t.Fatalf("message_kind = %q, want task_timeout", msg.Metadata["message_kind"])
		}
	case <-time.After(time.Second):
		t.Fatal("no timeout message within 1s")
	}

	store := NewStore(workspace)
	got, _ := store.Get("inv-timeout-001")
	if got == nil || got.Status != WatchStatusTimeout {
		t.Fatalf("entry status = %v, want timeout", got)
	}
}

func TestServiceScanSkipsUntilCheckInterval(t *testing.T) {
	workspace := t.TempDir()
	bus := messagebus.NewMessageBus()
	defer bus.Close()

	invocationDir := filepath.Join(workspace, "invocations", "inv-skip-001")
	if err := os.MkdirAll(invocationDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	statusData, _ := json.Marshal(InvocationStatus{Status: "succeeded"})
	if err := os.WriteFile(filepath.Join(invocationDir, "status.json"), statusData, 0o644); err != nil {
		t.Fatalf("WriteFile(status.json) error = %v", err)
	}

	svc := NewService(Options{Workspace: workspace, MessageBus: bus})

	entry := WatchEntry{
		InvocationID:  "inv-skip-001",
		InvocationDir: invocationDir,
		CallerProfile: "caller",
		TargetProfile: "target",
		CheckInterval: Duration(10 * time.Minute),
		LastCheckedAt: time.Now().UTC(),
	}
	if err := svc.Register(entry); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	impl := svc.(*service)
	impl.scan()

	// Entry should NOT be completed yet because check interval hasn't passed.
	store := NewStore(workspace)
	got, _ := store.Get("inv-skip-001")
	if got == nil || got.Status != WatchStatusActive {
		t.Fatalf("entry status = %v, want active (skipped due to check interval)", got)
	}
}

func TestServiceStartStop(t *testing.T) {
	workspace := t.TempDir()
	bus := messagebus.NewMessageBus()
	defer bus.Close()

	svc := NewService(Options{Workspace: workspace, MessageBus: bus})

	if err := svc.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	// Double start should be safe.
	if err := svc.Start(); err != nil {
		t.Fatalf("second Start() error = %v", err)
	}
	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	// Double stop should be safe.
	if err := svc.Stop(); err != nil {
		t.Fatalf("second Stop() error = %v", err)
	}
}

func TestServiceScanHandlesMissingStatusJSON(t *testing.T) {
	workspace := t.TempDir()
	bus := messagebus.NewMessageBus()
	defer bus.Close()

	// Invocation dir exists but has no status.json.
	invocationDir := filepath.Join(workspace, "invocations", "inv-no-status")
	if err := os.MkdirAll(invocationDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	svc := NewService(Options{Workspace: workspace, MessageBus: bus})

	entry := WatchEntry{
		InvocationID:  "inv-no-status",
		InvocationDir: invocationDir,
		CallerProfile: "caller",
		TargetProfile: "target",
		CheckInterval: Duration(1 * time.Millisecond),
	}
	if err := svc.Register(entry); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	impl := svc.(*service)
	impl.scan()

	// Entry should still be active — not crashed.
	store := NewStore(workspace)
	got, _ := store.Get("inv-no-status")
	if got == nil || got.Status != WatchStatusActive {
		t.Fatalf("entry status = %v, want active after missing status.json", got)
	}
}

func TestBuildCompletionMessage(t *testing.T) {
	entry := WatchEntry{
		InvocationID:  "inv-msg-001",
		InvocationDir: "/workspace/invocations/inv-msg-001",
		CallerProfile: "main",
		TargetProfile: "worker",
		ReturnRouting: map[string]string{
			"channel_id": "feishu",
			"chat_id":    "chat-123",
		},
	}
	status := &InvocationStatus{Status: "succeeded"}
	now := time.Now().UTC()

	msg := buildCompletionMessage(entry, status, now)

	if msg.ChannelID != "feishu" {
		t.Fatalf("ChannelID = %q, want feishu", msg.ChannelID)
	}
	if msg.ChatID != "chat-123" {
		t.Fatalf("ChatID = %q, want chat-123", msg.ChatID)
	}
	if msg.Metadata["agent_profile"] != "main" {
		t.Fatalf("agent_profile = %q, want main", msg.Metadata["agent_profile"])
	}
	if msg.Metadata["completion_status"] != "succeeded" {
		t.Fatalf("completion_status = %q, want succeeded", msg.Metadata["completion_status"])
	}
}

func TestBuildTimeoutMessage(t *testing.T) {
	entry := WatchEntry{
		InvocationID:  "inv-timeout-msg",
		InvocationDir: "/workspace/invocations/inv-timeout-msg",
		CallerProfile: "main",
		TargetProfile: "worker",
		Timeout:       Duration(3600 * time.Second),
		CreatedAt:     time.Now().UTC().Add(-2 * time.Hour),
	}
	now := time.Now().UTC()

	msg := buildTimeoutMessage(entry, now)

	if msg.Metadata["message_kind"] != "task_timeout" {
		t.Fatalf("message_kind = %q, want task_timeout", msg.Metadata["message_kind"])
	}
	if msg.Metadata["completion_status"] != "timeout" {
		t.Fatalf("completion_status = %q, want timeout", msg.Metadata["completion_status"])
	}
}
