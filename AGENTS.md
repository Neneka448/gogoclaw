## Commit Message Convention

All commits **must** follow this format:

```
predicate(scope): summary

- detail point one
- detail point two
```

- The **summary** line should be concise (≤ 72 characters), written in the imperative mood, lowercase, no trailing period.
- The **bullet points** are optional for trivial changes but required when the commit touches multiple concerns.

### Scope Convention

- Prefer a scope that matches the primary directory or layer being changed.
- Scopes may be hierarchical using `/` when that makes the target area clearer.
- If one change spans multiple files in the same feature path, use the most specific shared scope.
- If a change crosses unrelated layers, use the dominant user-facing area instead of listing multiple scopes.
- If one change legitimately spans multiple primary layers, you may list multiple scopes separated by commas, such as `feat(cli,config): ...`.
- Keep multi-scope commit headers short and use them only when the change truly affects more than one primary layer.

Recommended scopes for this repository:

| Scope      | Use for                                                                          |
| ---------- | -------------------------------------------------------------------------------- |
| `root`     | Repository entrypoints such as `main.go` and `cmd/root.go`                       |
| `cli`      | User-facing command flow across `cmd/` and `internal/cli/`                       |
| `bootstrap`| Runtime assembly under `internal/bootstrap/`                                      |
| `config`   | Configuration schema, defaults, and loaders under `internal/config/`             |
| `agent`    | Agent runtime, orchestration, and future code under `internal/agent/`            |
| `memory`   | Memory management and persistence under `internal/memory/`                       |
| `tools`    | Tool registry, execution, and adapters under `internal/tools/`                   |
| `session`  | Session lifecycle and state under `internal/session/`                            |
| `gateway`  | Gateway server and transport flow under `cmd/gateway.go` and `internal/gateway/` |
| `channels` | Channel integrations and dispatch under `internal/channels/`                     |
| `message_bus` | Message queue and transport contracts under `internal/message_bus/`           |
| `provider` | Model/provider integrations under `cmd/provider.go` and `internal/provider/`     |
| `skills`   | Skill discovery and workspace skill loading under `internal/skills/`             |
| `systemprompt` | Prompt assembly from workspace files under `internal/systemprompt/`          |
| `vectorstore` | Embedding persistence and sqlite-vec integration under `internal/vectorstore/` |
| `workspace` | Workspace bootstrap templates and file scaffolding under `internal/workspace/`   |
| `utils`    | Shared helpers under `internal/utils/`                                           |

Examples:

- `feat(cli): wire onboard command to config creation`
- `refactor(config): centralize default profile values`
- `feat(provider): add provider integration scaffolding`
- `docs(utils): document shared helper conventions`

---

## Predicates

| Predicate  | When to use                                                        |
| ---------- | ------------------------------------------------------------------ |
| `feat`     | Introduces a new feature visible to users or callers               |
| `fix`      | Fixes a bug or incorrect behavior                                  |
| `chore`    | Maintenance tasks: dependency updates, config changes, scaffolding |
| `refactor` | Code restructuring with no behavior change                         |
| `perf`     | Performance improvement                                            |
| `style`    | Formatting, whitespace, or lint-only changes (no logic change)     |
| `test`     | Adding or updating tests                                           |
| `docs`     | Documentation only                                                 |
| `build`    | Changes to build system, bundler config, or CI pipeline            |
| `revert`   | Reverts a previous commit                                          |

---

## Runtime and Workflow Notes

- Runtime wiring lives in `internal/bootstrap/bootstrap.go`; it resolves the `default` agent/embedding profiles, builds providers, registers channels/tools, and constructs the gateway context.
- User-facing command flow is currently implemented in `cmd/onboard.go`, `cmd/auth.go`, `cmd/agent.go`, `cmd/gateway.go`, and `cmd/version.go`. `cmd/config.go`, `cmd/provider.go`, and `cmd/status.go` are placeholders.
- `cmd/agent.go` uses one-shot processing through `Gateway.DirectProcessAndReturn`; `cmd/gateway.go` runs long-lived channel processing through `Gateway.Start`.
- In `internal/agent/agent_loop.go`, `/new` archives and resets the session, tool iterations are bounded by profile `maxToolIterations`, and unfinished loops emit a max-iterations assistant message.

## Workspace Prompt Composition

