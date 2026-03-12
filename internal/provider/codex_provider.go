package provider

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	cliauth "github.com/Neneka448/gogoclaw/internal/cli/auth"
	"github.com/Neneka448/gogoclaw/internal/config"
	openai "github.com/sashabaranov/go-openai"
)

const (
	defaultCodexURL        = "https://chatgpt.com/backend-api/codex/responses"
	defaultCodexOriginator = "gogoclaw"
)

type openAICompatibleProvider struct {
	client  *openai.Client
	timeout time.Duration
}

func NewOpenAICompatibleProvider(providerConfig *config.ProviderConfig) (LLMProviderOpenaiCompatible, error) {
	if providerConfig == nil {
		return nil, fmt.Errorf("provider config is nil")
	}
	if providerConfig.Name == "codex" {
		return &codexProvider{timeout: providerTimeout(providerConfig.Timeout)}, nil
	}

	clientConfig := openai.DefaultConfig(providerConfig.Auth.Token)
	baseURL, err := resolveProviderBaseURL(providerConfig)
	if err != nil {
		return nil, err
	}
	if baseURL != "" {
		clientConfig.BaseURL = baseURL
	}

	timeout := time.Duration(providerConfig.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	return &openAICompatibleProvider{
		client:  openai.NewClientWithConfig(clientConfig),
		timeout: timeout,
	}, nil
}

type codexProvider struct {
	timeout time.Duration
}

func (provider *openAICompatibleProvider) ChatCompletion(params openai.ChatCompletionRequest) (LLMCommonResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), provider.timeout)
	defer cancel()

	response, err := provider.client.CreateChatCompletion(ctx, params)
	if err != nil {
		return nil, err
	}

	return NormalizeOpenaiResponse(response), nil
}

func (provider *codexProvider) ChatCompletion(params openai.ChatCompletionRequest) (LLMCommonResponse, error) {
	token, err := cliauth.GetCodexToken()
	if err != nil {
		return nil, err
	}

	systemPrompt, inputItems := convertCodexMessages(params.Messages)
	body := map[string]any{
		"model":               stripModelPrefix(params.Model),
		"store":               false,
		"stream":              true,
		"instructions":        systemPrompt,
		"input":               inputItems,
		"text":                map[string]any{"verbosity": "medium"},
		"include":             []string{"reasoning.encrypted_content"},
		"prompt_cache_key":    promptCacheKey(params.Messages),
		"tool_choice":         "auto",
		"parallel_tool_calls": true,
	}
	if params.ReasoningEffort != "" {
		body["reasoning"] = map[string]any{"effort": params.ReasoningEffort}
	}
	if len(params.Tools) > 0 {
		body["tools"] = convertCodexTools(params.Tools)
	}

	encodedBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, defaultCodexURL, strings.NewReader(string(encodedBody)))
	if err != nil {
		return nil, err
	}
	for key, value := range buildCodexHeaders(token.AccountID, token.Access) {
		req.Header.Set(key, value)
	}

	resp, err := (&http.Client{Timeout: provider.timeout}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, errors.New(friendlyCodexError(resp.StatusCode, string(body)))
	}

	return consumeCodexSSE(resp.Body)
}

func resolveProviderBaseURL(providerConfig *config.ProviderConfig) (string, error) {
	baseURL := strings.TrimSpace(providerConfig.BaseURL)
	if baseURL == "" {
		switch providerConfig.Name {
		case "openrouter":
			baseURL = "https://openrouter.ai/api/v1"
		case "codex":
			baseURL = "https://api.openai.com/v1"
		case "voyageai":
			baseURL = "https://api.voyageai.com/v1"
		}
	}

	if strings.TrimSpace(providerConfig.Path) == "" {
		return baseURL, nil
	}
	if baseURL == "" {
		return "", fmt.Errorf("provider %s path configured without base url", providerConfig.Name)
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse provider base url: %w", err)
	}
	parsed.Path = path.Join(parsed.Path, providerConfig.Path)
	return parsed.String(), nil
}

