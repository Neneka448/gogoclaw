package onboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Neneka448/gogoclaw/internal/config"
)

func TestNormalizeContextPathsAppliesDefaultsAndExpansion(t *testing.T) {
	homePath := filepath.Join(string(os.PathSeparator), "tmp", "home")
	ctx := &onboardContext{ProfilePath: "~/.gogoclaw"}

	normalizeContextPaths(ctx, homePath)

	if got, want := ctx.ProfilePath, filepath.Join(homePath, ".gogoclaw"); got != want {
		t.Fatalf("ProfilePath = %q, want %q", got, want)
	}
	if got, want := ctx.Workspace, filepath.Join(homePath, ".gogoclaw", "workspace"); got != want {
		t.Fatalf("Workspace = %q, want %q", got, want)
	}
}

func TestPrepareProfilePathRejectsExistingConfig(t *testing.T) {
	profilePath := t.TempDir()
	configPath := filepath.Join(profilePath, configFileName)
	if err := os.WriteFile(configPath, []byte("{}"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err := prepareProfilePath(profilePath)
	if err == nil || !strings.Contains(err.Error(), "config file exists") {
		t.Fatalf("prepareProfilePath() error = %v, want config file exists", err)
	}
}

func TestPrepareWorkspacePathCreatesDirectoryAndRejectsExistingPath(t *testing.T) {
	workspacePath := filepath.Join(t.TempDir(), "workspace")
	if err := prepareWorkspacePath(workspacePath); err != nil {
		t.Fatalf("prepareWorkspacePath(create) error = %v", err)
	}
	if _, err := os.Stat(workspacePath); err != nil {
		t.Fatalf("Stat() error = %v", err)
	}

	err := prepareWorkspacePath(workspacePath)
	if err == nil || !strings.Contains(err.Error(), "workspace exists") {
		t.Fatalf("prepareWorkspacePath(existing) error = %v, want workspace exists", err)
	}
}

func TestWriteConfigWritesDefaultProfileOverrides(t *testing.T) {
	profilePath := t.TempDir()
	ctx := &onboardContext{
		ProfilePath: profilePath,
		Workspace:   filepath.Join(t.TempDir(), "workspace"),
		Provider:    "codex",
		Model:       "openai-codex/gpt-5.4",
		APIKey:      "secret-token",
	}

	if err := writeConfig(ctx); err != nil {
		t.Fatalf("writeConfig() error = %v", err)
	}

	configPath := filepath.Join(profilePath, configFileName)
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var got config.SysConfig
	if err := json.Unmarshal(content, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	defaultProfile := got.Agents.Profiles["default"]
	if defaultProfile.Workspace != ctx.Workspace {
		t.Fatalf("default workspace = %q, want %q", defaultProfile.Workspace, ctx.Workspace)
	}
	if defaultProfile.Provider != ctx.Provider {
		t.Fatalf("default provider = %q, want %q", defaultProfile.Provider, ctx.Provider)
	}
	if defaultProfile.Model != ctx.Model {
		t.Fatalf("default model = %q, want %q", defaultProfile.Model, ctx.Model)
	}
	if defaultProfile.MaxTokens != 8192 {
		t.Fatalf("default maxTokens = %d, want 8192", defaultProfile.MaxTokens)
	}

	for _, provider := range got.Providers {
		if provider.Name == ctx.Provider {
			if provider.Auth.Token != ctx.APIKey {
				t.Fatalf("provider token = %q, want %q", provider.Auth.Token, ctx.APIKey)
			}
			return
		}
	}

	t.Fatalf("provider %q not found", ctx.Provider)
}
