package cron

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeCronManager struct {
	registered []string
	deleted    []string
	startCalls int
	stopCalls  int
}

func (manager *fakeCronManager) RegisterCron(cronTask Cron) error {
	manager.registered = append(manager.registered, cronTask.GetCronConfig().CronID)
	return nil
}

func (manager *fakeCronManager) GetCron(cronID string) (Cron, error) {
	return nil, ErrCronNotFound
}

func (manager *fakeCronManager) DeleteCron(cronID string) error {
	manager.deleted = append(manager.deleted, cronID)
	return nil
}

func (manager *fakeCronManager) Start() error {
	manager.startCalls++
	return nil
}

func (manager *fakeCronManager) Stop() error {
	manager.stopCalls++
	return nil
}

func TestCronServiceCreateAndGetCron(t *testing.T) {
	workspace := t.TempDir()
	manager := &fakeCronManager{}
	service := NewCronService(workspace, manager, nil, nil)

	storedCron, err := service.CreateCron(UpsertCronInput{
		CronID:         "nightly-report",
		CronExpression: "0 * * * *",
		Enabled:        true,
		Task:           "generate the nightly report",
	})
	if err != nil {
		t.Fatalf("CreateCron() error = %v", err)
	}
	if storedCron.Config.CronID != "nightly-report" {
		t.Fatalf("CronID = %q, want nightly-report", storedCron.Config.CronID)
	}
	if len(manager.registered) != 1 || manager.registered[0] != "nightly-report" {
		t.Fatalf("manager.registered = %#v, want nightly-report", manager.registered)
	}

	loadedCron, err := service.GetCron("nightly-report")
	if err != nil {
		t.Fatalf("GetCron() error = %v", err)
	}
	if loadedCron.Task != "generate the nightly report" {
		t.Fatalf("Task = %q, want generate the nightly report", loadedCron.Task)
	}
	if _, err := os.Stat(filepath.Join(workspace, "crons", "nightly-report", "config.json")); err != nil {
		t.Fatalf("config.json not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "crons", "nightly-report", "task.md")); err != nil {
		t.Fatalf("task.md not created: %v", err)
	}
}

func TestCronServiceExecuteCronCreatesExecutionArtifacts(t *testing.T) {
	workspace := t.TempDir()
	location := time.FixedZone("UTC+2", 2*60*60)
	restore := CronNowForTest(func() time.Time {
		return time.Date(2026, 3, 13, 8, 0, 0, 0, time.UTC)
	})
	defer restore()

	var captured ExecutionRequest
	service := NewCronService(workspace, nil, func(request ExecutionRequest) error {
		captured = request
		artifactPath := filepath.Join(request.ExecutionDir, "output.txt")
		return os.WriteFile(artifactPath, []byte("done"), 0644)
	}, location)
	if _, err := service.CreateCron(UpsertCronInput{
		CronID:         "indexer",
		CronExpression: "0 * * * *",
		Enabled:        true,
		Task:           "refresh the search index",
	}); err != nil {
		t.Fatalf("CreateCron() error = %v", err)
	}

	if err := service.ExecuteCron("indexer"); err != nil {
		t.Fatalf("ExecuteCron() error = %v", err)
	}
	if captured.CronID != "indexer" {
		t.Fatalf("captured.CronID = %q, want indexer", captured.CronID)
	}
	if !strings.Contains(captured.SessionID, "cron:indexer:task_exec_") {
		t.Fatalf("captured.SessionID = %q, want cron:indexer:task_exec_*", captured.SessionID)
	}
	executionDir := filepath.Join(workspace, "crons", "indexer", "task_exec_20260313T100000+0200")
	if _, err := os.Stat(filepath.Join(executionDir, "output.txt")); err != nil {
		t.Fatalf("output.txt not created: %v", err)
	}

	manifestContent, err := os.ReadFile(filepath.Join(executionDir, "manifest.json"))
	if err != nil {
		t.Fatalf("ReadFile(manifest.json) error = %v", err)
	}
	var manifest ExecutionManifest
	if err := json.Unmarshal(manifestContent, &manifest); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if manifest.Status != "succeeded" {
		t.Fatalf("manifest.Status = %q, want succeeded", manifest.Status)
	}
	if len(manifest.Artifacts) != 1 || manifest.Artifacts[0] != "output.txt" {
		t.Fatalf("manifest.Artifacts = %#v, want [output.txt]", manifest.Artifacts)
	}

	sessionRefContent, err := os.ReadFile(filepath.Join(executionDir, "session.json"))
	if err != nil {
		t.Fatalf("ReadFile(session.json) error = %v", err)
	}
	if !strings.Contains(string(sessionRefContent), `"sessionFile": "sessions/cron:indexer:task_exec_20260313T100000+0200.json"`) {
		t.Fatalf("session.json = %s", string(sessionRefContent))
	}
}

func TestCronServiceLoadAllRegistersEnabledCrons(t *testing.T) {
	workspace := t.TempDir()
	manager := &fakeCronManager{}
	service := NewCronService(workspace, manager, nil, nil)
	if _, err := service.CreateCron(UpsertCronInput{
		CronID:         "enabled-job",
		CronExpression: "0 * * * *",
		Enabled:        true,
		Task:           "enabled task",
	}); err != nil {
		t.Fatalf("CreateCron(enabled) error = %v", err)
	}
	if _, err := service.CreateCron(UpsertCronInput{
		CronID:         "disabled-job",
		CronExpression: "0 * * * *",
		Enabled:        false,
		Task:           "disabled task",
	}); err != nil {
		t.Fatalf("CreateCron(disabled) error = %v", err)
	}
	manager.registered = nil

	if err := service.LoadAll(); err != nil {
		t.Fatalf("LoadAll() error = %v", err)
	}
	if len(manager.registered) != 1 || manager.registered[0] != "enabled-job" {
		t.Fatalf("manager.registered = %#v, want [enabled-job]", manager.registered)
	}
}
