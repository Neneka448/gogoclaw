package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	cronpkg "github.com/Neneka448/gogoclaw/internal/cron"
	"github.com/Neneka448/gogoclaw/internal/utils"
	openai "github.com/sashabaranov/go-openai"
)

type ExecuteCronTool struct {
	service cronpkg.Service
}

type executeCronArgs struct {
	CronID string `json:"cron_id"`
	Async  *bool  `json:"async,omitempty"`
}

type executeCronResult struct {
	CronID string `json:"cron_id,omitempty"`
	Status string `json:"status,omitempty"`
	Mode   string `json:"mode,omitempty"`
	Error  string `json:"error,omitempty"`
}

func NewExecuteCronTool(service cronpkg.Service) ToolDescriptor {
	return ToolDescriptor{
		Name: "execute_cron",
		Tool: &ExecuteCronTool{service: service},
		ToolForLLM: openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "execute_cron",
				Description: "Trigger an existing cron task to execute immediately, without waiting for its next scheduled tick. Use this after creating a cron via the cron_task skill scripts and calling sync_crons when you need the task to start right away.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"cron_id": map[string]any{
							"type":        "string",
							"description": "The ID of the cron task to execute.",
						},
						"async": map[string]any{
							"type":        "boolean",
							"description": "If true (default), trigger execution in the background and return immediately. If false, block until execution completes.",
						},
					},
					"required": []string{"cron_id"},
				},
			},
		},
		Timeout: DefaultToolExecutionTimeout,
	}
}

func (tool *ExecuteCronTool) Execute(args string) (string, error) {
	if tool.service == nil {
		return encodeExecuteCronResult(executeCronResult{Error: "cron service is not initialized"})
	}

	var input executeCronArgs
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return encodeExecuteCronResult(executeCronResult{Error: fmt.Sprintf("parse execute_cron args: %v", err)})
	}

	input.CronID = strings.TrimSpace(input.CronID)
	if input.CronID == "" {
		return encodeExecuteCronResult(executeCronResult{Error: "cron_id is required"})
	}

	stored, err := tool.service.GetCron(input.CronID)
	if err != nil {
		return encodeExecuteCronResult(executeCronResult{CronID: input.CronID, Error: fmt.Sprintf("get cron: %v", err)})
	}
	if !stored.Config.Enabled {
		return encodeExecuteCronResult(executeCronResult{CronID: input.CronID, Error: "cron is disabled"})
	}

	async := input.Async == nil || *input.Async
	if async {
		go func() {
			if execErr := tool.service.ExecuteCron(input.CronID); execErr != nil {
				fmt.Fprintf(os.Stderr, "[execute_cron] async execution failed for %s: %v\n", input.CronID, execErr)
			}
		}()
		return encodeExecuteCronResult(executeCronResult{CronID: input.CronID, Status: "triggered", Mode: "async"})
	}

	if execErr := tool.service.ExecuteCron(input.CronID); execErr != nil {
		return encodeExecuteCronResult(executeCronResult{CronID: input.CronID, Status: "failed", Mode: "sync", Error: execErr.Error()})
	}
	return encodeExecuteCronResult(executeCronResult{CronID: input.CronID, Status: "completed", Mode: "sync"})
}

func encodeExecuteCronResult(result executeCronResult) (string, error) {
	return utils.EncodeJSON(result)
}
