package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

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

func (tool *ListDirTool) encodeResult(result listDirResult) (string, error) {
	encoded, err := json.Marshal(result)
	if err != nil {
		return "", err
	}

	return string(encoded), nil
}

func (tool *ListDirTool) Execute(args string) (string, error) {
	workspacePath, err := filepath.Abs(tool.workspace)
	if err != nil {
		return tool.encodeResult(listDirResult{
			Files: []fileDesc{}, Error: err.Error(),
		})
	}
	var input listDirArgs
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return tool.encodeResult(listDirResult{
			Files: []fileDesc{}, Error: err.Error(),
		})
	}
	input.Path = filepath.Clean(strings.TrimSpace(input.Path))

	if filepath.IsAbs(input.Path) {
		return tool.encodeResult(listDirResult{
			Files: []fileDesc{}, Error: "must not use absolute path",
		})
	}

	path, err := filepath.Abs(filepath.Join(workspacePath, input.Path))
	if err != nil {
		return tool.encodeResult(listDirResult{
			Files: []fileDesc{}, Error: err.Error(),
		})
	}

	rel, err := filepath.Rel(workspacePath, path)
	if err != nil {
		return tool.encodeResult(listDirResult{
			Files: []fileDesc{}, Error: err.Error(),
		})
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return tool.encodeResult(listDirResult{
			Files: []fileDesc{}, Error: "path is outside the workspace",
		})
	}

	files, err := listDir(path)
	if err != nil {
		return tool.encodeResult(listDirResult{
			Files: []fileDesc{}, Error: err.Error(),
		})
	}
	return tool.encodeResult(listDirResult{
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
