package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Neneka448/gogoclaw/internal/config"
	toolspkg "github.com/Neneka448/gogoclaw/internal/tools"
	versionpkg "github.com/Neneka448/gogoclaw/internal/version"
	gosdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	openai "github.com/sashabaranov/go-openai"
)

const (
	defaultConnectTimeout = 30 * time.Second
	defaultToolPrefix     = "mcp__"
	statusDisabled        = "disabled"
	statusReady           = "ready"
	statusError           = "error"
)

var invalidToolNameChars = regexp.MustCompile(`[^a-zA-Z0-9_]+`)

type Options struct {
	FailFast bool
}

type Status struct {
	Name       string
	Transport  string
	Enabled    bool
	State      string
	ToolCount  int
	LastError  string
	RemoteName string
}

type Service interface {
	ToolDescriptors() []toolspkg.ToolDescriptor
	Statuses() []Status
	Restart(name string) (Status, error)
	Close() error
}

type service struct {
	workspace string
	options   Options

	mu      sync.RWMutex
	servers map[string]*serverRuntime
}

type serverRuntime struct {
	name      string
	workspace string
	cfg       config.MCPServerConfig

	mu              sync.RWMutex
	client          *gosdkmcp.Client
	session         *gosdkmcp.ClientSession
	descriptors     []toolspkg.ToolDescriptor
	status          Status
	connectErr      error
	connectionAlive bool
}

type remoteToolProxy struct {
	server     *serverRuntime
	remoteName string
}

func NewService(workspace string, mcpConfig config.MCPConfig, options Options) (Service, error) {
	instance := &service{
		workspace: strings.TrimSpace(workspace),
		options:   options,
		servers:   make(map[string]*serverRuntime, len(mcpConfig.MCPServers)),
	}

	names := make([]string, 0, len(mcpConfig.MCPServers))
	for name := range mcpConfig.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		runtime := &serverRuntime{
			name:      name,
			workspace: instance.workspace,
			cfg:       mcpConfig.MCPServers[name],
			status: Status{
				Name:    name,
				Enabled: mcpConfig.MCPServers[name].Enabled,
				State:   statusDisabled,
			},
		}
		instance.servers[name] = runtime
		if err := runtime.connect(); err != nil && options.FailFast && runtime.cfg.Enabled {
			_ = instance.Close()
			return nil, err
		}
	}

	return instance, nil
}

func (service *service) ToolDescriptors() []toolspkg.ToolDescriptor {
	service.mu.RLock()
	defer service.mu.RUnlock()

	all := make([]toolspkg.ToolDescriptor, 0)
	for _, name := range sortedRuntimeNames(service.servers) {
		runtime := service.servers[name]
		runtime.mu.RLock()
		all = append(all, cloneToolDescriptors(runtime.descriptors)...)
		runtime.mu.RUnlock()
	}
	return all
}

func (service *service) Statuses() []Status {
	service.mu.RLock()
	defer service.mu.RUnlock()

	statuses := make([]Status, 0, len(service.servers))
	for _, name := range sortedRuntimeNames(service.servers) {
		runtime := service.servers[name]
		runtime.mu.RLock()
		statuses = append(statuses, runtime.status)
		runtime.mu.RUnlock()
	}
	return statuses
}

func (service *service) Restart(name string) (Status, error) {
	service.mu.RLock()
	runtime, ok := service.servers[strings.TrimSpace(name)]
	service.mu.RUnlock()
	if !ok {
		return Status{}, fmt.Errorf("mcp server not found: %s", name)
	}
	if err := runtime.connect(); err != nil {
		runtime.mu.RLock()
		status := runtime.status
		runtime.mu.RUnlock()
		return status, err
	}
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()
	return runtime.status, nil
}

func (service *service) Close() error {
	service.mu.RLock()
	defer service.mu.RUnlock()

	var firstErr error
	for _, runtime := range service.servers {
		if err := runtime.close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (runtime *serverRuntime) connect() error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if err := runtime.closeLocked(); err != nil {
		return err
	}

	transportName, transport, err := runtime.buildTransport()
	runtime.status.Transport = transportName
	runtime.status.Enabled = runtime.cfg.Enabled
	runtime.status.RemoteName = runtime.name
	if err != nil {
		runtime.status.State = statusError
		runtime.status.LastError = err.Error()
		runtime.connectErr = err
		return err
	}
	if !runtime.cfg.Enabled {
		runtime.status.State = statusDisabled
		runtime.status.LastError = ""
		runtime.descriptors = nil
		runtime.connectErr = nil
		return nil
	}

	client := gosdkmcp.NewClient(&gosdkmcp.Implementation{
		Name:    "gogoclaw",
		Title:   "gogoclaw",
		Version: versionpkg.Version,
	}, &gosdkmcp.ClientOptions{
		Capabilities: &gosdkmcp.ClientCapabilities{
			RootsV2: &gosdkmcp.RootCapabilities{},
		},
		KeepAlive: runtime.keepAliveDuration(),
	})
	if root := runtime.workspaceRoot(); root != nil {
		client.AddRoots(root)
	}

	ctx, cancel := context.WithTimeout(context.Background(), runtime.timeout())
	defer cancel()

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		runtime.status.State = statusError
		runtime.status.LastError = err.Error()
		runtime.connectErr = err
		return err
	}

	toolsResult, err := session.ListTools(ctx, &gosdkmcp.ListToolsParams{})
	if err != nil {
		_ = session.Close()
		runtime.status.State = statusError
		runtime.status.LastError = err.Error()
		runtime.connectErr = err
		return err
	}

	descriptors := buildToolDescriptors(runtime, toolsResult.Tools)
	runtime.client = client
	runtime.session = session
	runtime.descriptors = descriptors
	runtime.connectErr = nil
	runtime.connectionAlive = true
	runtime.status.State = statusReady
	runtime.status.LastError = ""
	runtime.status.ToolCount = len(descriptors)

	return nil
}

