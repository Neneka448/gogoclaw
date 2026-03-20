package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
)

func TestDetectDelegatedTaskTargetFromResponses(t *testing.T) {
	responses := []messagebus.Message{{
		Message: "已委派给 default profile。\n\n详情：\n- Invocation ID: `inv-20260320-c52748`\n- Invocation 目录: `/tmp/workspace/invocations/inv-20260320-c52748`\n- Task cron: `inv-20260320-c52748-task`",
	}}

	target, ok := detectDelegatedTaskTarget(responses)
	if !ok {
		t.Fatal("detectDelegatedTaskTarget() = false, want true")
	}
	if target.InvocationID != "inv-20260320-c52748" {
		t.Fatalf("InvocationID = %q, want inv-20260320-c52748", target.InvocationID)
	}
	if target.InvocationDir != "/tmp/workspace/invocations/inv-20260320-c52748" {
		t.Fatalf("InvocationDir = %q, want /tmp/workspace/invocations/inv-20260320-c52748", target.InvocationDir)
	}
	if target.TaskCronID != "inv-20260320-c52748-task" {
		t.Fatalf("TaskCronID = %q, want inv-20260320-c52748-task", target.TaskCronID)
	}
}

func TestDetectDelegatedTaskTargetFromResponsesReturnsFalseWhenMissing(t *testing.T) {
	_, ok := detectDelegatedTaskTarget([]messagebus.Message{{Message: "普通回答，没有委派信息"}})
	if ok {
		t.Fatal("detectDelegatedTaskTarget() = true, want false")
	}
}