func providerTimeout(timeoutSeconds int) time.Duration {
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout <= 0 {
		return 60 * time.Second
	}
	return timeout
}

func stripModelPrefix(model string) string {
	if strings.HasPrefix(model, "openai-codex/") || strings.HasPrefix(model, "openai_codex/") {
		parts := strings.SplitN(model, "/", 2)
		return parts[1]
	}
	return model
}

func buildCodexHeaders(accountID string, token string) map[string]string {
	return map[string]string{
		"Authorization":      "Bearer " + token,
		"chatgpt-account-id": accountID,
		"OpenAI-Beta":        "responses=experimental",
		"originator":         defaultCodexOriginator,
		"User-Agent":         "gogoclaw (go)",
		"accept":             "text/event-stream",
		"content-type":       "application/json",
	}
}

func convertCodexTools(tools []openai.Tool) []map[string]any {
	converted := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		if tool.Type != openai.ToolTypeFunction || tool.Function == nil {
			continue
		}
		params := map[string]any{}
		if tool.Function.Parameters != nil {
			if converted, ok := tool.Function.Parameters.(map[string]any); ok {
				params = converted
			}
		}
		converted = append(converted, map[string]any{
			"type":        "function",
			"name":        tool.Function.Name,
			"description": tool.Function.Description,
			"parameters":  params,
		})
	}
	return converted
}

func convertCodexMessages(messages []openai.ChatCompletionMessage) (string, []map[string]any) {
	systemPrompt := ""
	items := make([]map[string]any, 0, len(messages))
	for index, message := range messages {
		switch message.Role {
		case openai.ChatMessageRoleSystem:
			systemPrompt = message.Content
		case openai.ChatMessageRoleUser:
			items = append(items, map[string]any{
				"role": "user",
				"content": []map[string]any{{
					"type": "input_text",
					"text": message.Content,
				}},
			})
		case openai.ChatMessageRoleAssistant:
			if strings.TrimSpace(message.Content) != "" {
				items = append(items, map[string]any{
					"type":   "message",
					"role":   "assistant",
					"status": "completed",
					"id":     fmt.Sprintf("msg_%d", index),
					"content": []map[string]any{{
						"type": "output_text",
						"text": message.Content,
					}},
				})
			}
			for _, toolCall := range message.ToolCalls {
				callID, itemID := splitToolCallID(toolCall.ID)
				if callID == "" {
					callID = fmt.Sprintf("call_%d", index)
				}
				if itemID == "" {
					itemID = fmt.Sprintf("fc_%d", index)
				}
				items = append(items, map[string]any{
					"type":      "function_call",
					"id":        itemID,
					"call_id":   callID,
					"name":      toolCall.Function.Name,
					"arguments": toolCall.Function.Arguments,
				})
			}
			if message.FunctionCall != nil {
				items = append(items, map[string]any{
					"type":      "function_call",
					"id":        fmt.Sprintf("fc_%d", index),
					"call_id":   fmt.Sprintf("call_%d", index),
					"name":      message.FunctionCall.Name,
					"arguments": message.FunctionCall.Arguments,
				})
			}
		case openai.ChatMessageRoleTool:
			callID, _ := splitToolCallID(message.ToolCallID)
			items = append(items, map[string]any{
				"type":    "function_call_output",
				"call_id": callID,
				"output":  message.Content,
			})
		}
	}
	return systemPrompt, items
}

