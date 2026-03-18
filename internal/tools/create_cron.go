package tools

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	cronpkg "github.com/Neneka448/gogoclaw/internal/cron"
	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
	"github.com/Neneka448/gogoclaw/internal/utils"
	openai "github.com/sashabaranov/go-openai"
)

type CreateCronTool struct {
	service cronpkg.Service

	mu      sync.Mutex
	context messagebus.Message
}

type createCronArgs struct {
	CronID         string `json:"cron_id"`
	CronExpression string `json:"cron_expression"`
	Task           string `json:"task"`
	Enabled        bool   `json:"enabled"`
	ProfileName    string `json:"profile_name,omitempty"`
}

type createCronResult struct {
	CronID         string `json:"cron_id,omitempty"`
	CronExpression string `json:"cron_expression,omitempty"`
	Enabled        bool   `json:"enabled,omitempty"`
	Task           string `json:"task,omitempty"`
	Path           string `json:"path,omitempty"`
	Error          string `json:"error,omitempty"`
}

func NewCreateCronTool(service cronpkg.Service) ToolDescriptor {
	return ToolDescriptor{
		Name: "create_cron",
		Tool: &CreateCronTool{service: service},
		ToolForLLM: openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "create_cron",
				Description: "Create a workspace cron task that runs the agent on a schedule. Use for recurring inbox checks, periodic reports, or other repeated agent workflows. By default the cron inherits the current agent profile; only set profile_name when the user explicitly wants the cron to run under a different profile.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"cron_id": map[string]any{
							"type":        "string",
							"description": "Stable cron identifier, for example qq-inbox or nightly-report.",
						},
						"cron_expression": map[string]any{
							"type":        "string",
							"description": "Standard 5-field cron expression, for example */5 * * * *.",
						},
						"task": map[string]any{
							"type":        "string",
							"description": "Complete task definition that the scheduled agent run will execute.",
						},
						"enabled": map[string]any{
							"type":        "boolean",
							"description": "Whether the cron should be enabled immediately after creation.",
						},
						"profile_name": map[string]any{
							"type":        "string",
							"description": "Optional target agent profile. Omit this unless the user explicitly wants a different profile than the current conversation profile.",
						},
					},
					"required": []string{"cron_id", "cron_expression", "task", "enabled"},
				},
			},
		},
	}
}

func (tool *CreateCronTool) Execute(args string) (string, error) {
	if tool.service == nil {
		return encodeCreateCronResult(createCronResult{Error: "cron service is not initialized"})
	}

	var input createCronArgs
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return encodeCreateCronResult(createCronResult{Error: fmt.Sprintf("parse create_cron args: %v", err)})
	}

	input.CronID = strings.TrimSpace(input.CronID)
	input.CronExpression = strings.TrimSpace(input.CronExpression)
	input.Task = strings.TrimSpace(input.Task)
	input.ProfileName = strings.TrimSpace(input.ProfileName)
	if input.CronID == "" {
		return encodeCreateCronResult(createCronResult{Error: "cron_id is required"})
	}
	if input.CronExpression == "" {
		return encodeCreateCronResult(createCronResult{CronID: input.CronID, Error: "cron_expression is required"})
	}
	if input.Task == "" {
		return encodeCreateCronResult(createCronResult{CronID: input.CronID, CronExpression: input.CronExpression, Error: "task is required"})
	}
	tool.mu.Lock()
	ctx := tool.context
	tool.mu.Unlock()
	profileName := ""
	invocationMode := ""
	if ctx.Metadata != nil {
		profileName = strings.TrimSpace(ctx.Metadata["agent_profile"])
		invocationMode = strings.TrimSpace(ctx.Metadata["invocation_mode"])
	}
	if input.ProfileName != "" {
		profileName = input.ProfileName
	}

	storedCron, err := tool.service.CreateCron(cronpkg.UpsertCronInput{
		CronID:         input.CronID,
		CronExpression: input.CronExpression,
		Enabled:        input.Enabled,
		Task:           input.Task,
		ProfileName:    profileName,
		InvocationMode: invocationMode,
	})
	if err != nil {
		return encodeCreateCronResult(createCronResult{
			CronID:         input.CronID,
			CronExpression: input.CronExpression,
			Enabled:        input.Enabled,
			Task:           input.Task,
			Error:          err.Error(),
		})
	}

	return encodeCreateCronResult(createCronResult{
		CronID:         storedCron.Config.CronID,
		CronExpression: storedCron.Config.CronExpression,
		Enabled:        storedCron.Config.Enabled,
		Task:           storedCron.Task,
		Path:           storedCron.Path,
	})
}

func encodeCreateCronResult(result createCronResult) (string, error) {
	return utils.EncodeJSON(result)
}

func (tool *CreateCronTool) SetMessageContext(message messagebus.Message) {
	tool.mu.Lock()
	defer tool.mu.Unlock()
	tool.context = message
}
