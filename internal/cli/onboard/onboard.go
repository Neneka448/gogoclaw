package onboard

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/Neneka448/gogoclaw/internal/cli/auth"
	"github.com/Neneka448/gogoclaw/internal/config"
	workspacepkg "github.com/Neneka448/gogoclaw/internal/workspace"
	"github.com/charmbracelet/huh"
)

const configFileName = "config.json"

type OnboardOptions struct {
	ProfilePath string
	ProfileName string
	Provider    string
	Model       string
	APIKey      string
	Workspace   string
	Interactive bool
}

type onboardContext struct {
	ProfilePath string
	ProfileName string
	Provider    string
	Model       string
	APIKey      string
	Workspace   string
}

func RunOnboard(options OnboardOptions) error {
	onboardCtx := onboardContext{
		ProfilePath: options.ProfilePath,
		ProfileName: options.ProfileName,
		Provider:    options.Provider,
		Model:       options.Model,
		APIKey:      options.APIKey,
		Workspace:   options.Workspace,
	}

	if options.Interactive {
		if err := interactiveOnboard(&onboardCtx); err != nil {
			return err
		}
	}

	if err := onboard(&onboardCtx); err != nil {
		return err
	}

	return nil
}

func interactiveOnboard(ctx *onboardContext) error {
	tmpCtx := *ctx
	homePath, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get user home directory: %w", err)
	}

	err = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("What profile name do you want to create or update?").
				Value(&tmpCtx.ProfileName).
				Validate(validateProfileName),
			huh.NewInput().
				Title("Which directory do you decide to store your config file? (Default: ~/.gogoclaw, so ~/.gogoclaw/config.json is the default profile, Recommended use ~ as path prefix to store your own config file)").
				Value(&tmpCtx.ProfilePath),
			huh.NewSelect[string]().
				Title("Which provider do you want to use?").
				Options(huh.NewOption("OpenRouter", "openrouter"), huh.NewOption("Codex", "codex")).
				Value(&tmpCtx.Provider),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Which model do you want to use? (empty to skip)").
				Value(&tmpCtx.Model),
		).WithHideFunc(func() bool {
			return tmpCtx.Provider != "openrouter"
		}),

		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Which model do you want to use?").
				Options(huh.NewOption("GPT-5.4", "openai-codex/gpt-5.4"), huh.NewOption("GPT-5.3-codex", "openai-codex/gpt-5.3-codex")).
				Value(&tmpCtx.Model),
		).WithHideFunc(func() bool {
			return tmpCtx.Provider != "codex"
		}),

		huh.NewGroup(
			huh.NewInput().
				Title("Enter your API key").
				Value(&tmpCtx.APIKey),
		).WithHideFunc(func() bool {
			return tmpCtx.Provider != "openrouter"
		}),
	).Run()

	if err != nil {
		return err
	}

	if tmpCtx.Provider == "codex" {
		var authNow bool
		err := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title("Do you want to authenticate now?").
					Value(&authNow),
			),
		).Run()
		if err != nil {
			return err
		}
		if authNow {
			if token, err := auth.AuthCodex(); err != nil {
				return err
			} else {
				tmpCtx.APIKey = token
			}
		} else {
			fmt.Println("You can authenticate later: `gogoclaw auth --provider codex`")
		}
	}

	err = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Where you decide to store your workspace, relative to your config directory or use absolute path to another directory? (Default: config-dir-you-chooose/workspace, Recommended use ~ as path prefix to store your own workspace)").
				Value(&tmpCtx.Workspace).
				Validate(func(value string) error {
					return validateInteractiveWorkspaceInput(value, &tmpCtx, homePath)
				}),
		),
	).Run()
	if err != nil {
		return err
	}
	tmpCtx.ProfilePath = resolveInteractiveProfilePath(tmpCtx.ProfilePath, homePath)
	tmpCtx.Workspace = resolveInteractiveWorkspacePath(tmpCtx.Workspace, tmpCtx.ProfilePath, homePath)

	*ctx = tmpCtx

	return nil

}

func onboard(ctx *onboardContext) error {
	homePath, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get user home directory: %w", err)
	}

	normalizeContextPaths(ctx, homePath)

	if err := prepareProfilePath(ctx.ProfilePath); err != nil {
		return err
	}

	if err := prepareWorkspacePath(ctx.Workspace); err != nil {
		return err
	}
	if err := workspacepkg.EnsureBootstrapFiles(ctx.Workspace); err != nil {
		return fmt.Errorf("prepare workspace bootstrap files: %w", err)
	}
	if err := workspacepkg.EnsureMemorySkill(ctx.Workspace); err != nil {
		return fmt.Errorf("prepare memory skill: %w", err)
	}
	if err := workspacepkg.EnsureDefaultSkills(ctx.Workspace); err != nil {
		return fmt.Errorf("prepare workspace default skills: %w", err)
	}

	if _, err := writeConfig(ctx); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