func splitToolCallID(toolCallID string) (string, string) {
	if toolCallID == "" {
		return "", ""
	}
	parts := strings.SplitN(toolCallID, "|", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

func promptCacheKey(messages []openai.ChatCompletionMessage) string {
	encoded, _ := json.Marshal(messages)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func consumeCodexSSE(body io.Reader) (LLMCommonResponse, error) {
	content := strings.Builder{}
	finishReason := "stop"
	type toolBuffer struct {
		itemID    string
		name      string
		arguments strings.Builder
	}
	buffers := make(map[string]*toolBuffer)
	toolCalls := make([]LLMToolCall, 0)

	for event, err := range iterateSSE(body) {
		if err != nil {
			return nil, err
		}
		eventType, _ := event["type"].(string)
		switch eventType {
		case "response.output_item.added":
			item, _ := event["item"].(map[string]any)
			if item["type"] == "function_call" {
				callID, _ := item["call_id"].(string)
				if callID == "" {
					continue
				}
				buffer := &toolBuffer{}
				buffer.itemID, _ = item["id"].(string)
				buffer.name, _ = item["name"].(string)
				if raw, _ := item["arguments"].(string); raw != "" {
					buffer.arguments.WriteString(raw)
				}
				buffers[callID] = buffer
			}
		case "response.output_text.delta":
			delta, _ := event["delta"].(string)
			content.WriteString(delta)
		case "response.function_call_arguments.delta":
			callID, _ := event["call_id"].(string)
			if buffer := buffers[callID]; buffer != nil {
				delta, _ := event["delta"].(string)
				buffer.arguments.WriteString(delta)
			}
		case "response.function_call_arguments.done":
			callID, _ := event["call_id"].(string)
			if buffer := buffers[callID]; buffer != nil {
				buffer.arguments.Reset()
				finalArguments, _ := event["arguments"].(string)
				buffer.arguments.WriteString(finalArguments)
			}
		case "response.output_item.done":
			item, _ := event["item"].(map[string]any)
			if item["type"] != "function_call" {
				continue
			}
			callID, _ := item["call_id"].(string)
			if callID == "" {
				continue
			}
			buffer := buffers[callID]
			itemID, _ := item["id"].(string)
			name, _ := item["name"].(string)
			arguments, _ := item["arguments"].(string)
			if buffer != nil {
				if buffer.itemID != "" {
					itemID = buffer.itemID
				}
				if buffer.name != "" {
					name = buffer.name
				}
				if buffer.arguments.Len() > 0 {
					arguments = buffer.arguments.String()
				}
			}
			toolCalls = append(toolCalls, LLMToolCall{
				ID:        callID + "|" + itemID,
				Name:      name,
				Arguments: arguments,
				Type:      string(openai.ToolTypeFunction),
			})
		case "response.completed":
			responsePayload, _ := event["response"].(map[string]any)
			status, _ := responsePayload["status"].(string)
			finishReason = mapCodexFinishReason(status)
		case "error", "response.failed":
			return nil, fmt.Errorf("codex response failed")
		}
	}

	return NormalizedResponse{Content: content.String(), FinishReason: finishReason, ToolCalls: toolCalls}, nil
}

func iterateSSE(body io.Reader) func(func(map[string]any, error) bool) {
	return func(yield func(map[string]any, error) bool) {
		scanner := bufio.NewScanner(body)
		scanner.Buffer(make([]byte, 0, 1024), 1024*1024)
		dataLines := make([]string, 0, 4)
		flush := func() bool {
			if len(dataLines) == 0 {
				return true
			}
			payload := strings.TrimSpace(strings.Join(dataLines, "\n"))
			dataLines = dataLines[:0]
			if payload == "" || payload == "[DONE]" {
				return true
			}
			var event map[string]any
			if err := json.Unmarshal([]byte(payload), &event); err != nil {
				return yield(nil, err)
			}
			return yield(event, nil)
		}
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				if !flush() {
					return
				}
				continue
			}
			if strings.HasPrefix(line, "data:") {
				dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
		if err := scanner.Err(); err != nil {
			yield(nil, err)
			return
		}
		flush()
	}
}

func mapCodexFinishReason(status string) string {
	switch status {
	case "completed", "":
		return "stop"
	case "incomplete":
		return "length"
	case "failed", "cancelled":
		return "error"
	default:
		return "stop"
	}
}

func friendlyCodexError(statusCode int, raw string) string {
	if statusCode == http.StatusTooManyRequests {
		return "ChatGPT usage quota exceeded or rate limit triggered. Please try again later."
	}
	return fmt.Sprintf("HTTP %d: %s", statusCode, strings.TrimSpace(raw))
}
