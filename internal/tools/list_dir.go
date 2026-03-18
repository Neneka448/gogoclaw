package tools

import (
	"encoding/json"
	"os"
	"sort"

	"github.com/Neneka448/gogoclaw/internal/utils"
	"github.com/Neneka448/gogoclaw/internal/utils/pathutil"
	openai "github.com/sashabaranov/go-openai"
)

type ListDirTool struct {
	workspace string
}

func NewListDirTool(workspace string) ToolDescriptor {
	return ToolDescriptor{
		Name: "list_dir",
		Tool: &ListDirTool{workspace: workspace},
		ToolForLLM: openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "list_dir",
				Description: "List the files in a directory from the current workspace",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{
							"type":        "string",
							"description": "Workspace-relative directory path to list. set to empty string to list the root directory. use absolute path is an error.",
						},
					},
					"required": []string{"path"},
				},
			},
		},
	}
}

type fileDesc struct {
	FileName string `json:"file_name"`
	IsDir    bool   `json:"is_dir"`
}
type listDirArgs struct {
	Path string `json:"path"`
}

type listDirResult struct {
	Files []fileDesc `json:"files"`
	Error string     `json:"error,omitempty"`
}

func (tool *ListDirTool) Execute(args string) (string, error) {
	var input listDirArgs
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return utils.EncodeJSON(listDirResult{
			Files: []fileDesc{}, Error: err.Error(),
		})
	}
	path, err := pathutil.ResolveRelativeOnly(input.Path, tool.workspace)
	if err != nil {
		return utils.EncodeJSON(listDirResult{
			Files: []fileDesc{}, Error: err.Error(),
		})
	}

	files, err := listDir(path)
	if err != nil {
		return utils.EncodeJSON(listDirResult{
			Files: []fileDesc{}, Error: err.Error(),
		})
	}
	return utils.EncodeJSON(listDirResult{
		Files: files,
	})
}

func listDir(path string) ([]fileDesc, error) {
	files, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name() < files[j].Name()
	})
	result := make([]fileDesc, 0, len(files))
	for _, file := range files {
		result = append(result, fileDesc{
			FileName: file.Name(),
			IsDir:    file.IsDir(),
		})
	}
	return result, nil

}
