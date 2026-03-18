package agent

import (
	"os"
	"path/filepath"
	"testing"

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
