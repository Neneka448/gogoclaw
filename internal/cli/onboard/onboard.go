package onboard

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/Neneka448/gogoclaw/internal/cli/auth"
	"github.com/charmbracelet/huh"
)

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
		interactiveOnboard(&onboardCtx)
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
				Title("Where you decide to store your workspace? (Default: ~/.gogoclaw/workspace, Recommended use ~ as path prefix to store your own workspace)").
				Value(&tmpCtx.Workspace),
		),
	).Run()
	if err != nil {
		return err
	}

	return nil
}

func onboard(ctx *onboardContext) error {
	homePath, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if ctx.ProfilePath == "" {
		ctx.ProfilePath = homePath + "/.gogoclaw"
		slog.Warn("ProfilePath not set, use default", "path", ctx.ProfilePath)
	}

	if _, err = os.Stat(ctx.ProfilePath); os.IsNotExist(err) {
		if err := os.MkdirAll(ctx.ProfilePath, 0755); err != nil {
			return err
		}
		slog.Info("Profile Directory created", "path", ctx.ProfilePath)
	} else if err != nil {
		if !os.IsExist(err) {
			slog.Error("Profile Directory exists", "path", ctx.ProfilePath)
		}
		return err
	}

	if ctx.Workspace == "" {
		ctx.Workspace = ctx.ProfilePath + "/workspace"
		slog.Warn("Workspace not set, use default", "path", ctx.Workspace)
	}
	if _, err = os.Stat(ctx.Workspace); os.IsNotExist(err) {
		if err := os.MkdirAll(ctx.Workspace, 0755); err != nil {
			return err
		}
		slog.Info("Workspace created", "path", ctx.Workspace)
	} else if err != nil {
		if !os.IsExist(err) {
			slog.Error("Workspace exists", "path", ctx.Workspace)
		}
		return err
	}

	writeConfig(ctx)

	return nil

}

func writeConfig(ctx *onboardContext) error {
	return nil
}
