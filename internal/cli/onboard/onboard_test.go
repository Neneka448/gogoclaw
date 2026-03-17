package onboard

import (
	"encoding/json"
	"os"
	"path/filepath"
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

func TestValidateProfileNameRejectsEmptyValue(t *testing.T) {
	if err := validateProfileName("   "); err == nil {
		t.Fatal("validateProfileName() error = nil, want non-empty profile name error")
	}
}

func TestResolveInteractiveWorkspacePathUsesProfileDefault(t *testing.T) {
	homePath := filepath.Join(string(os.PathSeparator), "tmp", "home")
	profilePath := filepath.Join(homePath, ".gogoclaw")

	got := resolveInteractiveWorkspacePath("", profilePath, homePath)
	want := filepath.Join(profilePath, "workspace")
	if got != want {
		t.Fatalf("resolveInteractiveWorkspacePath() = %q, want %q", got, want)
	}
}

func TestPrepareProfilePathAllowsExistingConfig(t *testing.T) {
	profilePath := t.TempDir()
	configPath := filepath.Join(profilePath, configFileName)
	if err := os.WriteFile(configPath, []byte("{}"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := prepareProfilePath(profilePath); err != nil {
		t.Fatalf("prepareProfilePath() error = %v, want nil", err)
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

	if err := prepareWorkspacePath(workspacePath); err != nil {
		t.Fatalf("prepareWorkspacePath(existing) error = %v, want nil", err)
	}
}

func TestWriteConfigWritesDefaultProfileOverrides(t *testing.T) {
	profilePath := t.TempDir()
	ctx := &onboardContext{
		ProfilePath: profilePath,
		ProfileName: "default",
		Workspace:   filepath.Join(t.TempDir(), "workspace"),
		Provider:    "codex",
		Model:       "openai-codex/gpt-5.4",
		APIKey:      "secret-token",
	}

	writtenConfig, err := writeConfig(ctx)
	if err != nil {
		t.Fatalf("writeConfig() error = %v", err)
	}
	if writtenConfig == nil {
		t.Fatal("writeConfig() returned nil config")
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

func TestWriteConfigRejectsWorkspaceConflictAcrossProfiles(t *testing.T) {
	profilePath := t.TempDir()
	configPath := filepath.Join(profilePath, configFileName)
	sharedWorkspace := filepath.Join(t.TempDir(), "shared-workspace")
	existingConfig := config.CreateDefaultConfig()
	existingConfig.Agents.Profiles["default"] = config.ProfileConfig{
		Workspace:         sharedWorkspace,
		Provider:          "codex",
		Model:             "gpt-5.4",
		MaxTokens:         512,
		Temperature:       0.1,
		MaxToolIterations: 4,
		MemoryWindow:      10,
		MaxRetryTimes:     1,
	}
	encoded, err := json.Marshal(existingConfig)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(configPath, encoded, 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	ctx := &onboardContext{
		ProfilePath: profilePath,
		ProfileName: "worker",
		Workspace:   sharedWorkspace,
		Provider:    "openrouter",
		Model:       "openai/gpt-4.1",
	}

	if _, err := writeConfig(ctx); err == nil {
		t.Fatal("writeConfig() error = nil, want workspace conflict")
	}
}

func TestValidateInteractiveWorkspaceInputRejectsConflict(t *testing.T) {
	homePath := t.TempDir()
	profilePath := filepath.Join(homePath, ".gogoclaw")
	if err := os.MkdirAll(profilePath, 0755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	sharedWorkspace := filepath.Join(homePath, "shared-workspace")
	existingConfig := config.CreateDefaultConfig()
	existingConfig.Agents.Profiles["default"] = config.ProfileConfig{
		Workspace:         sharedWorkspace,
		Provider:          "codex",
		Model:             "gpt-5.4",
		MaxTokens:         512,
		Temperature:       0.1,
		MaxToolIterations: 4,
		MemoryWindow:      10,
		MaxRetryTimes:     1,
	}
	encoded, err := json.Marshal(existingConfig)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(profilePath, configFileName), encoded, 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	ctx := &onboardContext{
		ProfilePath: profilePath,
		ProfileName: "worker",
	}
	if err := validateInteractiveWorkspaceInput(sharedWorkspace, ctx, homePath); err == nil {
		t.Fatal("validateInteractiveWorkspaceInput() error = nil, want workspace conflict")
	}
}

func TestWriteConfigUpsertsNamedProfileAndPreservesExistingProfiles(t *testing.T) {
	profilePath := t.TempDir()
	configPath := filepath.Join(profilePath, configFileName)
	existingConfig := config.CreateDefaultConfig()
	existingConfig.Agents.Profiles["default"] = config.ProfileConfig{
		Workspace:         filepath.Join(t.TempDir(), "default-workspace"),
		Provider:          "codex",
		Model:             "gpt-5.4",
		MaxTokens:         512,
		Temperature:       0.1,
		MaxToolIterations: 4,
		MemoryWindow:      10,
		MaxRetryTimes:     1,
	}
	encoded, err := json.Marshal(existingConfig)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(configPath, encoded, 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	ctx := &onboardContext{
		ProfilePath: profilePath,
		ProfileName: "worker",
		Workspace:   filepath.Join(t.TempDir(), "worker-workspace"),
		Provider:    "openrouter",
		Model:       "openai/gpt-4.1",
		APIKey:      "worker-token",
	}

	writtenConfig, err := writeConfig(ctx)
	if err != nil {
		t.Fatalf("writeConfig() error = %v", err)
	}
	if writtenConfig.Agents.Profiles["default"].Workspace != existingConfig.Agents.Profiles["default"].Workspace {
		t.Fatalf("default profile workspace = %q, want preserved %q", writtenConfig.Agents.Profiles["default"].Workspace, existingConfig.Agents.Profiles["default"].Workspace)
	}
	workerProfile, ok := writtenConfig.Agents.Profiles["worker"]
	if !ok {
		t.Fatal("worker profile missing after upsert")
	}
	if workerProfile.Workspace != ctx.Workspace {
		t.Fatalf("worker workspace = %q, want %q", workerProfile.Workspace, ctx.Workspace)
	}
	if workerProfile.Provider != ctx.Provider {
		t.Fatalf("worker provider = %q, want %q", workerProfile.Provider, ctx.Provider)
	}
	if workerProfile.Model != ctx.Model {
		t.Fatalf("worker model = %q, want %q", workerProfile.Model, ctx.Model)
	}
}

func TestOnboardCreatesWorkspaceBootstrapFiles(t *testing.T) {
	profilePath := filepath.Join(t.TempDir(), "profile")
	workspacePath := filepath.Join(t.TempDir(), "workspace")
	ctx := &onboardContext{
		ProfilePath: profilePath,
		Workspace:   workspacePath,
		Provider:    "codex",
		Model:       "gpt-5.4",
	}

	if err := onboard(ctx); err != nil {
		t.Fatalf("onboard() error = %v", err)
	}

	for _, fileName := range []string{"AGENTS.md", "SOUL.md", "TOOLS.md", "USER.md", "HEARTBEAT.md"} {
		if _, err := os.Stat(filepath.Join(workspacePath, fileName)); err != nil {
			t.Fatalf("workspace file %s missing: %v", fileName, err)
		}
	}
	if _, err := os.Stat(filepath.Join(workspacePath, "sqlite-vec", "store.db")); err != nil {
		t.Fatalf("sqlite-vec store missing: %v", err)
	}
}
