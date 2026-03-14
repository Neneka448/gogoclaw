package channels

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Neneka448/gogoclaw/internal/config"
	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
)

type fakeChannel struct {
	name     string
	enabled  bool
	started  bool
	stopped  bool
	received []messagebus.Message
}

func (f *fakeChannel) Name() string {
	return f.name
}

func (f *fakeChannel) Enabled() bool {
	return f.enabled
}

func (f *fakeChannel) Start() error {
	f.started = true
	return nil
}

func (f *fakeChannel) Stop() error {
	f.stopped = true
	return nil
}

func (f *fakeChannel) Send(message messagebus.Message) error {
	f.received = append(f.received, message)
	return nil
}

func TestRegistryDispatchesToTargetChannel(t *testing.T) {
	registry := NewRegistry()
	channel := &fakeChannel{name: "feishu", enabled: true}
	if err := registry.Register(channel); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	msg := messagebus.Message{ChannelID: "feishu", Message: "hello"}
	if err := registry.Dispatch(msg); err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if len(channel.received) != 1 {
		t.Fatalf("len(channel.received) = %d, want 1", len(channel.received))
	}
	if channel.received[0].Message != "hello" {
		t.Fatalf("channel.received[0].Message = %q, want hello", channel.received[0].Message)
	}
}

func TestCLIChannelFormatsMessages(t *testing.T) {
	buffer := &bytes.Buffer{}
	channel := NewCLIChannel(config.CLIChannelConfig{ChannelConfig: config.ChannelConfig{Enabled: true}}, buffer)

	if err := channel.Send(messagebus.Message{Message: "[memory]: short-term memory generating", Metadata: map[string]string{"message_kind": "progress"}}); err != nil {
		t.Fatalf("Send(progress) error = %v", err)
	}
	if err := channel.Send(messagebus.Message{Message: "search()", FinishReason: "tool_calls"}); err != nil {
		t.Fatalf("Send(tool call) error = %v", err)
	}
	if err := channel.Send(messagebus.Message{Message: "done", FinishReason: "stop"}); err != nil {
		t.Fatalf("Send(message) error = %v", err)
	}

	want := "\x1b[36m[memory]: short-term memory generating\x1b[0m\n[tool call]: search()\n[message]:\ndone\n"
	if buffer.String() != want {
		t.Fatalf("buffer.String() = %q, want %q", buffer.String(), want)
	}
}

func TestExtractFeishuContent(t *testing.T) {
	text := extractFeishuContent("text", `{"text":"hello"}`)
	if text != "hello" {
		t.Fatalf("extractFeishuContent(text) = %q, want hello", text)
	}

	textWithoutAt := extractFeishuContent("text", `{"text":"@bot hello","text_without_at_bot":"hello"}`)
	if textWithoutAt != "hello" {
		t.Fatalf("extractFeishuContent(text_without_at_bot) = %q, want hello", textWithoutAt)
	}

	post := extractFeishuContent("post", `{"zh_cn":{"title":"Daily Report","content":[[{"tag":"text","text":"Done"}]]}}`)
	if post != "Daily Report Done" {
		t.Fatalf("extractFeishuContent(post) = %q, want Daily Report Done", post)
	}

	interactive := extractFeishuContent("interactive", `{"header":{"title":{"content":"Alert"}},"elements":[{"tag":"markdown","content":"Please handle this"}]}`)
	if interactive != "Alert Please handle this" {
		t.Fatalf("extractFeishuContent(interactive) = %q, want Alert Please handle this", interactive)
	}
}

func TestReceiveIDTypeForChatID(t *testing.T) {
	if got := receiveIDTypeForChatID("oc_123"); got != "chat_id" {
		t.Fatalf("receiveIDTypeForChatID(group) = %q, want chat_id", got)
	}
	if got := receiveIDTypeForChatID("ou_123"); got != "open_id" {
		t.Fatalf("receiveIDTypeForChatID(user) = %q, want open_id", got)
	}
}