- `internal/systemprompt/service.go` builds `<system_prompt>` from workspace `AGENTS.md`, `SOUL.md`, `TOOLS.md`, and `USER.md`; `HEARTBEAT.md` is bootstrap state and not part of prompt assembly.
- Skills are loaded from `skills/<name>/SKILL.md` (`internal/skills/`), and skill metadata is injected into the system prompt to drive conditional `get_skill` usage.
- `internal/workspace/bootstrap_files.go` only creates missing bootstrap files, so existing workspace prompt files are intentionally preserved on re-run.

## Tool and Integration Constraints

- Built-in model tools are registered in bootstrap: `read_file`, `list_dir`, `terminal`, `message`, and `get_skill`.
- `read_file`, `list_dir`, and `terminal` are workspace-scoped and reject path traversal outside the configured workspace.
- `terminal` executes `bash -lc` non-interactively with a default 30s timeout (`internal/tools/terminal.go`), and bootstrap can override timeout via config tool entry `name: terminal`.
- Gateway lifecycle starts/stops `VectorStore` with channels (`internal/gateway/gateway.go`); sqlite-vec data is stored under `<workspace>/sqlite-vec` (`internal/vectorstore/sqlite_vec_service.go`).

## Local Development Commands

- Build with `make build` and run full tests with `make test` (currently `go test ./...`).
- Use package-level test runs for focused iteration, for example `go test ./internal/agent ./internal/gateway`.

---

## Writing Code

### Project Layout

All runtime code lives under `internal/`. Each subdirectory is a self-contained Go package with its own interface. The dependency graph flows inward: `cmd/` → `bootstrap/` → `agent/` → everything else. Never import a sibling package's internals — depend on its exported interface.

```
cmd/           CLI entrypoints — thin wrappers that call bootstrap or gateway
internal/
  agent/       ReAct loop + InvocationService (profile runtime lifecycle)
  bootstrap/   Wires config → providers → channels → gateway
  channels/    Inbound/outbound channel adapters (CLI, Feishu)
  cli/         Onboarding and auth interactive flows
  config/      JSON config schema, defaults, ConfigManager
  context/     SystemContext (central dependency bag) + InvocationRequest
  cron/        Cron scheduler + workspace-backed cron storage
  gateway/     Message routing, worker pool, lifecycle
  mcp/         Model Context Protocol client (stdio + HTTP transports)
  memory/      5W1H+R memory extraction, graph storage, recall
  message_bus/ Inbound/Outbound queues + OutputSink abstraction
  provider/    LLM and embedding provider adapters (OpenAI-compatible)
  session/     Workspace-backed session persistence (JSON files)
  skills/      Skill discovery from workspace skills/<name>/SKILL.md
  systemprompt/ Prompt assembly from AGENTS.md, SOUL.md, TOOLS.md, USER.md + skills
  tools/       Built-in tool implementations + ToolRegistry
  vectorstore/ SQLite-vec backed embedding storage
  workspace/   Embedded bootstrap templates + file scaffolding
  utils/       Shared helpers (perf logging, etc.)
```

### Adding a New Built-in Tool

1. Create `internal/tools/<tool_name>.go`. Implement the `Tool` interface:

```go
type Tool interface {
    Execute(args string) (string, error)
}
```

`args` is the raw JSON string from the LLM tool call. Parse it yourself with `json.Unmarshal`. Return a string result or error.

2. Optionally implement additional interfaces if the tool needs richer lifecycle:

| Interface                  | Purpose                                              |
| -------------------------- | ---------------------------------------------------- |
| `MessageContextTool`       | Receive the inbound message before each turn         |
| `TurnLifecycleTool`        | `StartTurn()` called at the beginning of each turn   |
| `OutboundSuppressionTool`  | Suppress tool result from being sent to the channel  |
| `SentMessageTool`          | Report whether the tool sent a message this turn     |

3. Build a `ToolDescriptor` with the OpenAI function schema:

```go
func NewMyTool(workspace string) tools.ToolDescriptor {
    return tools.ToolDescriptor{
        Name: "my_tool",
        Tool: &myTool{workspace: workspace},
        ToolForLLM: openai.Tool{
            Type: openai.ToolTypeFunction,
            Function: &openai.FunctionDefinition{
                Name:        "my_tool",
                Description: "What this tool does",
                Parameters:  jsonschema.Definition{ /* ... */ },
            },
        },
        Timeout: tools.DefaultToolExecutionTimeout,
    }
}
```

4. Register it in `buildInvocationToolRegistry()` inside `internal/agent/invocation.go`:

