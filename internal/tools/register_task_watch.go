package tools

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Neneka448/gogoclaw/internal/taskwatch"
	"github.com/Neneka448/gogoclaw/internal/utils"
	openai "github.com/sashabaranov/go-openai"
)

type RegisterTaskWatchTool struct {
	service taskwatch.Service
}

type registerTaskWatchArgs struct {
	InvocationID         string            `json:"invocation_id"`
	InvocationDir        string            `json:"invocation_dir"`
	CallerProfile        string            `json:"caller_profile"`
	TargetProfile        string            `json:"target_profile"`
	TaskCronID           string            `json:"task_cron_id,omitempty"`
	CheckIntervalSeconds int               `json:"check_interval_seconds,omitempty"`
	TimeoutSeconds       int               `json:"timeout_seconds,omitempty"`
	ReturnRouting        map[string]string  `json:"return_routing,omitempty"`
}

type registerTaskWatchResult struct {
	InvocationID  string `json:"invocation_id,omitempty"`
	Status        string `json:"status,omitempty"`
	CheckInterval string `json:"check_interval,omitempty"`
	Timeout       string `json:"timeout,omitempty"`
	Error         string `json:"error,omitempty"`
}

func NewRegisterTaskWatchTool(service taskwatch.Service) ToolDescriptor {
	return ToolDescriptor{
		Name: "register_task_watch",
		Tool: &RegisterTaskWatchTool{service: service},
		ToolForLLM: openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "register_task_watch",
				Description: "Register a task watch that monitors an invocation directory for completion and automatically notifies the caller profile. Use this after creating a task cron for a delegated invocation instead of creating a separate heartbeat cron. The runtime will periodically check status.json and inject a completion message back to the caller when the task finishes.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"invocation_id": map[string]any{
							"type":        "string",
							"description": "Unique invocation identifier, e.g. inv-20260322-abc123.",
						},
						"invocation_dir": map[string]any{
							"type":        "string",
							"description": "Absolute path to the invocation directory containing status.json.",
						},
						"caller_profile": map[string]any{
							"type":        "string",
							"description": "Profile name of the agent that initiated the delegation. The completion notification will be delivered to this profile.",
						},
						"target_profile": map[string]any{
							"type":        "string",
							"description": "Profile name of the agent executing the task.",
						},
						"task_cron_id": map[string]any{
							"type":        "string",
							"description": "ID of the task execution cron. If provided, the cron will be automatically disabled when the task completes.",
						},
						"check_interval_seconds": map[string]any{
							"type":        "integer",
							"description": "How often (in seconds) to check for task completion. Default: 60.",
						},
						"timeout_seconds": map[string]any{
							"type":        "integer",
							"description": "Maximum time (in seconds) to wait for task completion before sending a timeout notification. Default: 3600.",
						},
						"return_routing": map[string]any{
							"type":        "object",
							"description": "Optional routing metadata for the completion message. Keys: channel_id, chat_id, sender_id, reply_to, message_type, session_id. If omitted, completion is delivered as a new taskwatch message.",
							"properties": map[string]any{
								"channel_id":   map[string]any{"type": "string"},
								"chat_id":      map[string]any{"type": "string"},
								"sender_id":    map[string]any{"type": "string"},
								"reply_to":     map[string]any{"type": "string"},
								"message_type": map[string]any{"type": "string"},
								"session_id":   map[string]any{"type": "string"},
							},
						},
					},
					"required": []string{"invocation_id", "invocation_dir", "caller_profile", "target_profile"},
				},
			},
		},
		Timeout: DefaultToolExecutionTimeout,
	}
}

func (tool *RegisterTaskWatchTool) Execute(args string) (string, error) {
	if tool.service == nil {
		return utils.EncodeJSON(registerTaskWatchResult{Error: "task watch service is not initialized"})
	}

	var input registerTaskWatchArgs
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return utils.EncodeJSON(registerTaskWatchResult{Error: fmt.Sprintf("parse args: %v", err)})
	}

	input.InvocationID = strings.TrimSpace(input.InvocationID)
	input.InvocationDir = strings.TrimSpace(input.InvocationDir)
	input.CallerProfile = strings.TrimSpace(input.CallerProfile)
	input.TargetProfile = strings.TrimSpace(input.TargetProfile)
	input.TaskCronID = strings.TrimSpace(input.TaskCronID)

	if input.InvocationID == "" {
		return utils.EncodeJSON(registerTaskWatchResult{Error: "invocation_id is required"})
	}
	if input.InvocationDir == "" {
		return utils.EncodeJSON(registerTaskWatchResult{Error: "invocation_dir is required"})
	}
	if input.CallerProfile == "" {
		return utils.EncodeJSON(registerTaskWatchResult{Error: "caller_profile is required"})
	}

	checkInterval := taskwatch.DefaultCheckInterval
	if input.CheckIntervalSeconds > 0 {
		checkInterval = time.Duration(input.CheckIntervalSeconds) * time.Second
	}
	timeout := taskwatch.DefaultTimeout
	if input.TimeoutSeconds > 0 {
		timeout = time.Duration(input.TimeoutSeconds) * time.Second
	}

	entry := taskwatch.WatchEntry{
		InvocationID:  input.InvocationID,
		InvocationDir: input.InvocationDir,
		CallerProfile: input.CallerProfile,
		TargetProfile: input.TargetProfile,
		TaskCronID:    input.TaskCronID,
		CheckInterval: taskwatch.Duration(checkInterval),
		Timeout:       taskwatch.Duration(timeout),
		ReturnRouting: input.ReturnRouting,
	}

	if err := tool.service.Register(entry); err != nil {
		return utils.EncodeJSON(registerTaskWatchResult{
			InvocationID: input.InvocationID,
			Error:        fmt.Sprintf("register watch: %v", err),
		})
	}

	return utils.EncodeJSON(registerTaskWatchResult{
		InvocationID:  input.InvocationID,
		Status:        "registered",
		CheckInterval: checkInterval.String(),
		Timeout:       timeout.String(),
	})
}
