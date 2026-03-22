package tools

import (
	cronpkg "github.com/Neneka448/gogoclaw/internal/cron"
	"github.com/Neneka448/gogoclaw/internal/utils"
	openai "github.com/sashabaranov/go-openai"
)

type SyncCronsTool struct {
	service cronpkg.Service
}

type syncCronsResult struct {
	Status string `json:"status,omitempty"`
	Count  int    `json:"count,omitempty"`
	Error  string `json:"error,omitempty"`
}

func NewSyncCronsTool(service cronpkg.Service) ToolDescriptor {
	return ToolDescriptor{
		Name: "sync_crons",
		Tool: &SyncCronsTool{service: service},
		ToolForLLM: openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "sync_crons",
				Description: "Reload all workspace cron tasks from disk and synchronize the in-memory scheduler. Call this after creating, updating, or deleting crons via the cron_task skill scripts.",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
		},
		Timeout: DefaultToolExecutionTimeout,
	}
}

func (tool *SyncCronsTool) Execute(_ string) (string, error) {
	if tool.service == nil {
		return utils.EncodeJSON(syncCronsResult{Error: "cron service is not initialized"})
	}

	if err := tool.service.Stop(); err != nil {
		return utils.EncodeJSON(syncCronsResult{Error: "stop scheduler: " + err.Error()})
	}

	if err := tool.service.LoadAll(); err != nil {
		return utils.EncodeJSON(syncCronsResult{Error: "load crons: " + err.Error()})
	}

	if err := tool.service.Start(); err != nil {
		return utils.EncodeJSON(syncCronsResult{Error: "start scheduler: " + err.Error()})
	}

	crons, err := tool.service.ListCrons()
	if err != nil {
		return utils.EncodeJSON(syncCronsResult{Status: "synced", Error: "list crons for count: " + err.Error()})
	}

	return utils.EncodeJSON(syncCronsResult{Status: "synced", Count: len(crons)})
}
