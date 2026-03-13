package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Neneka448/gogoclaw/internal/config"
	gosdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestNewServiceConnectsStreamableHTTPServer(t *testing.T) {
	server := newTestMCPServer(t)
	defer server.Close()

	service, err := NewService(t.TempDir(), config.MCPConfig{
		MCPServers: map[string]config.MCPServerConfig{
			"echo": {
				Enabled:              true,
				URL:                  server.URL,
				DisableStandaloneSSE: true,
			},
		},
	}, Options{FailFast: true})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	statuses := service.Statuses()
	if len(statuses) != 1 {
		t.Fatalf("len(statuses) = %d, want 1", len(statuses))
	}
	if statuses[0].State != statusReady {
		t.Fatalf("statuses[0].State = %q, want %q", statuses[0].State, statusReady)
	}
	if statuses[0].ToolCount != 1 {
		t.Fatalf("statuses[0].ToolCount = %d, want 1", statuses[0].ToolCount)
	}

	descriptors := service.ToolDescriptors()
	if len(descriptors) != 1 {
		t.Fatalf("len(descriptors) = %d, want 1", len(descriptors))
	}
	if descriptors[0].Name != "mcp__echo__echo" {
		t.Fatalf("descriptors[0].Name = %q, want mcp__echo__echo", descriptors[0].Name)
	}

	result, err := descriptors[0].Tool.Execute(`{"text":"hello"}`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result, "hello") {
		t.Fatalf("result = %q, want to contain hello", result)
	}
}

func TestServiceRestartReconnectsServer(t *testing.T) {
	server := newTestMCPServer(t)
	defer server.Close()

	service, err := NewService(t.TempDir(), config.MCPConfig{
		MCPServers: map[string]config.MCPServerConfig{
			"echo": {
				Enabled:              true,
				URL:                  server.URL,
				DisableStandaloneSSE: true,
			},
		},
	}, Options{FailFast: true})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	status, err := service.Restart("echo")
	if err != nil {
		t.Fatalf("Restart() error = %v", err)
	}
	if status.State != statusReady {
		t.Fatalf("status.State = %q, want %q", status.State, statusReady)
	}
	if status.ToolCount != 1 {
		t.Fatalf("status.ToolCount = %d, want 1", status.ToolCount)
	}
}

func newTestMCPServer(t *testing.T) *httptest.Server {
	t.Helper()

	server := gosdkmcp.NewServer(&gosdkmcp.Implementation{Name: "test", Version: "v1.0.0"}, nil)
	type echoInput struct {
		Text string `json:"text" jsonschema:"text to echo"`
	}
	type echoOutput struct {
		Echo string `json:"echo" jsonschema:"echo result"`
	}
	gosdkmcp.AddTool(server, &gosdkmcp.Tool{Name: "echo", Description: "Echo input text"}, func(ctx context.Context, req *gosdkmcp.CallToolRequest, input echoInput) (*gosdkmcp.CallToolResult, echoOutput, error) {
		return nil, echoOutput{Echo: input.Text}, nil
	})

	handler := gosdkmcp.NewStreamableHTTPHandler(func(request *http.Request) *gosdkmcp.Server {
		return server
	}, nil)

	return httptest.NewServer(handler)
}
