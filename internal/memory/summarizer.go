package memory

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Neneka448/gogoclaw/internal/provider"
	openai "github.com/sashabaranov/go-openai"
)

type Summarizer struct {
	llm   provider.LLMProviderOpenaiCompatible
	model string
}

func NewSummarizer(llm provider.LLMProviderOpenaiCompatible, model string) *Summarizer {
	return &Summarizer{llm: llm, model: model}
}

type sessionSummaryOutput struct {
	Who    string `json:"who"`
	What   string `json:"what"`
	When   string `json:"when"`
	Where  string `json:"where"`
	Why    string `json:"why"`
	How    string `json:"how"`
	Result string `json:"result"`
}

func truncateForError(content string, maxLen int) string {
	content = strings.TrimSpace(content)
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen] + "...(truncated)"
}

const sessionSummaryPrompt = `You are a memory extraction agent. Given a conversation session transcript, extract a structured 5W1H+R summary.

Rules:
- Who: the participant(s) involved
- What: the main task or topic being addressed
- When: temporal context if available, otherwise "unknown"
- Where: environment/platform/context (e.g., "production server", "local dev", "code review")
- Why: the motivation or goal behind the task
- How: a detailed step-by-step narrative of what was done. Do NOT abbreviate. Write it as a fluent, natural story: what was tried first, what happened next, what problems were encountered, how they were resolved. This should read like a concise case study that someone could learn from.
- Result: "success", "partial", or "failure" with a brief explanation

Return a single JSON object with exactly these keys: who, what, when, where, why, how, result.
Do not include any text outside of the JSON object.`

// SummarizeSession extracts a 5W1H+R summary from raw session messages.
func (s *Summarizer) SummarizeSession(sessionContent string) (*sessionSummaryOutput, error) {
	request := provider.BuildOpenaiRequestParams(provider.ChatCompletionParams{
		Model: s.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: sessionSummaryPrompt},
			{Role: openai.ChatMessageRoleUser, Content: "Here is the session transcript to summarize:\n\n" + sessionContent},
		},
		MaxCompletionTokens: 2048,
		Temperature:         0.1,
	})

	response, err := s.llm.ChatCompletion(request)
	if err != nil {
		return nil, fmt.Errorf("session summary LLM call: %w", err)
	}

	content := strings.TrimSpace(response.GetContent())
	content = stripMarkdownCodeFence(content)

	var output sessionSummaryOutput
	if err := json.Unmarshal([]byte(content), &output); err != nil {
		return nil, fmt.Errorf("parse session summary JSON: %w (content: %s)", err, truncateForError(content, 500))
	}
	return &output, nil
}

const communitySummaryPrompt = `You are a memory consolidation agent. You are given several memory entries that describe similar scenarios or tasks. Your job is to merge them into a single higher-level memory that captures the common pattern, accumulated strategy, and key lessons.

Rules:
- Who: generalize if the same, or list participants
- What: describe the recurring scenario/pattern, not individual instances
- When: time range or "recurring"
- Where: generalize the environment
- Why: the common underlying motivation
- How: synthesize the approaches taken across all instances into an integrated strategy. Mention what works, what doesn't, and key decision points. Write it as actionable guidance that reads naturally.
- Result: overall track record (e.g., "mostly successful", "mixed results - failed when X")

Return a single JSON object with exactly these keys: who, what, when, where, why, how, result.
Do not include any text outside of the JSON object.`

// SummarizeCommunity merges multiple memory nodes into a single higher-level node.
func (s *Summarizer) SummarizeCommunity(nodes []MemoryNode) (*sessionSummaryOutput, error) {
	var builder strings.Builder
	for i, node := range nodes {
		builder.WriteString(fmt.Sprintf("--- Memory %d ---\n", i+1))
		builder.WriteString(node.EmbeddingText())
		if node.Summary != "" {
			builder.WriteString("Summary: " + node.Summary + "\n")
		}
		builder.WriteString("\n")
	}

	request := provider.BuildOpenaiRequestParams(provider.ChatCompletionParams{
		Model: s.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: communitySummaryPrompt},
			{Role: openai.ChatMessageRoleUser, Content: "Here are the memory entries to consolidate:\n\n" + builder.String()},
		},
		MaxCompletionTokens: 2048,
		Temperature:         0.1,
	})

	response, err := s.llm.ChatCompletion(request)
	if err != nil {
		return nil, fmt.Errorf("community summary LLM call: %w", err)
	}

	content := strings.TrimSpace(response.GetContent())
	content = stripMarkdownCodeFence(content)

	var output sessionSummaryOutput
	if err := json.Unmarshal([]byte(content), &output); err != nil {
		return nil, fmt.Errorf("parse community summary JSON: %w (content: %s)", err, truncateForError(content, 500))
	}
	return &output, nil
}

func stripMarkdownCodeFence(content string) string {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```json") {
		content = strings.TrimPrefix(content, "```json")
	} else if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```")
	}
	if strings.HasSuffix(content, "```") {
		content = strings.TrimSuffix(content, "```")
	}
	return strings.TrimSpace(content)
}
