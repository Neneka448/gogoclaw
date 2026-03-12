# gogoclaw

gogoclaw is a Go-based coding agent runtime inspired by [OpenClaw](https://github.com/openclaw/openclaw) and [Nanobot](https://github.com/HKUDS/nanobot).

The project focuses on a clean, modular runtime that separates agent orchestration, tool execution, session persistence, provider integration, channel delivery, and workspace bootstrap. The goal is to make the system easy to assemble, extend, and embed into different engineering workflows.

## Status

This repository is in an early, pre-release stage.

What is already implemented:

- Cobra-based CLI entrypoints
- onboarding flow that creates a profile, config file, and starter workspace
- JSON configuration schema with default profile/provider/channel settings
- agent loop with tool-calling support over OpenAI-compatible chat completions
- local session persistence under the workspace
- gateway orchestration for direct CLI execution and long-running channel mode
- built-in tools for file reading, directory listing, terminal execution, active messaging, and workspace skill loading
- workspace bootstrap files for prompt/system context composition
- optional Feishu channel wiring in addition to the CLI channel

What is not positioned as stable yet:

- public APIs and config shape may still evolve
- some command entrypoints are placeholders for future expansion
- the project should currently be treated as source-first and development-oriented

## Why This Project

Compared with a monolithic agent runner, gogoclaw aims to keep the core layers explicit:

- agent: drives the LLM loop and tool execution
- provider: wraps OpenAI-compatible chat completion models
- tools: exposes controlled capabilities to the model
- session: persists conversation state to workspace files
- channels: delivers messages through CLI or external integrations
- gateway: coordinates message flow between channels and the agent runtime
- workspace: stores system prompt fragments, skills, and durable runtime data

This makes it easier to swap providers, add tools, or attach new channels without rewriting the whole runtime.

## Features

- Interactive and non-interactive onboarding
- Default workspace bootstrap with AGENTS.md, SOUL.md, TOOLS.md, USER.md, and HEARTBEAT.md
- OpenAI-compatible provider abstraction
- Built-in provider presets for openrouter and codex
- Codex OAuth login flow via local callback server
- Direct single-turn CLI execution with agent responses streamed through the message bus
- Persistent sessions stored as JSON files inside the workspace
- Workspace-local skill discovery from skills/<skill-name>/SKILL.md
- Tool call visibility in the CLI output
- Configurable tool timeout for terminal execution

## Project Layout

```text
.
├── cmd/                    # CLI entrypoints
├── internal/agent/         # agent loop and tool-call orchestration
├── internal/bootstrap/     # runtime wiring from config to gateway
├── internal/channels/      # CLI and Feishu channels
├── internal/cli/           # onboarding and auth flows
├── internal/config/        # config schema and loading
├── internal/gateway/       # message routing and runtime lifecycle
├── internal/provider/      # OpenAI-compatible provider adapter
├── internal/session/       # workspace-backed session persistence
├── internal/skills/        # workspace skill discovery
├── internal/systemprompt/  # prompt assembly from workspace files
├── internal/tools/         # built-in model tools
└── internal/workspace/     # embedded workspace bootstrap templates
```

## Requirements

- Go 1.26.1 or newer
- an OpenAI-compatible model endpoint or Codex-compatible authentication flow

## Installation

Build from source:

```bash
git clone https://github.com/Neneka448/gogoclaw.git
cd gogoclaw
go build -o gogoclaw .
```

Or use the provided Make target:

```bash
make build
```

## Quick Start

### 1. Create a profile and workspace

Interactive onboarding:

```bash
./gogoclaw onboard --interactive
```

Non-interactive onboarding example:

```bash
./gogoclaw onboard \
	--provider openrouter \
	--model openai/gpt-4.1-mini \
	--apikey "$OPENROUTER_API_KEY"
```

By default this creates:

- profile directory at ~/.gogoclaw
- config file at ~/.gogoclaw/config.json
- workspace at ~/.gogoclaw/workspace

### 2. Run a one-shot agent command

```bash
./gogoclaw agent --message "Summarize the current repository structure"
```

The agent will bootstrap the runtime from the configured profile, load workspace prompt files and skills, then execute the request through the configured model.

### 3. Start the gateway

```bash
./gogoclaw gateway
```

This starts the configured channels and keeps the process alive until interrupted. The CLI channel is enabled by default, and Feishu can be enabled via config.

### 4. Authenticate Codex if needed

```bash
./gogoclaw auth --provider codex
```

This opens a browser-based OAuth flow and stores the token locally for later reuse.

## CLI Commands

Current user-facing commands:

- onboard: initialize config and workspace files
- auth: authenticate an OAuth-backed provider, currently Codex
- agent: send a direct message through the agent runtime
- gateway: start the long-running channel gateway
- version: print build metadata

Global flags:

- --config, -c: override the default config path

## Configuration

The default config file is JSON-based and lives at ~/.gogoclaw/config.json.

Example:

```json
{
  "agents": {
    "profiles": {
      "default": {
        "workspace": "/Users/you/.gogoclaw/workspace",
        "provider": "openrouter",
        "model": "openai/gpt-4.1-mini",
        "maxTokens": 8192,
        "temperature": 0.1,
        "maxToolIterations": 40,
        "memoryWindow": 30,
        "maxRetryTimes": 3
      }
    }
  },
  "embedding": {
    "profiles": {
      "default": {
        "text": {
          "provider": "voyageai",
          "model": "voyage-4-large",
          "outputDimension": 1024
        },
        "modal": {
          "provider": "voyageai",
          "model": "voyage-multimodal-3.5",
          "outputDimension": 1024
        }
      }
    },
    "providers": [
      {
        "name": "voyageai",
        "timeout": 60,
        "baseURL": "",
        "path": "",
        "auth": {
          "token": "<voyage-api-key>"
        }
      }
    ]
  },
  "providers": [
    {
      "name": "openrouter",
      "timeout": 60,
      "baseURL": "",
      "path": "",
      "auth": {
        "token": "<token>"
      }
    }
  ],
  "channels": {
    "cli": {
      "enabled": true
    },
    "feishu": {
      "enabled": false,
      "appId": "",
      "appSecret": "",
      "encryptKey": "",
      "verificationToken": "",
      "allowFrom": ["*"],
      "reactEmoji": "THUMBSUP"
    },
    "sendProgress": true,
    "sendToolHints": true
  },
  "gateway": {
    "port": 8080,
    "host": "127.0.0.1",
    "heartbeat": {
      "interval": 1800,
      "enable": true
    }
  },
  "tools": []
}
```

Notes:

- the default agent profile name is default
- provider lookup is based on the profile's provider field
- embedding models are configured separately under the embedding section
- terminal tool timeout can be configured through the tools array
- if no custom workspace is provided during onboarding, it defaults to <profile>/workspace

## Workspace Conventions

Onboarding bootstraps a workspace with several prompt and instruction files:

- AGENTS.md: high-level agent instructions
- SOUL.md: persona and values
- TOOLS.md: tool usage notes
- USER.md: durable user preferences
- HEARTBEAT.md: reserved workspace state file

Additional runtime conventions:

- skills are loaded from skills/<name>/SKILL.md
- sessions are persisted under sessions/<session-id>.json
- archived sessions are written when the user sends /new

## Built-in Tools

The runtime currently registers these tools for the model:

- read_file: read files from inside the workspace with optional line ranges
- list_dir: list directory contents inside the workspace
- terminal: run non-interactive shell commands inside the workspace
- message: actively send a message back to the user through the channel layer
- get_skill: load a workspace skill by name

All file and terminal tools are workspace-scoped to prevent escaping the configured workspace root.

## How It Works

At a high level, the runtime bootstraps like this:

1. Load config and resolve the default agent profile.
2. Create the provider, message bus, tool registry, session manager, skill registry, and system prompt service.
3. Register enabled channels.
4. Start an agent loop for each inbound message.
5. Persist messages to the workspace-backed session.
6. Execute tool calls until the model returns a final response or hits the iteration limit.

## Development

Run tests:

```bash
go test ./...
```

Build with version metadata:

```bash
make build
```

Fast local development:

```bash
make test
```

Fast local build:

```bash
make build
```

The repository already includes tests for core areas such as bootstrap, channels, config, gateway, provider normalization, sessions, skills, tools, and workspace bootstrap files.

## Contributing

Before contributing, read [AGENTS.md](AGENTS.md).

This repository expects changes to stay focused, tested, and aligned with the layer boundaries already present in the codebase.

Commit messages should follow the convention documented in AGENTS.md:

- use predicate(scope): summary
- keep the summary imperative, lowercase, and concise
- prefer scopes that match the primary directory or layer being changed

If your change affects behavior, add or update the relevant tests before submitting it.

## Roadmap Direction

The current codebase is centered on getting the runtime spine right. Likely next areas of growth include:

- richer provider integrations
- more channel adapters
- expanded tool surface
- stronger memory and session management
- more complete operational commands around config, status, and provider management
