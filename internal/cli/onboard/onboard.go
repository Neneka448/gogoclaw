package onboard

import (
	"encoding/json"
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
	Provider    string
	Model       string
	APIKey      string
	Workspace   string
	Interactive bool
}

type onboardContext struct {
	ProfilePath string
	Provider    string
	Model       string
	APIKey      string
	Workspace   string
}

func RunOnboard(options OnboardOptions) error {
	onboardCtx := onboardContext{
		ProfilePath: options.ProfilePath,
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

	err := huh.NewForm(
		huh.NewGroup(
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
				Value(&tmpCtx.Workspace),
		),
	).Run()
	if err != nil {
		return err
	}

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

	if err := writeConfig(ctx); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

func writeConfig(ctx *onboardContext) error {
	defaultConfig := config.CreateDefaultConfig()
	applyOnboardContext(&defaultConfig, ctx)

	configPath := filepath.Join(ctx.ProfilePath, configFileName)
	configFile, err := os.OpenFile(configPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return fmt.Errorf("create config file %s: %w", configPath, err)
	}
	defer configFile.Close()

	encoder := json.NewEncoder(configFile)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(defaultConfig); err != nil {
		return fmt.Errorf("encode config file %s: %w", configPath, err)
	}
	slog.Info("Config file created", "path", configPath)

	return nil
}

func normalizeContextPaths(ctx *onboardContext, homePath string) {
	if ctx.ProfilePath == "" {
		ctx.ProfilePath = filepath.Join(homePath, ".gogoclaw")
		slog.Warn("ProfilePath not set, use default", "path", ctx.ProfilePath)
	}
	ctx.ProfilePath = expandHomePath(ctx.ProfilePath, homePath)

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
		slog.Error("Config file exists", "path", configPath)
		return fmt.Errorf("config file exists: %s", configPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat config file %s: %w", configPath, err)
	}

	slog.Info("Config file not exists, will create one", "path", configPath)
	return nil
}

func prepareWorkspacePath(workspacePath string) error {
	if _, err := os.Stat(workspacePath); os.IsNotExist(err) {
		if err := os.MkdirAll(workspacePath, 0755); err != nil {
			return fmt.Errorf("create workspace directory %s: %w", workspacePath, err)
		}
		slog.Info("Workspace created", "path", workspacePath)
		return nil
	} else if err != nil {
		return fmt.Errorf("stat workspace path %s: %w", workspacePath, err)
	}

	slog.Error("Workspace exists", "path", workspacePath)
	return fmt.Errorf("workspace exists: %s", workspacePath)
}

func applyOnboardContext(defaultConfig *config.SysConfig, ctx *onboardContext) {
	defaultProfile := defaultConfig.Agents.Profiles["default"]
	defaultProfile.Workspace = ctx.Workspace
	defaultProfile.Provider = ctx.Provider
	defaultProfile.Model = ctx.Model
	defaultConfig.Agents.Profiles["default"] = defaultProfile

	for i := range defaultConfig.Providers {
		if defaultConfig.Providers[i].Name == ctx.Provider {
			defaultConfig.Providers[i].Auth.Token = ctx.APIKey
			break
		}
	}
}
