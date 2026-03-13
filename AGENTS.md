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

