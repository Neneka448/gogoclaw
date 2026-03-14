package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Neneka448/gogoclaw/internal/memory"
	openai "github.com/sashabaranov/go-openai"
)

type RecallMemoryTool struct {
	memoryService memory.Service
}

type recallMemoryArgs struct {
	Query         string  `json:"query"`
	TopK          int     `json:"top_k,omitempty"`
	MinSimilarity float64 `json:"min_similarity,omitempty"`
}

type recallMemoryResult struct {
	Memories []recallMemoryEntry `json:"memories,omitempty"`
	Error    string              `json:"error,omitempty"`
	Count    int                 `json:"count"`
}

type recallMemoryEntry struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Level    int    `json:"level"`
	Who      string `json:"who,omitempty"`
	What     string `json:"what,omitempty"`
	When     string `json:"when,omitempty"`
	Where    string `json:"where,omitempty"`
	Why      string `json:"why,omitempty"`
	How      string `json:"how,omitempty"`
	Result   string `json:"result,omitempty"`
	RefCount int    `json:"ref_count"`
}

func NewRecallMemoryTool(memoryService memory.Service) ToolDescriptor {
	return ToolDescriptor{
		Name: "recall_memory",
		Tool: &RecallMemoryTool{memoryService: memoryService},
		ToolForLLM: openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "recall_memory",
				Description: "Search past experience memories relevant to the current task. Returns structured memory entries (5W1H+Result) from both short-term episodic and long-term consolidated memories. Use this when you encounter a situation that might benefit from past experience.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{
							"type":        "string",
							"description": "A natural language description of the situation, task, or problem you want to recall relevant memories for. Be specific about the scenario.",
						},
						"top_k": map[string]any{
							"type":        "integer",
							"description": "Maximum number of memories to return. Defaults to 5 if not specified.",
						},
						"min_similarity": map[string]any{
							"type":        "number",
							"description": "Minimum similarity threshold (0-1). Set to 0 to return all results regardless of similarity. Omit or set to a negative value to use the configured default (typically 0.6).",
						},
					},
					"required": []string{"query"},
				},
			},
		},
	}
}

func (tool *RecallMemoryTool) Execute(args string) (string, error) {
	if tool.memoryService == nil {
		return encodeRecallResult(recallMemoryResult{Error: "memory service is not initialized"})
	}

	var input recallMemoryArgs
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return encodeRecallResult(recallMemoryResult{Error: fmt.Sprintf("parse recall_memory args: %v", err)})
	}
	if strings.TrimSpace(input.Query) == "" {
		return encodeRecallResult(recallMemoryResult{Error: "query is required"})
	}

	nodes, err := tool.memoryService.Recall(input.Query, input.TopK, input.MinSimilarity)
	if err != nil {
		return encodeRecallResult(recallMemoryResult{Error: fmt.Sprintf("recall failed: %v", err)})
	}

	entries := make([]recallMemoryEntry, 0, len(nodes))
	for _, node := range nodes {
		entries = append(entries, recallMemoryEntry{
			ID:       node.ID,
			Kind:     string(node.Kind),
			Level:    node.Level,
			Who:      node.Who,
			What:     node.What,
			When:     node.When,
			Where:    node.Where,
			Why:      node.Why,
			How:      node.How,
			Result:   node.Result,
			RefCount: node.RefCount,
		})
	}

	return encodeRecallResult(recallMemoryResult{
		Memories: entries,
		Count:    len(entries),
	})
}

func encodeRecallResult(result recallMemoryResult) (string, error) {
	encoded, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}
