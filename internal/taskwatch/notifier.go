package taskwatch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
)

// InvocationStatus is the schema written by invoke_agent task lifecycle scripts.
type InvocationStatus struct {
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
	StartedAt string `json:"started_at,omitempty"`
	EndedAt   string `json:"ended_at,omitempty"`
}

func readInvocationStatus(invocationDir string) (*InvocationStatus, error) {
	path := filepath.Join(invocationDir, "status.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var status InvocationStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

func readResultText(invocationDir string) string {
	data, err := os.ReadFile(filepath.Join(invocationDir, "result.txt"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// buildCompletionMessage constructs an inbound message that the gateway
// delivers to the caller profile's session, notifying it that a delegated
// task has finished.
func buildCompletionMessage(entry WatchEntry, status *InvocationStatus, now time.Time) messagebus.Message {
	state := strings.TrimSpace(status.Status)
	if state == "" {
		state = "unknown"
	}

	resultText := readResultText(entry.InvocationDir)
	reportPath := filepath.Join("invocations", entry.InvocationID, "reports", "final.md")
	resultPath := ""
	if resultText != "" {
		resultPath = filepath.Join("invocations", entry.InvocationID, "result.txt")
	}

	var bodyLines []string
	bodyLines = append(bodyLines,
		"SYSTEM EVENT: A delegated task you previously started for this user conversation has completed.",
		"",
		fmt.Sprintf("Invocation ID: %s", entry.InvocationID),
		fmt.Sprintf("Target profile: %s", entry.TargetProfile),
		fmt.Sprintf("Status: %s", state),
		fmt.Sprintf("Final report: %s", reportPath),
	)
	if resultPath != "" {
		bodyLines = append(bodyLines, fmt.Sprintf("Result file: %s", resultPath))
	}
	if status.Error != "" {
		bodyLines = append(bodyLines, "", fmt.Sprintf("Error: %s", status.Error))
	}
	bodyLines = append(bodyLines,
		"",
		"Read the final report first.",
		"If a result file is listed, read it before replying.",
		"Do not infer or invent exact numbers when the report is vague.",
		"Then continue the user conversation in this session with a concise update.",
	)

	metadata := map[string]string{
		"source":            "taskwatch",
		"message_kind":      "task_completion",
		"invocation_id":     entry.InvocationID,
		"agent_profile":     entry.CallerProfile,
		"completion_status":  state,
		"report_path":       reportPath,
		"result_path":       resultPath,
		"target_profile":    entry.TargetProfile,
	}
	if status.Error != "" {
		metadata["error"] = status.Error
	}

	channelID := valueOr(entry.ReturnRouting, "channel_id", "taskwatch")
	chatID := valueOr(entry.ReturnRouting, "chat_id", entry.InvocationID)
	senderID := valueOr(entry.ReturnRouting, "sender_id", "taskwatch")
	replyTo := entry.ReturnRouting["reply_to"]
	messageType := valueOr(entry.ReturnRouting, "message_type", "text")
	sessionID := entry.ReturnRouting["session_id"]

	if sessionID != "" {
		metadata["restore_session_id"] = sessionID
	}

	return messagebus.Message{
		ChannelID:   channelID,
		ChatID:      chatID,
		SenderID:    senderID,
		ReplyTo:     replyTo,
		MessageID:   fmt.Sprintf("tw-%s-%d", entry.InvocationID, now.UnixMilli()),
		MessageType: messageType,
		Message:     strings.Join(bodyLines, "\n"),
		Metadata:    metadata,
	}
}

// buildTimeoutMessage constructs a notification for the caller when
// a watched task has exceeded its configured timeout.
func buildTimeoutMessage(entry WatchEntry, now time.Time) messagebus.Message {
	bodyLines := []string{
		"SYSTEM EVENT: A delegated task has timed out without completing.",
		"",
		fmt.Sprintf("Invocation ID: %s", entry.InvocationID),
		fmt.Sprintf("Target profile: %s", entry.TargetProfile),
		fmt.Sprintf("Timeout: %s", time.Duration(entry.Timeout).String()),
		fmt.Sprintf("Created at: %s", entry.CreatedAt.Format(time.RFC3339)),
		"",
		"The task may still be running but has not reported success or failure within the expected time.",
		"Check the invocation directory for partial results, or consider retrying the task.",
	}

	metadata := map[string]string{
		"source":            "taskwatch",
		"message_kind":      "task_timeout",
		"invocation_id":     entry.InvocationID,
		"agent_profile":     entry.CallerProfile,
		"completion_status":  "timeout",
		"target_profile":    entry.TargetProfile,
	}

	channelID := valueOr(entry.ReturnRouting, "channel_id", "taskwatch")
	chatID := valueOr(entry.ReturnRouting, "chat_id", entry.InvocationID)
	senderID := valueOr(entry.ReturnRouting, "sender_id", "taskwatch")
	replyTo := entry.ReturnRouting["reply_to"]
	messageType := valueOr(entry.ReturnRouting, "message_type", "text")

	return messagebus.Message{
		ChannelID:   channelID,
		ChatID:      chatID,
		SenderID:    senderID,
		ReplyTo:     replyTo,
		MessageID:   fmt.Sprintf("tw-timeout-%s-%d", entry.InvocationID, now.UnixMilli()),
		MessageType: messageType,
		Message:     strings.Join(bodyLines, "\n"),
		Metadata:    metadata,
	}
}

func valueOr(m map[string]string, key, defaultValue string) string {
	if m == nil {
		return defaultValue
	}
	v := strings.TrimSpace(m[key])
	if v == "" {
		return defaultValue
	}
	return v
}