func TestParseFeishuInboundMessage(t *testing.T) {
	body := map[string]any{
		"event": map[string]any{
			"sender": map[string]any{
				"sender_id": map[string]any{
					"open_id": "ou_user_1",
				},
			},
			"message": map[string]any{
				"message_id":   "om_1",
				"message_type": "text",
				"chat_id":      "oc_chat_1",
				"chat_type":    "group",
				"parent_id":    "om_parent",
				"thread_id":    "omt_thread",
				"content":      `{"text_without_at_bot":"hello"}`,
			},
		},
	}

	message, ok := parseFeishuInboundMessage(body)
	if !ok {
		t.Fatal("parseFeishuInboundMessage() ok = false, want true")
	}
	if message.ChannelID != "feishu" {
		t.Fatalf("message.ChannelID = %q, want feishu", message.ChannelID)
	}
	if message.Message != "hello" {
		t.Fatalf("message.Message = %q, want hello", message.Message)
	}
	if message.ChatID != "oc_chat_1" || message.SenderID != "ou_user_1" {
		t.Fatalf("message ids = (%q,%q), want (oc_chat_1,ou_user_1)", message.ChatID, message.SenderID)
	}
	if message.ReplyTo != "om_parent" {
		t.Fatalf("message.ReplyTo = %q, want om_parent", message.ReplyTo)
	}
	if message.Metadata["thread_id"] != "omt_thread" {
		t.Fatalf("message.Metadata[thread_id] = %q, want omt_thread", message.Metadata["thread_id"])
	}
}

func TestParseFeishuInboundMessageSkipsEmptyContent(t *testing.T) {
	body := map[string]any{
		"event": map[string]any{
			"message": map[string]any{
				"message_type": "image",
				"content":      `{}`,
			},
		},
	}

	if _, ok := parseFeishuInboundMessage(body); ok {
		t.Fatal("parseFeishuInboundMessage() ok = true, want false")
	}
}

func TestOutboundMediaPathsPrefersMediaPathsField(t *testing.T) {
	message := messagebus.Message{
		MediaPaths: []string{"/tmp/a.png", "/tmp/b.pdf"},
		Metadata:   map[string]string{"media_path": "/tmp/c.png"},
	}
	paths := outboundMediaPaths(message)
	if len(paths) != 2 || paths[0] != "/tmp/a.png" || paths[1] != "/tmp/b.pdf" {
		t.Fatalf("outboundMediaPaths() = %#v, want explicit MediaPaths", paths)
	}
}

func TestOutboundMediaPathsFallsBackToMetadataJSON(t *testing.T) {
	message := messagebus.Message{
		Metadata: map[string]string{"media_paths": `["/tmp/a.png","/tmp/b.pdf"]`},
	}
	paths := outboundMediaPaths(message)
	if len(paths) != 2 || paths[1] != "/tmp/b.pdf" {
		t.Fatalf("outboundMediaPaths() = %#v, want metadata json paths", paths)
	}
}

func TestOutboundMediaPathsFallsBackToMessagePath(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "image.png")
	message := messagebus.Message{Message: filePath, MessageType: "image"}
	if err := os.WriteFile(filePath, []byte("x"), 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	paths := outboundMediaPaths(message)
	if len(paths) != 1 || paths[0] != filePath {
		t.Fatalf("outboundMediaPaths() = %#v, want [%s]", paths, filePath)
	}
}

func TestFeishuFileTypeHelpers(t *testing.T) {
	if got := feishuUploadFileType(".pdf"); got != "pdf" {
		t.Fatalf("feishuUploadFileType(.pdf) = %q, want pdf", got)
	}
	if got := feishuUploadFileType(".zip"); got != "stream" {
		t.Fatalf("feishuUploadFileType(.zip) = %q, want stream", got)
	}
	if got := feishuMessageTypeForFileExt(".mp4"); got != "media" {
		t.Fatalf("feishuMessageTypeForFileExt(.mp4) = %q, want media", got)
	}
	if got := feishuMessageTypeForFileExt(".pdf"); got != "file" {
		t.Fatalf("feishuMessageTypeForFileExt(.pdf) = %q, want file", got)
	}
}