func (runtime *serverRuntime) close() error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.closeLocked()
}

func (runtime *serverRuntime) closeLocked() error {
	runtime.connectionAlive = false
	runtime.descriptors = nil
	runtime.status.ToolCount = 0

	if runtime.session == nil {
		return nil
	}
	err := runtime.session.Close()
	runtime.session = nil
	runtime.client = nil
	return err
}

func (runtime *serverRuntime) buildTransport() (string, gosdkmcp.Transport, error) {
	if !runtime.cfg.Enabled {
		return detectTransport(runtime.cfg), nil, nil
	}
	command := strings.TrimSpace(runtime.cfg.Command)
	endpoint := strings.TrimSpace(runtime.cfg.URL)
	if command == "" && endpoint == "" {
		return "", nil, fmt.Errorf("mcp server %s must set either command or url", runtime.name)
	}
	if command != "" && endpoint != "" {
		return "", nil, fmt.Errorf("mcp server %s must not set both command and url", runtime.name)
	}
	if command != "" {
		cmd := exec.Command(command, runtime.cfg.Args...)
		cmd.Env = mergeEnv(runtime.cfg.Env)
		cwd, err := resolveWorkingDir(runtime.workspace, runtime.cfg.Cwd)
		if err != nil {
			return "", nil, err
		}
		cmd.Dir = cwd
		return "stdio", &gosdkmcp.CommandTransport{Command: cmd}, nil
	}
	httpClient := &http.Client{
		Timeout: runtime.timeout(),
		Transport: &headerTransport{
			base:    http.DefaultTransport,
			headers: cloneStringMap(runtime.cfg.Headers),
		},
	}
	return "streamable_http", &gosdkmcp.StreamableClientTransport{
		Endpoint:             endpoint,
		HTTPClient:           httpClient,
		DisableStandaloneSSE: runtime.cfg.DisableStandaloneSSE,
	}, nil
}

func (runtime *serverRuntime) timeout() time.Duration {
	if runtime.cfg.Timeout <= 0 {
		return defaultConnectTimeout
	}
	return time.Duration(runtime.cfg.Timeout) * time.Second
}

func (runtime *serverRuntime) keepAliveDuration() time.Duration {
	if runtime.cfg.KeepAlive <= 0 {
		return 0
	}
	return time.Duration(runtime.cfg.KeepAlive) * time.Second
}

func (runtime *serverRuntime) workspaceRoot() *gosdkmcp.Root {
	if strings.TrimSpace(runtime.workspace) == "" {
		return nil
	}
	absPath, err := filepath.Abs(runtime.workspace)
	if err != nil {
		return nil
	}
	return &gosdkmcp.Root{
		Name: runtime.name + "-workspace",
		URI:  (&url.URL{Scheme: "file", Path: filepath.ToSlash(absPath)}).String(),
	}
}