```go
myTool := tools.NewMyTool(workspace)
myTool.Timeout = resolveInvocationToolTimeout(toolConfigIndex, "my_tool", tools.DefaultToolExecutionTimeout)
if err := registry.RegisterTool("my_tool", myTool); err != nil {
    return nil, err
}
```

5. If the tool needs config-driven timeout, add a `ToolConfig` entry in the user's config JSON under `"tools"`.

### Adding a New Config Field

1. Add the field to the relevant struct in `internal/config/schema.go`.
2. Set a sensible default in `CreateDefaultConfig()`.
3. If validation is needed, add a `Validate*` function next to the struct.
4. The `ConfigManager` loads from JSON and caches — no extra wiring needed for reads.
5. Consume the new field where needed (usually in `internal/agent/invocation.go` during `buildProfileRuntime` or `buildExecutionContext`).

### Adding a New Service to SystemContext

1. Define the service interface in its own package under `internal/<service>/`.
2. Add the interface field to `SystemContext` in `internal/context/system_context.go`.
3. Construct the service in `buildProfileRuntime()` in `internal/agent/invocation.go`.
4. If the service needs startup/shutdown, wire it into `profileRuntime.ensureReady()` and `profileRuntime.close()`.
5. Access it via `al.context.<ServiceName>` in the agent loop or pass it to tools that need it.

### Adding a New Channel

1. Create `internal/channels/<channel_name>.go` implementing the channel interface.
2. Register it in `internal/bootstrap/bootstrap.go` alongside the CLI and Feishu channels.
3. Add config fields to `ChannelsConfig` in `internal/config/schema.go`.
4. The gateway dispatches outbound messages to channels via `ChannelRegistry` — no agent loop changes needed.

### Key Patterns

- **Workspace-scoped**: Tools like `read_file`, `list_dir`, `terminal` reject paths outside the workspace. New tools that touch the filesystem should do the same.
- **OutputSink abstraction**: Foreground mode writes to `MessageBusOutputSink`; background/cron mode uses `NoopOutputSink`. Tools that emit messages should go through `OutputSink`, not `MessageBus` directly.
- **Cron as agent launcher**: Cron is a profile-aware, mode-aware, file-oriented agent launcher — not a workflow engine or a second agent runtime. Each cron task stores `profileName` and `invocationMode` in its config, and at execution time these are passed through the unified `InvocationService` path (`executeCronRequest` → `buildExecutionContext`). Execution artifacts (manifest, session references) are written to the cron execution directory. Do not add orchestration logic, task DAGs, or channel coupling into the cron layer.
- **Lazy initialization**: `profileRuntime` uses `sync.Once` for startup. Services are built once per profile and cached.
- **Retry with backoff**: LLM calls use exponential backoff with jitter (`chatCompletionWithRetry`). Follow this pattern for any external call that can transiently fail.
- **Tool filtering**: Profiles can set `allowedTools` / `forbiddenTools`. `FilteredRegistry` wraps the inner registry. Wildcard `"*"` is supported.

---

## Writing Skills

Skills are the primary extension mechanism. They are natural-language protocols that teach the agent how to handle a specific domain or task. There is no "scenes" abstraction — skills fill that role.

### Skill File Structure

A skill lives in `<workspace>/skills/<skill-name>/SKILL.md`. The directory name is the skill name.

```
skills/
  my-skill/
    SKILL.md              # Required — the skill protocol
    references/           # Optional — supporting docs the skill can read
    scripts/              # Optional — shell scripts the skill can execute
    templates/            # Optional — file templates the skill can use
```

### SKILL.md Format

```markdown
---
name: my-skill
description: One-line summary used for skill matching in the system prompt
trigger: When to activate this skill (natural language)
---

# My Skill

## When to Use
Describe the scenarios where this skill applies.

## Protocol
Step-by-step instructions the agent should follow.

## References
Point to files in the references/ directory if needed.
```

- The YAML frontmatter is parsed and injected into the system prompt as JSON metadata.
- The agent sees skill names + metadata and decides whether to call `get_skill` to load the full content.
- The body after the frontmatter is the full protocol — write it as instructions the agent will follow literally.

### How Skills Are Loaded at Runtime

1. `workspace.EnsureDefaultSkills()` deploys embedded template skills on first run and keeps bundled skill resources synced on later runs while preserving workspace `SKILL.md` files.
2. `skills.LoadWorkspaceSkills(workspace)` scans `skills/*/SKILL.md`, parses frontmatter, and builds the registry.
3. `systemprompt.Service.Build()` injects skill names + metadata into the `<skills>` section of the system prompt.
4. When the agent decides a skill is relevant, it calls the `get_skill` tool which returns the full SKILL.md content.
5. The agent then follows the protocol in the skill body.

