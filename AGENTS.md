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
| `config`   | Configuration schema, defaults, and loaders under `internal/config/`             |
| `agent`    | Agent runtime, orchestration, and future code under `internal/agent/`            |
| `memory`   | Memory management and persistence under `internal/memory/`                       |
| `tools`    | Tool registry, execution, and adapters under `internal/tools/`                   |
| `session`  | Session lifecycle and state under `internal/session/`                            |
| `gateway`  | Gateway server and transport flow under `cmd/gateway.go` and `internal/gateway/` |
| `channels` | Channel integrations and dispatch under `internal/channels/`                     |
| `provider` | Model/provider integrations under `cmd/provider.go` and `internal/provider/`     |
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
