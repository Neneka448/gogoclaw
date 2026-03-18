package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Neneka448/gogoclaw/internal/config"
	"github.com/Neneka448/gogoclaw/internal/provider"
)

type stubCodexTokenProvider struct{}

func (stubCodexTokenProvider) GetToken() (string, string, error) {
	return "access-token", "account-123", nil
}

func TestInvocationServiceEvictsRuntimeAfterInitializationFailure(t *testing.T) {
	configPath := writeTestConfig(t)
	configManager := config.NewConfigManager(configPath)

	previousExtensionPath, hadExtensionPath := os.LookupEnv("GOGOCLAW_SQLITE_VEC_PATH")
	badExtensionPath := filepath.Join(t.TempDir(), "missing", "vec0")
	if err := os.Setenv("GOGOCLAW_SQLITE_VEC_PATH", badExtensionPath); err != nil {
		t.Fatalf("Setenv() error = %v", err)
	}
	defer func() {
		if hadExtensionPath {
			_ = os.Setenv("GOGOCLAW_SQLITE_VEC_PATH", previousExtensionPath)
			return
		}
		_ = os.Unsetenv("GOGOCLAW_SQLITE_VEC_PATH")
	}()

	rawService, err := NewInvocationService(configManager, nil, nil, nil, false, provider.TokenProvider(stubCodexTokenProvider{}))
	if err != nil {
		t.Fatalf("NewInvocationService() error = %v", err)
	}
	service, ok := rawService.(*invocationService)
	if !ok {
		t.Fatalf("service type = %T, want *invocationService", rawService)
	}
	defer func() {
		if err := service.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	}()

	if err := service.EnsureProfile("default"); err == nil {
		t.Fatal("EnsureProfile() error = nil, want initialization failure")
	}
	if len(service.runtimes) != 0 {
		t.Fatalf("len(service.runtimes) = %d, want 0 after failed initialization", len(service.runtimes))
	}

	if err := os.Unsetenv("GOGOCLAW_SQLITE_VEC_PATH"); err != nil {
		t.Fatalf("Unsetenv() error = %v", err)
	}
	if err := service.EnsureProfile("default"); err != nil {
		t.Fatalf("EnsureProfile() retry error = %v", err)
	}
	if len(service.runtimes) != 1 {
		t.Fatalf("len(service.runtimes) = %d, want 1 after successful retry", len(service.runtimes))
	}
}

func TestBuildInvocationToolConfigIndexNormalizesNamesAndKeepsFirstMatch(t *testing.T) {
	index := buildInvocationToolConfigIndex([]config.ToolConfig{
		{Name: " terminal ", Timeout: 15},
		{Name: "TERMINAL", Timeout: 99},
		{Name: "", Timeout: 30},
	})

	if len(index) != 1 {
		t.Fatalf("len(index) = %d, want 1", len(index))
	}

	toolConfig, ok := index["terminal"]
	if !ok {
		t.Fatal("index[terminal] missing")
	}
	if toolConfig.Timeout != 15 {
		t.Fatalf("toolConfig.Timeout = %d, want 15", toolConfig.Timeout)
	}
}

func TestResolveInvocationToolTimeoutUsesIndexedConfig(t *testing.T) {
	defaultTimeout := 30 * time.Second
	index := buildInvocationToolConfigIndex([]config.ToolConfig{
		{Name: "terminal", Timeout: 12},
		{Name: "message", Timeout: 0},
	})

	if got := resolveInvocationToolTimeout(index, " terminal ", defaultTimeout); got != 12*time.Second {
		t.Fatalf("resolveInvocationToolTimeout(terminal) = %s, want %s", got, 12*time.Second)
	}
	if got := resolveInvocationToolTimeout(index, "message", defaultTimeout); got != defaultTimeout {
		t.Fatalf("resolveInvocationToolTimeout(message) = %s, want %s", got, defaultTimeout)
	}
	if got := resolveInvocationToolTimeout(index, "missing", defaultTimeout); got != defaultTimeout {
		t.Fatalf("resolveInvocationToolTimeout(missing) = %s, want %s", got, defaultTimeout)
	}
}