func TestWaitForDelegatedTaskTerminalStateWaitsForSucceeded(t *testing.T) {
	workspace := t.TempDir()
	invocationID := "inv-20260320-c52748"
	invocationDir := filepath.Join(workspace, "invocations", invocationID)
	if err := os.MkdirAll(filepath.Join(invocationDir, "reports"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	statusPath := filepath.Join(invocationDir, "status.json")
	if err := os.WriteFile(statusPath, []byte(`{"status":"pending","started_at":"","finished_at":"","error":""}`), 0644); err != nil {
		t.Fatalf("WriteFile(status.json) error = %v", err)
	}

	target := delegatedTaskTarget{
		InvocationID:  invocationID,
		InvocationDir: invocationDir,
		TaskCronID:    invocationID + "-task",
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = os.WriteFile(statusPath, []byte(`{"status":"running","started_at":"2026-03-20T10:00:00Z","finished_at":"","error":""}`), 0644)
		time.Sleep(50 * time.Millisecond)
		_ = os.WriteFile(statusPath, []byte(`{"status":"succeeded","started_at":"2026-03-20T10:00:00Z","finished_at":"2026-03-20T10:00:10Z","error":""}`), 0644)
	}()

	result, err := waitForDelegatedTaskTerminalState(target, time.Second, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("waitForDelegatedTaskTerminalState() error = %v", err)
	}
	if result.Status != "succeeded" {
		t.Fatalf("result.Status = %q, want succeeded", result.Status)
	}
	if result.TimedOut {
		t.Fatal("result.TimedOut = true, want false")
	}
}

func TestWaitForDelegatedTaskTerminalStateDoesNotReturnOnRunning(t *testing.T) {
	workspace := t.TempDir()
	invocationID := "inv-20260320-c52748"
	invocationDir := filepath.Join(workspace, "invocations", invocationID)
	if err := os.MkdirAll(filepath.Join(invocationDir, "reports"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	statusPath := filepath.Join(invocationDir, "status.json")
	if err := os.WriteFile(statusPath, []byte(`{"status":"pending","started_at":"","finished_at":"","error":""}`), 0644); err != nil {
		t.Fatalf("WriteFile(status.json) error = %v", err)
	}

	target := delegatedTaskTarget{
		InvocationID:  invocationID,
		InvocationDir: invocationDir,
		TaskCronID:    invocationID + "-task",
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = os.WriteFile(statusPath, []byte(`{"status":"running","started_at":"2026-03-20T10:00:00Z","finished_at":"","error":""}`), 0644)
		time.Sleep(150 * time.Millisecond)
		_ = os.WriteFile(statusPath, []byte(`{"status":"failed","started_at":"2026-03-20T10:00:00Z","finished_at":"2026-03-20T10:00:20Z","error":"boom"}`), 0644)
	}()

	start := time.Now()
	result, err := waitForDelegatedTaskTerminalState(target, time.Second, 10*time.Millisecond)
	if err == nil {
		t.Fatal("waitForDelegatedTaskTerminalState() error = nil, want failure")
	}
	if result.Status != "failed" {
		t.Fatalf("result.Status = %q, want failed", result.Status)
	}
	if time.Since(start) < 150*time.Millisecond {
		t.Fatalf("returned too early after %s, want to wait past running state", time.Since(start))
	}
}

func TestWaitForDelegatedTaskTerminalStateTimesOut(t *testing.T) {
	workspace := t.TempDir()
	invocationID := "inv-20260320-c52748"
	invocationDir := filepath.Join(workspace, "invocations", invocationID)
	if err := os.MkdirAll(filepath.Join(invocationDir, "reports"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	statusPath := filepath.Join(invocationDir, "status.json")
	if err := os.WriteFile(statusPath, []byte(`{"status":"running","started_at":"2026-03-20T10:00:00Z","finished_at":"","error":""}`), 0644); err != nil {
		t.Fatalf("WriteFile(status.json) error = %v", err)
	}

	target := delegatedTaskTarget{
		InvocationID:  invocationID,
		InvocationDir: invocationDir,
		TaskCronID:    invocationID + "-task",
	}

	result, err := waitForDelegatedTaskTerminalState(target, 120*time.Millisecond, 10*time.Millisecond)
	if err == nil {
		t.Fatal("waitForDelegatedTaskTerminalState() error = nil, want timeout")
	}
	if !result.TimedOut {
		t.Fatal("result.TimedOut = false, want true")
	}
	if result.Status != "running" {
		t.Fatalf("result.Status = %q, want running", result.Status)
	}
}

func TestWaitForDelegatedTaskTerminalStateFallsBackToManifest(t *testing.T) {
	workspace := t.TempDir()
	invocationID := "inv-20260320-c52748"
	invocationDir := filepath.Join(workspace, "invocations", invocationID)
	if err := os.MkdirAll(filepath.Join(invocationDir, "reports"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	statusPath := filepath.Join(invocationDir, "status.json")
	if err := os.WriteFile(statusPath, []byte(`{"status":"pending","started_at":"","finished_at":"","error":""}`), 0644); err != nil {
		t.Fatalf("WriteFile(status.json) error = %v", err)
	}
	cronDir := filepath.Join(workspace, "crons", invocationID+"-task", "task_exec_20260320T100000Z")
	if err := os.MkdirAll(cronDir, 0755); err != nil {
		t.Fatalf("MkdirAll(cronDir) error = %v", err)
	}
	manifestPath := filepath.Join(cronDir, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte(`{"cronID":"inv-20260320-c52748-task","executionID":"task_exec_20260320T100000Z","status":"succeeded"}`), 0644); err != nil {
		t.Fatalf("WriteFile(manifest.json) error = %v", err)
	}

	target := delegatedTaskTarget{
		InvocationID:  invocationID,
		InvocationDir: invocationDir,
		TaskCronID:    invocationID + "-task",
	}

	result, err := waitForDelegatedTaskTerminalState(target, 200*time.Millisecond, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("waitForDelegatedTaskTerminalState() error = %v", err)
	}
	if result.Status != "succeeded" {
		t.Fatalf("result.Status = %q, want succeeded", result.Status)
	}
	if result.ExecutionDir == "" {
		t.Fatal("result.ExecutionDir = empty, want latest execution dir")
	}
}
