package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
	"github.com/Neneka448/gogoclaw/internal/utils"
	"github.com/Neneka448/gogoclaw/internal/utils/pathutil"
	openai "github.com/sashabaranov/go-openai"
)

const defaultTerminalTimeout = 30 * time.Second

func DefaultTerminalTimeout() time.Duration {
	return defaultTerminalTimeout
}

type TerminalTool struct {
	workspace string
	timeout   time.Duration
	context   messagebus.Message
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
		return utils.EncodeJSON(terminalResult{Error: fmt.Sprintf("parse terminal args: %v", err), ExitCode: -1})
	}

	input.Command = strings.TrimSpace(input.Command)
	if input.Command == "" {
		return utils.EncodeJSON(terminalResult{Cwd: strings.TrimSpace(input.Cwd), Error: "terminal command is required", ExitCode: -1})
	}

	resolvedCwd, err := tool.resolveWorkingDirectory(input.Cwd)
	if err != nil {
		return utils.EncodeJSON(terminalResult{Command: input.Command, Cwd: strings.TrimSpace(input.Cwd), Error: err.Error(), ExitCode: -1})
	}

	ctx, cancel := context.WithTimeout(context.Background(), tool.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-lc", input.Command)
	cmd.Dir = resolvedCwd
	cmd.Env = append(os.Environ(), tool.commandEnv()...)

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
		return utils.EncodeJSON(result)
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		result.TimedOut = true
		result.Error = fmt.Sprintf("command timed out after %s", tool.timeout)
		result.ExitCode = -1
		return utils.EncodeJSON(result)
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return utils.EncodeJSON(result)
	}

	result.ExitCode = -1
	result.Error = err.Error()
	return utils.EncodeJSON(result)
}

func (tool *TerminalTool) SetMessageContext(message messagebus.Message) {
	tool.context = message
}

func (tool *TerminalTool) commandEnv() []string {
	context := tool.context
	metadataJSON := "{}"
	if len(context.Metadata) > 0 {
		if encoded, err := json.Marshal(context.Metadata); err == nil {
			metadataJSON = string(encoded)
		}
	}

	return []string{
		"PYTHONPATH=" + tool.pythonPathEnv(),
		"GOGOCLAW_WORKSPACE=" + strings.TrimSpace(tool.workspace),
		"GOGOCLAW_CHANNEL_ID=" + strings.TrimSpace(context.ChannelID),
		"GOGOCLAW_CHAT_ID=" + strings.TrimSpace(context.ChatID),
		"GOGOCLAW_MESSAGE_ID=" + strings.TrimSpace(context.MessageID),
		"GOGOCLAW_MESSAGE_TYPE=" + strings.TrimSpace(context.MessageType),
		"GOGOCLAW_SENDER_ID=" + strings.TrimSpace(context.SenderID),
		"GOGOCLAW_REPLY_TO=" + strings.TrimSpace(context.ReplyTo),
		"GOGOCLAW_SESSION_ID=" + strings.TrimSpace(context.ChannelID) + ":" + strings.TrimSpace(context.ChatID),
		"GOGOCLAW_AGENT_PROFILE=" + strings.TrimSpace(context.Metadata["agent_profile"]),
		"GOGOCLAW_INVOCATION_MODE=" + strings.TrimSpace(context.Metadata["invocation_mode"]),
		"GOGOCLAW_MESSAGE_METADATA_JSON=" + metadataJSON,
	}
}

func (tool *TerminalTool) resolveWorkingDirectory(cwd string) (string, error) {
	resolvedCwd, err := pathutil.ResolveWithinWorkspace(cwd, tool.workspace)
	if err != nil {
		if err.Error() == "path is outside the workspace" {
			return "", fmt.Errorf("terminal cwd %q is outside the workspace", cwd)
		}
		return "", err
	}

	return resolvedCwd, nil
}

func (tool *TerminalTool) pythonPathEnv() string {
	workspace := strings.TrimSpace(tool.workspace)
	if workspace == "" {
		return os.Getenv("PYTHONPATH")
	}

	existing := strings.TrimSpace(os.Getenv("PYTHONPATH"))
	if existing == "" {
		return workspace
	}
	return workspace + string(os.PathListSeparator) + existing
}
