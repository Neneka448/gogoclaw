package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	cronpkg "github.com/Neneka448/gogoclaw/internal/cron"
	openai "github.com/sashabaranov/go-openai"
)

type CreateCronTool struct {
	service cronpkg.Service
}

type createCronArgs struct {
	CronID         string `json:"cron_id"`
	CronExpression string `json:"cron_expression"`
	Task           string `json:"task"`
	Enabled        bool   `json:"enabled"`
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
				Description: "Create a workspace cron task that runs the agent on a schedule. Use for recurring inbox checks, periodic reports, or other repeated agent workflows.",
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
	if input.CronID == "" {
		return encodeCreateCronResult(createCronResult{Error: "cron_id is required"})
	}
	if input.CronExpression == "" {
		return encodeCreateCronResult(createCronResult{CronID: input.CronID, Error: "cron_expression is required"})
	}
	if input.Task == "" {
		return encodeCreateCronResult(createCronResult{CronID: input.CronID, CronExpression: input.CronExpression, Error: "task is required"})
	}

	storedCron, err := tool.service.CreateCron(cronpkg.UpsertCronInput{
		CronID:         input.CronID,
		CronExpression: input.CronExpression,
		Enabled:        input.Enabled,
		Task:           input.Task,
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
	encoded, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}