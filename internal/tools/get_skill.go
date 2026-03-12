package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Neneka448/gogoclaw/internal/skills"
	openai "github.com/sashabaranov/go-openai"
)

type GetSkillTool struct {
	registry skills.Registry
}

type getSkillArgs struct {
	Name string `json:"name"`
}

type getSkillResult struct {
	Name        string         `json:"name,omitempty"`
	FilePath    string         `json:"file_path,omitempty"`
	FrontMatter map[string]any `json:"front_matter,omitempty"`
	Content     string         `json:"content,omitempty"`
	Error       string         `json:"error,omitempty"`
}

func NewGetSkillTool(registry skills.Registry) ToolDescriptor {
	return ToolDescriptor{
		Name: "get_skill",
		Tool: &GetSkillTool{registry: registry},
		ToolForLLM: openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "get_skill",
				Description: "Read the full SKILL.md content for a workspace skill by skill name.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name": map[string]any{
							"type":        "string",
							"description": "Workspace skill name, which is also the folder name under workspace/skills.",
						},
					},
					"required": []string{"name"},
				},
			},
		},
	}
}

func (tool *GetSkillTool) Execute(args string) (string, error) {
	if tool.registry == nil {
		return encodeGetSkillResult(getSkillResult{Error: "skill registry is not initialized"})
	}

	var input getSkillArgs
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return encodeGetSkillResult(getSkillResult{Error: fmt.Sprintf("parse get_skill args: %v", err)})
	}
	if strings.TrimSpace(input.Name) == "" {
		return encodeGetSkillResult(getSkillResult{Error: "get_skill name is required"})
	}

	skill, err := tool.registry.Get(input.Name)
	if err != nil {
		return encodeGetSkillResult(getSkillResult{Name: input.Name, Error: err.Error()})
	}

	return encodeGetSkillResult(getSkillResult{
		Name:        skill.Name,
		FilePath:    skill.FilePath,
		FrontMatter: skill.FrontMatter,
		Content:     skill.Content,
	})
}

func encodeGetSkillResult(result getSkillResult) (string, error) {
	encoded, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}