func (runtime *serverRuntime) callTool(toolName string, args any) (string, error) {
	runtime.mu.RLock()
	session := runtime.session
	timeout := runtime.timeout()
	runtime.mu.RUnlock()
	if session == nil {
		return encodeToolError("mcp server is not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	result, err := session.CallTool(ctx, &gosdkmcp.CallToolParams{Name: toolName, Arguments: args})
	if err != nil {
		return encodeToolError(err.Error())
	}
	return renderToolResult(result)
}

func (proxy *remoteToolProxy) Execute(args string) (string, error) {
	parsedArgs := map[string]any{}
	trimmed := strings.TrimSpace(args)
	if trimmed != "" {
		if err := json.Unmarshal([]byte(trimmed), &parsedArgs); err != nil {
			return encodeToolError(fmt.Sprintf("parse tool arguments: %v", err))
		}
	}
	return proxy.server.callTool(proxy.remoteName, parsedArgs)
}

func buildToolDescriptors(runtime *serverRuntime, remoteTools []*gosdkmcp.Tool) []toolspkg.ToolDescriptor {
	descriptors := make([]toolspkg.ToolDescriptor, 0, len(remoteTools))
	usedNames := make(map[string]int, len(remoteTools))
	for _, remoteTool := range remoteTools {
		if remoteTool == nil {
			continue
		}
		localName := scopedToolName(runtime.name, remoteTool.Name)
		if count := usedNames[localName]; count > 0 {
			localName = fmt.Sprintf("%s_%d", localName, count+1)
		}
		usedNames[localName]++

		description := strings.TrimSpace(remoteTool.Description)
		if description == "" {
			description = fmt.Sprintf("MCP tool %s from server %s", remoteTool.Name, runtime.name)
		} else {
			description = description + fmt.Sprintf(" (MCP server: %s, remote tool: %s)", runtime.name, remoteTool.Name)
		}

		parameters := remoteTool.InputSchema
		if parameters == nil {
			parameters = map[string]any{"type": "object", "properties": map[string]any{}}
		}

		descriptors = append(descriptors, toolspkg.ToolDescriptor{
			Name: localName,
			Tool: &remoteToolProxy{server: runtime, remoteName: remoteTool.Name},
			ToolForLLM: openai.Tool{
				Type: openai.ToolTypeFunction,
				Function: &openai.FunctionDefinition{
					Name:        localName,
					Description: description,
					Parameters:  parameters,
				},
			},
		})
	}
	return descriptors
}

func renderToolResult(result *gosdkmcp.CallToolResult) (string, error) {
	if result == nil {
		return "", nil
	}

	parts := make([]string, 0, len(result.Content)+1)
	for _, content := range result.Content {
		switch item := content.(type) {
		case *gosdkmcp.TextContent:
			if strings.TrimSpace(item.Text) != "" {
				parts = append(parts, item.Text)
			}
		case *gosdkmcp.ImageContent:
			parts = append(parts, fmt.Sprintf("[image content: %s]", item.MIMEType))
		case *gosdkmcp.AudioContent:
			parts = append(parts, fmt.Sprintf("[audio content: %s]", item.MIMEType))
		default:
			encoded, err := json.Marshal(item)
			if err != nil {
				return "", err
			}
			parts = append(parts, string(encoded))
		}
	}

	if result.StructuredContent != nil {
		encoded, err := json.Marshal(result.StructuredContent)
		if err != nil {
			return "", err
		}
		parts = append(parts, string(encoded))
	}

	content := strings.TrimSpace(strings.Join(parts, "\n"))
	if content == "" && result.IsError {
		content = "mcp tool returned an error"
	}
	if result.IsError {
		return encodeToolError(content)
	}
	return content, nil
}

func encodeToolError(message string) (string, error) {
	encoded, err := json.Marshal(map[string]any{
		"content":  strings.TrimSpace(message),
		"is_error": true,
	})
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func cloneToolDescriptors(descriptors []toolspkg.ToolDescriptor) []toolspkg.ToolDescriptor {
	if len(descriptors) == 0 {
		return nil
	}
	cloned := make([]toolspkg.ToolDescriptor, len(descriptors))
	copy(cloned, descriptors)
	return cloned
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func sortedRuntimeNames(runtimes map[string]*serverRuntime) []string {
	names := make([]string, 0, len(runtimes))
	for name := range runtimes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func detectTransport(cfg config.MCPServerConfig) string {
	if strings.TrimSpace(cfg.Command) != "" {
		return "stdio"
	}
	if strings.TrimSpace(cfg.URL) != "" {
		return "streamable_http"
	}
	return ""
}

func scopedToolName(serverName string, remoteToolName string) string {
	base := defaultToolPrefix + serverName + "__" + remoteToolName
	base = invalidToolNameChars.ReplaceAllString(base, "_")
	base = strings.Trim(base, "_")
	if base == "" {
		return defaultToolPrefix + "tool"
	}
	return base
}

func mergeEnv(overrides map[string]string) []string {
	base := os.Environ()
	if len(overrides) == 0 {
		return base
	}
	values := make(map[string]string, len(base)+len(overrides))
	for _, entry := range base {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) == 2 {
			values[parts[0]] = parts[1]
		}
	}
	for key, value := range overrides {
		values[key] = value
	}
	result := make([]string, 0, len(values))
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	sort.Strings(result)
	return result
}

func resolveWorkingDir(workspace string, configured string) (string, error) {
	if strings.TrimSpace(configured) == "" {
		if strings.TrimSpace(workspace) == "" {
			return "", nil
		}
		return filepath.Abs(workspace)
	}
	if filepath.IsAbs(configured) {
		return filepath.Clean(configured), nil
	}
	base := workspace
	if strings.TrimSpace(base) == "" {
		base = "."
	}
	return filepath.Abs(filepath.Join(base, configured))
}

type headerTransport struct {
	base    http.RoundTripper
	headers map[string]string
}

func (transport *headerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	base := transport.base
	if base == nil {
		base = http.DefaultTransport
	}
	cloned := request.Clone(request.Context())
	for key, value := range transport.headers {
		if strings.TrimSpace(key) == "" {
			continue
		}
		cloned.Header.Set(key, value)
	}
	return base.RoundTrip(cloned)
}
