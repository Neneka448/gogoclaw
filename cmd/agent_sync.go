package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
)

const syncPollInterval = time.Second

var (
	invocationIDPattern  = regexp.MustCompile("Invocation ID:\\s*`?([A-Za-z0-9._-]+)`?")
	invocationDirPattern = regexp.MustCompile("Invocation (?:directory|目录):\\s*`?([^`\\n]+)`?")
)

type delegatedTaskTarget struct {
	InvocationID  string
	InvocationDir string
	TaskCronID    string
}

type delegatedTaskStatus struct {
	Status     string `json:"status"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
	Error      string `json:"error"`
}

type executionManifest struct {
	CronID      string   `json:"cronID"`
	ExecutionID string   `json:"executionID"`
	Status      string   `json:"status"`
	StartedAt   string   `json:"startedAt"`
	FinishedAt  string   `json:"finishedAt"`
	SessionID   string   `json:"sessionID"`
	SessionFile string   `json:"sessionFile"`
	Artifacts   []string `json:"artifacts"`
	Error       string   `json:"error"`
}

type waitSummary struct {
	Message string
	Status  string
}

func waitForDelegatedTask(responses []messagebus.Message, timeout time.Duration) (waitSummary, error) {
	target, ok := detectDelegatedTaskTarget(responses)
	if !ok {
		return waitSummary{Message: "No delegated background task detected; nothing to wait for."}, nil
	}
	result, err := waitForDelegatedTaskTerminalState(target, timeout, syncPollInterval)
	if err != nil {
		return waitSummary{Message: formatDelegatedTaskWaitResult(target, result)}, err
	}
	return waitSummary{Message: formatDelegatedTaskWaitResult(target, result), Status: result.Status}, nil
}

func detectDelegatedTaskTarget(responses []messagebus.Message) (delegatedTaskTarget, bool) {
	for i := len(responses) - 1; i >= 0; i-- {
		message := responses[i].Message
		if strings.TrimSpace(message) == "" {
			continue
		}
		idMatch := invocationIDPattern.FindStringSubmatch(message)
		dirMatch := invocationDirPattern.FindStringSubmatch(message)
		if len(idMatch) < 2 || len(dirMatch) < 2 {
			continue
		}
		invocationID := strings.TrimSpace(idMatch[1])
		invocationDir := strings.TrimSpace(dirMatch[1])
		if invocationID == "" || invocationDir == "" {
			continue
		}
		return delegatedTaskTarget{
			InvocationID:  invocationID,
			InvocationDir: invocationDir,
			TaskCronID:    invocationID + "-task",
		}, true
	}
	return delegatedTaskTarget{}, false
}

type delegatedTaskWaitResult struct {
	Status         string
	TimedOut       bool
	Error          string
	Duration       time.Duration
	ReportPath     string
	ExecutionDir   string
	ManifestStatus string
}

func waitForDelegatedTaskTerminalState(target delegatedTaskTarget, timeout time.Duration, pollInterval time.Duration) (delegatedTaskWaitResult, error) {
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	if pollInterval <= 0 {
		pollInterval = syncPollInterval
	}

	start := time.Now()
	deadline := start.Add(timeout)
	for {
		status, _ := readDelegatedTaskStatus(target.InvocationDir)
		manifest, executionDir, _ := readLatestExecutionManifest(target)
		reportPath := latestReportPath(target.InvocationDir)

		result := delegatedTaskWaitResult{
			Status:       status.Status,
			Error:        status.Error,
			Duration:     time.Since(start),
			ReportPath:   reportPath,
			ExecutionDir: executionDir,
		}
		if manifest != nil {
			result.ManifestStatus = manifest.Status
			if result.Error == "" {
				result.Error = manifest.Error
			}
		}

		switch status.Status {
		case "succeeded", "failed":
			if status.Status == "failed" {
				return result, fmt.Errorf("delegated task %s failed: %s", target.TaskCronID, fallbackString(result.Error, "unknown error"))
			}
			return result, nil
		}

		if manifest != nil {
			switch manifest.Status {
			case "succeeded":
				if result.Status == "" || result.Status == "pending" || result.Status == "running" {
					result.Status = manifest.Status
				}
				return result, nil
			case "failed":
				if result.Status == "" || result.Status == "pending" || result.Status == "running" {
					result.Status = manifest.Status
				}
				return result, fmt.Errorf("delegated task %s failed: %s", target.TaskCronID, fallbackString(result.Error, "unknown error"))
			}
		}

		if time.Now().After(deadline) {
			result.TimedOut = true
			if result.Status == "" && manifest != nil {
				result.Status = manifest.Status
			}
			return result, fmt.Errorf("delegated task %s did not reach terminal state within %s", target.TaskCronID, timeout)
		}

		time.Sleep(pollInterval)
	}
}

func readDelegatedTaskStatus(invocationDir string) (delegatedTaskStatus, error) {
	statusPath := filepath.Join(invocationDir, "status.json")
	raw, err := os.ReadFile(statusPath)
	if err != nil {
		return delegatedTaskStatus{}, err
	}
	var status delegatedTaskStatus
	if err := json.Unmarshal(raw, &status); err != nil {
		return delegatedTaskStatus{}, err
	}
	return status, nil
}

func readLatestExecutionManifest(target delegatedTaskTarget) (*executionManifest, string, error) {
	workspaceDir := filepath.Dir(filepath.Dir(target.InvocationDir))
	cronDir := filepath.Join(workspaceDir, "crons", target.TaskCronID)
	entries, err := os.ReadDir(cronDir)
	if err != nil {
		return nil, "", err
	}
	attemptDirs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "task_exec_") {
			attemptDirs = append(attemptDirs, entry.Name())
		}
	}
	if len(attemptDirs) == 0 {
		return nil, "", nil
	}
	sort.Strings(attemptDirs)
	executionDir := filepath.Join(cronDir, attemptDirs[len(attemptDirs)-1])
	raw, err := os.ReadFile(filepath.Join(executionDir, "manifest.json"))
	if err != nil {
		return nil, executionDir, err
	}
	var manifest executionManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, executionDir, err
	}
	return &manifest, executionDir, nil
}

func latestReportPath(invocationDir string) string {
	reportsDir := filepath.Join(invocationDir, "reports")
	entries, err := os.ReadDir(reportsDir)
	if err != nil {
		return ""
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		files = append(files, entry.Name())
	}
	if len(files) == 0 {
		return ""
	}
	sort.Strings(files)
	return filepath.Join(reportsDir, files[len(files)-1])
}

func formatDelegatedTaskWaitResult(target delegatedTaskTarget, result delegatedTaskWaitResult) string {
	lines := []string{fmt.Sprintf("Sync wait for delegated task %s:", target.TaskCronID)}
	if result.TimedOut {
		lines = append(lines, fmt.Sprintf("- timed out after %s", result.Duration.Round(time.Second)))
		if result.Status != "" {
			lines = append(lines, fmt.Sprintf("- last known status: %s", result.Status))
		}
	} else {
		lines = append(lines, fmt.Sprintf("- terminal status: %s", fallbackString(result.Status, "unknown")))
		lines = append(lines, fmt.Sprintf("- waited: %s", result.Duration.Round(time.Second)))
	}
	if result.ExecutionDir != "" {
		lines = append(lines, fmt.Sprintf("- execution dir: %s", result.ExecutionDir))
	}
	if result.ReportPath != "" {
		lines = append(lines, fmt.Sprintf("- latest report: %s", result.ReportPath))
	}
	if result.Error != "" {
		lines = append(lines, fmt.Sprintf("- error: %s", result.Error))
	}
	return strings.Join(lines, "\n")
}

func fallbackString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
