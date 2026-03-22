package taskwatch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const (
	invocationCronSuffix = "-task"
	invocationIDPrefix   = "inv-"
	invocationsDirName   = "invocations"
	manifestFileName     = "manifest.json"
)

// InvocationManifest is the subset of fields from the invoke_agent manifest
// that taskwatch needs to build return routing.
type InvocationManifest struct {
	InvocationID       string `json:"invocation_id"`
	CallerProfile      string `json:"caller_profile"`
	TargetProfile      string `json:"target_profile"`
	TaskCronID         string `json:"task_cron_id"`
	HeartbeatCronID    string `json:"heartbeat_cron_id"`
	ReturnChannelID    string `json:"return_channel_id"`
	ReturnChatID       string `json:"return_chat_id"`
	ReturnMessageID    string `json:"return_message_id"`
	ReturnMessageType  string `json:"return_message_type"`
	ReturnSenderID     string `json:"return_sender_id"`
	ReturnReplyTo      string `json:"return_reply_to"`
	ReturnSessionID    string `json:"return_session_id"`
	ReturnCorrelationID string `json:"return_correlation_id"`
	ReturnWorkspace    string `json:"return_workspace"`
}

// ParseInvocationCronID extracts the invocation_id from a cron_id that
// follows the convention "{invocation_id}-task". Returns empty string
// if the cron_id does not match the convention.
func ParseInvocationCronID(cronID string) string {
	cronID = strings.TrimSpace(cronID)
	if !strings.HasPrefix(cronID, invocationIDPrefix) {
		return ""
	}
	if !strings.HasSuffix(cronID, invocationCronSuffix) {
		return ""
	}
	return strings.TrimSuffix(cronID, invocationCronSuffix)
}

// ResolveInvocationDir returns the invocation directory for a given
// invocation_id within a workspace. It checks that manifest.json exists.
func ResolveInvocationDir(workspace, invocationID string) (string, error) {
	dir := filepath.Join(workspace, invocationsDirName, invocationID)
	mPath := filepath.Join(dir, manifestFileName)
	if _, err := os.Stat(mPath); err != nil {
		return "", err
	}
	return dir, nil
}

// ReadManifest reads the invocation manifest from the given directory.
func ReadManifest(invocationDir string) (*InvocationManifest, error) {
	data, err := os.ReadFile(filepath.Join(invocationDir, manifestFileName))
	if err != nil {
		return nil, err
	}
	var m InvocationManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// WatchEntryFromManifest builds a WatchEntry from an invocation manifest,
// populating all return routing fields automatically.
func WatchEntryFromManifest(manifest *InvocationManifest, invocationDir string) WatchEntry {
	routing := make(map[string]string)
	setIfNonEmpty(routing, "channel_id", manifest.ReturnChannelID)
	setIfNonEmpty(routing, "chat_id", manifest.ReturnChatID)
	setIfNonEmpty(routing, "sender_id", manifest.ReturnSenderID)
	setIfNonEmpty(routing, "reply_to", manifest.ReturnReplyTo)
	setIfNonEmpty(routing, "message_type", manifest.ReturnMessageType)
	setIfNonEmpty(routing, "session_id", manifest.ReturnSessionID)
	setIfNonEmpty(routing, "message_id", manifest.ReturnMessageID)
	setIfNonEmpty(routing, "correlation_id", manifest.ReturnCorrelationID)

	return WatchEntry{
		InvocationID:  manifest.InvocationID,
		InvocationDir: invocationDir,
		CallerProfile: manifest.CallerProfile,
		TargetProfile: manifest.TargetProfile,
		TaskCronID:    manifest.TaskCronID,
		ReturnRouting: routing,
	}
}

func setIfNonEmpty(m map[string]string, key, value string) {
	v := strings.TrimSpace(value)
	if v != "" {
		m[key] = v
	}
}
