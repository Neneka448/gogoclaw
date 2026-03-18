package tools

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Neneka448/gogoclaw/internal/utils"
	"github.com/Neneka448/gogoclaw/internal/utils/pathutil"
	openai "github.com/sashabaranov/go-openai"
)

type ReadFileTool struct {
	workspace string
}

type readFileArgs struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
}

type readFileResult struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Content   string `json:"content"`
	Error     string `json:"error,omitempty"`
}

func NewReadFileTool(workspace string) ToolDescriptor {
	return ToolDescriptor{
		Name: "read_file",
		Tool: &ReadFileTool{workspace: workspace},
		ToolForLLM: openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "read_file",
				Description: "Read a file from the current workspace. Supports optional start_line and end_line.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{
							"type":        "string",
							"description": "Workspace-relative file path to read.",
						},
						"start_line": map[string]any{
							"type":        "integer",
							"description": "Optional 1-based start line. Defaults to 1.",
						},
						"end_line": map[string]any{
							"type":        "integer",
							"description": "Optional 1-based end line. Defaults to EOF.",
						},
					},
					"required": []string{"path"},
				},
			},
		},
	}
}

func (tool *ReadFileTool) Execute(args string) (string, error) {
	var input readFileArgs
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return utils.EncodeJSON(readFileResult{Error: fmt.Sprintf("parse read_file args: %v", err)})
	}
	if strings.TrimSpace(input.Path) == "" {
		return utils.EncodeJSON(readFileResult{Error: "read_file path is required"})
	}

	resolvedPath, err := pathutil.ResolveWithinWorkspace(input.Path, tool.workspace)
	if err != nil {
		return utils.EncodeJSON(readFileResult{Path: input.Path, Error: err.Error()})
	}

	file, err := os.Open(resolvedPath)
	if err != nil {
		return utils.EncodeJSON(readFileResult{Path: input.Path, Error: err.Error()})
	}
	defer file.Close()

	startLine := input.StartLine
	if startLine <= 0 {
		startLine = 1
	}
	endLine := input.EndLine
	if endLine > 0 && endLine < startLine {
		return utils.EncodeJSON(readFileResult{
			Path:      input.Path,
			StartLine: startLine,
			EndLine:   endLine,
			Error:     "end_line must be greater than or equal to start_line",
		})
	}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNumber := 0
	selected := make([]string, 0, 32)
	lastLine := startLine - 1

	for scanner.Scan() {
		lineNumber++
		if lineNumber < startLine {
			continue
		}
		if endLine > 0 && lineNumber > endLine {
			break
		}
		selected = append(selected, scanner.Text())
		lastLine = lineNumber
	}
	if err := scanner.Err(); err != nil {
		return utils.EncodeJSON(readFileResult{
			Path:      input.Path,
			StartLine: startLine,
			EndLine:   lastLine,
			Error:     err.Error(),
		})
	}
	if len(selected) == 0 {
		lastLine = startLine - 1
	}

	result := readFileResult{
		Path:      input.Path,
		StartLine: startLine,
		EndLine:   lastLine,
		Content:   strings.Join(selected, "\n"),
	}

	return utils.EncodeJSON(result)
}