func writeConfig(ctx *onboardContext) (*config.SysConfig, error) {
	configPath := filepath.Join(ctx.ProfilePath, configFileName)
	manager := config.NewConfigManager(configPath)
	sysConfig, err := manager.ApplyOnboardUpdate(config.OnboardUpdate{
		ProfileName: ctx.ProfileName,
		Workspace:   ctx.Workspace,
		Provider:    ctx.Provider,
		Model:       ctx.Model,
		APIKey:      ctx.APIKey,
	})
	if err != nil {
		return nil, err
	}
	slog.Info("Config file created", "path", configPath)

	return &sysConfig, nil
}

func normalizeContextPaths(ctx *onboardContext, homePath string) {
	if strings.TrimSpace(ctx.ProfilePath) == "" {
		ctx.ProfilePath = filepath.Join(homePath, ".gogoclaw")
		slog.Warn("ProfilePath not set, use default", "path", ctx.ProfilePath)
	}
	ctx.ProfilePath = expandHomePath(ctx.ProfilePath, homePath)
	if strings.TrimSpace(ctx.ProfileName) == "" {
		ctx.ProfileName = "default"
	}

	if ctx.Workspace == "" {
		ctx.Workspace = filepath.Join(ctx.ProfilePath, "workspace")
		slog.Warn("Workspace not set, use default", "path", ctx.Workspace)
	}
	ctx.Workspace = expandHomePath(ctx.Workspace, homePath)
}

func expandHomePath(path string, homePath string) string {
	switch {
	case path == "~":
		return homePath
	case strings.HasPrefix(path, "~/"):
		return filepath.Join(homePath, path[2:])
	default:
		return path
	}
}

func prepareProfilePath(profilePath string) error {
	info, err := os.Stat(profilePath)
	switch {
	case os.IsNotExist(err):
		if err := os.MkdirAll(profilePath, 0755); err != nil {
			return fmt.Errorf("create profile directory %s: %w", profilePath, err)
		}
		slog.Info("Profile Directory created", "path", profilePath)
		return nil
	case err != nil:
		return fmt.Errorf("stat profile directory %s: %w", profilePath, err)
	case !info.IsDir():
		return fmt.Errorf("profile path is not a directory: %s", profilePath)
	}

	configPath := filepath.Join(profilePath, configFileName)
	if _, err := os.Stat(configPath); err == nil {
		slog.Info("Config file exists, will update", "path", configPath)
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat config file %s: %w", configPath, err)
	}

	slog.Info("Config file not exists, will create one", "path", configPath)
	return nil
}

func prepareWorkspacePath(workspacePath string) error {
	info, err := os.Stat(workspacePath)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(workspacePath, 0755); err != nil {
			return fmt.Errorf("create workspace directory %s: %w", workspacePath, err)
		}
		slog.Info("Workspace created", "path", workspacePath)
		return nil
	} else if err != nil {
		return fmt.Errorf("stat workspace path %s: %w", workspacePath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace path is not a directory: %s", workspacePath)
	}

	slog.Info("Workspace exists, will reuse", "path", workspacePath)
	return nil
}

func validateProfileName(profileName string) error {
	if strings.TrimSpace(profileName) == "" {
		return fmt.Errorf("profile name is required")
	}
	return nil
}

func resolveInteractiveProfilePath(profilePath string, homePath string) string {
	trimmed := strings.TrimSpace(profilePath)
	if trimmed == "" {
		trimmed = filepath.Join(homePath, ".gogoclaw")
	}
	return expandHomePath(trimmed, homePath)
}

func resolveInteractiveWorkspacePath(workspace string, profilePath string, homePath string) string {
	trimmed := strings.TrimSpace(workspace)
	if trimmed == "" {
		trimmed = filepath.Join(resolveInteractiveProfilePath(profilePath, homePath), "workspace")
	}
	return expandHomePath(trimmed, homePath)
}

func validateInteractiveWorkspaceInput(workspace string, ctx *onboardContext, homePath string) error {
	if err := validateProfileName(ctx.ProfileName); err != nil {
		return err
	}
	resolvedProfilePath := resolveInteractiveProfilePath(ctx.ProfilePath, homePath)
	resolvedWorkspace := resolveInteractiveWorkspacePath(workspace, resolvedProfilePath, homePath)
	manager := config.NewConfigManager(filepath.Join(resolvedProfilePath, configFileName))
	_, err := manager.PreviewOnboardUpdate(config.OnboardUpdate{
		ProfileName: strings.TrimSpace(ctx.ProfileName),
		Workspace:   resolvedWorkspace,
		Provider:    strings.TrimSpace(ctx.Provider),
		Model:       strings.TrimSpace(ctx.Model),
		APIKey:      strings.TrimSpace(ctx.APIKey),
	})
	return err
}
