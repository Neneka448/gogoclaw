package tools

import (
	"encoding/json"
	"path/filepath"
	"testing"

	cronpkg "github.com/Neneka448/gogoclaw/internal/cron"
)

func TestCreateCronToolCreatesWorkspaceCron(t *testing.T) {
	workspace := t.TempDir()
	service := cronpkg.NewCronService(workspace, nil, nil, nil)
	descriptor := NewCreateCronTool(service)

	result, err := descriptor.Tool.Execute(`{"cron_id":"qq-inbox","cron_expression":"*/5 * * * *","task":"First call get_skill for qqinbox.","enabled":true}`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var parsed createCronResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if parsed.Error != "" {
		t.Fatalf("parsed.Error = %q, want empty", parsed.Error)
	}
	if parsed.CronID != "qq-inbox" {
		t.Fatalf("parsed.CronID = %q, want qq-inbox", parsed.CronID)
	}
	if parsed.CronExpression != "*/5 * * * *" {
		t.Fatalf("parsed.CronExpression = %q, want */5 * * * *", parsed.CronExpression)
	}
	if parsed.Path != filepath.Join(workspace, "crons", "qq-inbox") {
		t.Fatalf("parsed.Path = %q, want workspace cron dir", parsed.Path)
	}
}

func TestCreateCronToolReturnsValidationErrorInResult(t *testing.T) {
	workspace := t.TempDir()
	service := cronpkg.NewCronService(workspace, nil, nil, nil)
	descriptor := NewCreateCronTool(service)

	result, err := descriptor.Tool.Execute(`{"cron_id":"","cron_expression":"*/5 * * * *","task":"x","enabled":true}`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var parsed createCronResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if parsed.Error != "cron_id is required" {
		t.Fatalf("parsed.Error = %q, want cron_id validation error", parsed.Error)
	}
}

func TestCreateCronToolReturnsServiceErrorsInResult(t *testing.T) {
	workspace := t.TempDir()
	service := cronpkg.NewCronService(workspace, nil, nil, nil)
	descriptor := NewCreateCronTool(service)

	first, err := descriptor.Tool.Execute(`{"cron_id":"qq-inbox","cron_expression":"*/5 * * * *","task":"First call get_skill for qqinbox.","enabled":true}`)
	if err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}
	if first == "" {
		t.Fatal("first Execute() returned empty result")
	}

	result, err := descriptor.Tool.Execute(`{"cron_id":"qq-inbox","cron_expression":"*/5 * * * *","task":"First call get_skill for qqinbox.","enabled":true}`)
	if err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}

	var parsed createCronResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if parsed.Error == "" {
		t.Fatal("parsed.Error = empty, want duplicate cron error")
	}
}