### Adding a Default (Embedded) Skill

To ship a skill as part of the gogoclaw binary:

1. Create the skill directory under `internal/workspace/templates/skills/<skill-name>/`.
2. Add `SKILL.md` with frontmatter + protocol.
3. Add any supporting files (scripts, references, templates).
4. The `//go:embed templates/*.md templates/skills` directive in `bootstrap_files.go` picks it up automatically.
5. `EnsureDefaultSkills()` deploys it to the workspace on first run. Existing `SKILL.md` files are preserved, while bundled scripts/references/assets are refreshed from the embedded defaults.

### Writing Effective Skills

- Keep the trigger description precise — the agent uses it to decide relevance.
- Write the protocol as imperative steps, not explanations.
- If the skill needs shell commands, put them in `scripts/` and reference them from the protocol.
- Skills can reference other workspace files via `read_file` — instruct the agent to do so in the protocol.
- A skill can use the cron-task scripts and `sync_crons` tool to schedule follow-up work, enabling autonomous multi-step workflows.

---

## When and How to Change Each Module

Use this as a decision guide. Find the behavior you want to change, then go to the right module.

| I want to…                                          | Change this module         | Key file(s)                                      |
| --------------------------------------------------- | -------------------------- | ------------------------------------------------ |
| Change how the agent reasons / loops over tools      | `agent`                    | `agent_loop.go`                                  |
| Add/remove a built-in tool                           | `tools` + `agent`          | `tools/<name>.go`, `invocation.go` (registration)|
| Change tool timeout or filtering                     | `config` + `agent`         | `schema.go`, `invocation.go`                     |
| Add a new LLM or embedding provider                  | `provider`                 | `llm_provider.go` or `embedding_provider.go`     |
| Add a new chat channel (Slack, Discord, etc.)        | `channels` + `bootstrap`   | `channels/<name>.go`, `bootstrap.go`             |
| Change how messages are queued or routed             | `message_bus` or `gateway`  | `message_bus.go`, `sink.go`, `gateway.go`        |
| Change the system prompt structure                   | `systemprompt`             | `service.go`                                     |
| Add a new workspace bootstrap file                   | `workspace`                | `bootstrap_files.go` + `templates/`              |
| Ship a new default skill                             | `workspace`                | `templates/skills/<name>/SKILL.md`               |
| Change session persistence or archival               | `session`                  | `session.go`                                     |
| Change memory extraction or recall                   | `memory`                   | `service.go`, `store.go`                         |
| Change embedding storage                             | `vectorstore`              | `sqlite_vec_service.go`                          |
| Add a config field or profile option                 | `config`                   | `schema.go`                                      |
| Change startup wiring or service initialization      | `bootstrap` + `agent`      | `bootstrap.go`, `invocation.go`                  |
| Add a new invocation mode                            | `context`                  | `system_context.go`                              |
| Change cron scheduling or execution                  | `cron`                     | `cron_service.go`, `cron.go`                     |
| Add/change MCP server integration                    | `mcp`                      | `service.go`                                     |
| Add a new CLI command                                | `cmd`                      | `cmd/<command>.go`                               |

### Cross-cutting changes

Some changes touch multiple modules. Common patterns:

**Adding a new capability the agent can use (tool + config + registration):**
`config/schema.go` → `tools/<name>.go` → `agent/invocation.go` (register) → optionally `workspace/templates/TOOLS.md` (document for the agent)

**Adding a new autonomous behavior (skill + cron):**
`workspace/templates/skills/<name>/SKILL.md` → skill protocol uses cron-task scripts + `sync_crons` → `cron/cron_service.go` executes on schedule

**Adding a new external integration (provider + config + bootstrap):**
`config/schema.go` → `provider/<name>.go` → `agent/invocation.go` (construct) → `context/system_context.go` (if new field needed)

### Module boundaries to respect

- `agent/` owns the ReAct loop and profile runtime. Other packages should not import from `agent/` — they get invoked through `context.InvocationService`.
- `context/` is the shared dependency bag. Keep it as a plain struct with interfaces — no logic.
- `tools/` defines the `Tool` interface and implementations. Tools receive dependencies via constructor, not by reaching into `SystemContext` directly.
- `bootstrap/` is the only place that wires everything together for the gateway path. `agent/invocation.go` does the same for per-profile runtime.
- `workspace/` only handles file scaffolding. It does not read config or know about runtime state.