func TestFeishuChannelResolveMediaPathUsesWorkspaceForRelativePaths(t *testing.T) {
	workspace := t.TempDir()
	relativePath := filepath.Join("tmp", "fullscreen.png")
	absPath := filepath.Join(workspace, relativePath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(absPath, []byte("x"), 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	channel := NewFeishuChannel(config.FeishuChannelConfig{}, nil, workspace)
	resolved, ok := channel.resolveMediaPath(relativePath)
	if !ok {
		t.Fatal("resolveMediaPath() ok = false, want true")
	}
	if resolved != absPath {
		t.Fatalf("resolveMediaPath() = %q, want %q", resolved, absPath)
	}
}

func TestFeishuChannelResolveMediaPathRejectsMissingRelativeFile(t *testing.T) {
	channel := NewFeishuChannel(config.FeishuChannelConfig{}, nil, t.TempDir())
	if resolved, ok := channel.resolveMediaPath(filepath.Join("tmp", "missing.png")); ok {
		t.Fatalf("resolveMediaPath() = %q, want not found", resolved)
	}
}

func TestIsBotSender(t *testing.T) {
	body := map[string]any{
		"event": map[string]any{
			"sender": map[string]any{
				"sender_type": "bot",
			},
		},
	}
	if !isBotSender(body) {
		t.Fatal("isBotSender() = false, want true")
	}
}

func TestDetectFeishuMessageFormat(t *testing.T) {
	if got := detectFeishuMessageFormat("short plain text"); got != "text" {
		t.Fatalf("detectFeishuMessageFormat(text) = %q, want text", got)
	}
	if got := detectFeishuMessageFormat("read [docs](https://example.com)"); got != "post" {
		t.Fatalf("detectFeishuMessageFormat(link) = %q, want post", got)
	}
	if got := detectFeishuMessageFormat("- item 1\n- item 2"); got != "interactive" {
		t.Fatalf("detectFeishuMessageFormat(list) = %q, want interactive", got)
	}
	if got := detectFeishuMessageFormat("**bold** text"); got != "interactive" {
		t.Fatalf("detectFeishuMessageFormat(markdown) = %q, want interactive", got)
	}
}

func TestMarkdownToFeishuPost(t *testing.T) {
	content, err := markdownToFeishuPost("hello [docs](https://example.com)")
	if err != nil {
		t.Fatalf("markdownToFeishuPost() error = %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(content), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	zhCN, ok := body["zh_cn"].(map[string]any)
	if !ok {
		t.Fatalf("body[zh_cn] missing: %#v", body)
	}
	rows, ok := zhCN["content"].([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("rows = %#v, want one row", zhCN["content"])
	}
}

func TestMarkdownToFeishuInteractive(t *testing.T) {
	content, err := markdownToFeishuInteractive("# heading")
	if err != nil {
		t.Fatalf("markdownToFeishuInteractive() error = %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(content), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	elements, ok := body["elements"].([]any)
	if !ok || len(elements) != 1 {
		t.Fatalf("elements = %#v, want one markdown element", body["elements"])
	}
}

func TestBuildFeishuOutboundContent(t *testing.T) {
	messageType, content, err := buildFeishuOutboundContent("plain text")
	if err != nil {
		t.Fatalf("buildFeishuOutboundContent(text) error = %v", err)
	}
	if messageType != "text" || content == "" {
		t.Fatalf("buildFeishuOutboundContent(text) = (%q,%q)", messageType, content)
	}
	var textPayload map[string]string
	if err := json.Unmarshal([]byte(content), &textPayload); err != nil {
		t.Fatalf("json.Unmarshal(text payload) error = %v", err)
	}
	if textPayload["text"] != "plain text" {
		t.Fatalf("textPayload[text] = %q, want plain text", textPayload["text"])
	}

	messageType, content, err = buildFeishuOutboundContent("see [docs](https://example.com)")
	if err != nil {
		t.Fatalf("buildFeishuOutboundContent(post) error = %v", err)
	}
	if messageType != "post" || content == "" {
		t.Fatalf("buildFeishuOutboundContent(post) = (%q,%q)", messageType, content)
	}

	messageType, content, err = buildFeishuOutboundContent("1. one\n2. two")
	if err != nil {
		t.Fatalf("buildFeishuOutboundContent(interactive) error = %v", err)
	}
	if messageType != "interactive" || content == "" {
		t.Fatalf("buildFeishuOutboundContent(interactive) = (%q,%q)", messageType, content)
	}
}

func TestBuildFeishuToolResultCard(t *testing.T) {
	content, err := buildFeishuToolResultCard("{\"name\":\"screenshot\",\"content\":\"very long body\"}")
	if err != nil {
		t.Fatalf("buildFeishuToolResultCard() error = %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(content), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if header, ok := body["header"].(map[string]any); !ok || header["template"] != "blue" {
		t.Fatalf("header = %#v, want blue template", body["header"])
	}
	elements, ok := body["elements"].([]any)
	if !ok || len(elements) != 2 {
		t.Fatalf("elements = %#v, want 2 elements", body["elements"])
	}
	panel, ok := elements[1].(map[string]any)
	if !ok || panel["tag"] != "collapsible_panel" {
		t.Fatalf("panel = %#v, want collapsible_panel", elements[1])
	}
	if expanded, ok := panel["expanded"].(bool); !ok || expanded {
		t.Fatalf("panel.expanded = %#v, want false", panel["expanded"])
	}
	panelElements, ok := panel["elements"].([]any)
	if !ok || len(panelElements) != 1 {
		t.Fatalf("panel.elements = %#v, want one markdown element", panel["elements"])
	}
	markdown, ok := panelElements[0].(map[string]any)
	if !ok || !strings.Contains(markdown["content"].(string), "screenshot") {
		t.Fatalf("markdown = %#v, want full tool result content", panelElements[0])
	}
	preview, ok := elements[0].(map[string]any)
	if !ok || preview["tag"] != "markdown" {
		t.Fatalf("preview = %#v, want markdown summary", elements[0])
	}
}

func TestSplitToolCallMessage(t *testing.T) {
	name, arguments := splitToolCallMessage("read_file({\"path\":\"sessions/a.json\"})")
	if name != "read_file" {
		t.Fatalf("splitToolCallMessage() name = %q, want read_file", name)
	}
	if arguments != `{"path":"sessions/a.json"}` {
		t.Fatalf("splitToolCallMessage() arguments = %q", arguments)
	}
}

func TestPrettyToolCallArguments(t *testing.T) {
	formatted := prettyToolCallArguments(`{"path":"sessions/a.json","start_line":1}`)
	if !strings.Contains(formatted, "\n") || !strings.Contains(formatted, `"path": "sessions/a.json"`) {
		t.Fatalf("prettyToolCallArguments() = %q", formatted)
	}
}

func TestFormatToolCallBatch(t *testing.T) {
	formatted := formatToolCallBatch([]messagebus.Message{{Message: `read_file({"path":"sessions/a.json","start_line":1})`}})
	if !strings.Contains(formatted, "Tool calls in progress") {
		t.Fatalf("formatToolCallBatch() missing header: %q", formatted)
	}
	if !strings.Contains(formatted, "1. read_file") {
		t.Fatalf("formatToolCallBatch() missing name: %q", formatted)
	}
	if !strings.Contains(formatted, "```json") {
		t.Fatalf("formatToolCallBatch() missing json fence: %q", formatted)
	}
}

func TestToolCallBatchKey(t *testing.T) {
	withReply := toolCallBatchKey(messagebus.Message{ChatID: "oc_1", ReplyTo: "om_root"})
	withoutReply := toolCallBatchKey(messagebus.Message{ChatID: "oc_1"})
	if withReply != "oc_1|om_root" {
		t.Fatalf("toolCallBatchKey(withReply) = %q", withReply)
	}
	if withoutReply != "oc_1" {
		t.Fatalf("toolCallBatchKey(withoutReply) = %q", withoutReply)
	}
}
