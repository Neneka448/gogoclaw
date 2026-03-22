package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Neneka448/gogoclaw/internal/config"
	cronpkg "github.com/Neneka448/gogoclaw/internal/cron"
)

func TestSyncCronsToolPicksUpCronsFromDisk(t *testing.T) {
	workspace := t.TempDir()
	resolver := config.NewProfileResolver(map[string]config.ProfileConfig{
		"default": {Workspace: workspace},
	}, "default")
	manager := cronpkg.NewCronManager(nil)
	service := cronpkg.NewCronService(resolver, manager, nil, nil)

	// Write a cron to disk before syncing.
	cronDir := filepath.Join(workspace, "crons", "test-cron")
	if err := os.MkdirAll(cronDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	configJSON := `{"cronID":"test-cron","cronExpression":"*/5 * * * *","enabled":true}`
	if err := os.WriteFile(filepath.Join(cronDir, "config.json"), []byte(configJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(config.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(cronDir, "task.md"), []byte("do something"), 0o644); err != nil {
		t.Fatalf("WriteFile(task.md) error = %v", err)
	}

	descriptor := NewSyncCronsTool(service)
	result, err := descriptor.Tool.Execute("{}")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var parsed syncCronsResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if parsed.Error != "" {
		t.Fatalf("parsed.Error = %q, want empty", parsed.Error)
	}
	if parsed.Status != "synced" {
		t.Fatalf("parsed.Status = %q, want synced", parsed.Status)
	}
	if parsed.Count != 1 {
		t.Fatalf("parsed.Count = %d, want 1", parsed.Count)
	}

	// Verify the service can see the cron.
	storedCron, err := service.GetCron("test-cron")
	if err != nil {
		t.Fatalf("GetCron() error = %v", err)
	}
	if storedCron.Config.CronID != "test-cron" {
		t.Fatalf("storedCron.Config.CronID = %q, want test-cron", storedCron.Config.CronID)
	}
}

func TestSyncCronsToolRemovesDeletedCrons(t *testing.T) {
	workspace := t.TempDir()
	resolver := config.NewProfileResolver(map[string]config.ProfileConfig{
		"default": {Workspace: workspace},
	}, "default")
	manager := cronpkg.NewCronManager(nil)
	service := cronpkg.NewCronService(resolver, manager, nil, nil)

	// Create a cron on disk and sync it in.
	cronDir := filepath.Join(workspace, "crons", "ephemeral")
	if err := os.MkdirAll(cronDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	configJSON := `{"cronID":"ephemeral","cronExpression":"0 * * * *","enabled":true}`
	if err := os.WriteFile(filepath.Join(cronDir, "config.json"), []byte(configJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(config.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(cronDir, "task.md"), []byte("temp task"), 0o644); err != nil {
		t.Fatalf("WriteFile(task.md) error = %v", err)
	}

	descriptor := NewSyncCronsTool(service)
	if _, err := descriptor.Tool.Execute("{}"); err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}

	// Verify it was loaded.
	if _, err := service.GetCron("ephemeral"); err != nil {
		t.Fatalf("GetCron() after first sync error = %v", err)
	}

	// Delete the cron from disk and re-sync.
	if err := os.RemoveAll(cronDir); err != nil {
		t.Fatalf("RemoveAll() error = %v", err)
	}

	result, err := descriptor.Tool.Execute("{}")
	if err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}

	var parsed syncCronsResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if parsed.Count != 0 {
		t.Fatalf("parsed.Count = %d, want 0 after deletion", parsed.Count)
	}
}

func TestSyncCronsToolReturnsErrorWhenServiceNil(t *testing.T) {
	descriptor := NewSyncCronsTool(nil)
	result, err := descriptor.Tool.Execute("{}")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var parsed syncCronsResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if parsed.Error == "" {
		t.Fatal("parsed.Error = empty, want service error")
	}
}
