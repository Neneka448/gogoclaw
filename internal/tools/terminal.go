package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

const defaultTerminalTimeout = 30 * time.Second

func DefaultTerminalTimeout() time.Duration {
	return defaultTerminalTimeout
}

type TerminalTool struct {
	workspace string
	timeout   time.Duration
}

type terminalArgs struct {
	Command string `json:"command"`
	Cwd     string `json:"cwd,omitempty"`
}

type terminalResult struct {
	Command  string `json:"command,omitempty"`
	Cwd      string `json:"cwd,omitempty"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode int    `json:"exit_code"`
	TimedOut bool   `json:"timed_out"`
	Error    string `json:"error,omitempty"`
}

func NewTerminalTool(workspace string, timeout time.Duration) ToolDescriptor {
	if timeout <= 0 {
		timeout = defaultTerminalTimeout
	}

	return ToolDescriptor{
		Name: "terminal",
		Tool: &TerminalTool{workspace: workspace, timeout: timeout},
		ToolForLLM: openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "terminal",
				Description: "Run a non-interactive bash command inside the current workspace and return stdout, stderr, exit code, and the working directory used.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"command": map[string]any{
							"type":        "string",
							"description": "The bash command to execute.",
						},
						"cwd": map[string]any{
							"type":        "string",
							"description": "Optional workspace-relative working directory. Defaults to the workspace root.",
						},
					},
					"required": []string{"command"},
				},
			},
		},
	}
}

func (tool *TerminalTool) Execute(args string) (string, error) {
	var input terminalArgs
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return encodeTerminalResult(terminalResult{Error: fmt.Sprintf("parse terminal args: %v", err), ExitCode: -1})
	}

	input.Command = strings.TrimSpace(input.Command)
	if input.Command == "" {
		return encodeTerminalResult(terminalResult{Cwd: strings.TrimSpace(input.Cwd), Error: "terminal command is required", ExitCode: -1})
	}

	resolvedCwd, err := tool.resolveWorkingDirectory(input.Cwd)
	if err != nil {
		return encodeTerminalResult(terminalResult{Command: input.Command, Cwd: strings.TrimSpace(input.Cwd), Error: err.Error(), ExitCode: -1})
	}

	ctx, cancel := context.WithTimeout(context.Background(), tool.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-lc", input.Command)
	cmd.Dir = resolvedCwd

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	result := terminalResult{
		Command:  input.Command,
		Cwd:      resolvedCwd,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
	}

	if err == nil {
		return encodeTerminalResult(result)
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		result.TimedOut = true
		result.Error = fmt.Sprintf("command timed out after %s", tool.timeout)
		result.ExitCode = -1
		return encodeTerminalResult(result)
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return encodeTerminalResult(result)
	}

	result.ExitCode = -1
	result.Error = err.Error()
	return encodeTerminalResult(result)
}

func encodeTerminalResult(result terminalResult) (string, error) {
	encoded, err := json.Marshal(result)
	if err != nil {
		return "", err
	}

	return string(encoded), nil
}

func (tool *TerminalTool) resolveWorkingDirectory(cwd string) (string, error) {
	workspaceRoot, err := filepath.Abs(tool.workspace)
	if err != nil {
		return "", err
	}

	trimmed := strings.TrimSpace(cwd)
	if trimmed == "" || trimmed == "." {
		return workspaceRoot, nil
	}
	if filepath.IsAbs(trimmed) {
		return "", fmt.Errorf("terminal cwd must not use absolute path")
	}

	candidate := filepath.Join(workspaceRoot, filepath.Clean(trimmed))
	resolved, err := filepath.Abs(candidate)
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(workspaceRoot, resolved)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("terminal cwd %q is outside the workspace", cwd)
	}

	return resolved, nil
}
