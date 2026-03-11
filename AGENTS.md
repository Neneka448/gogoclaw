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

Recommended scopes for this repository:

| Scope         | Use for                                                                                  |
| ------------- | ---------------------------------------------------------------------------------------- |
| `cli`         | User-facing command flow across `cmd/` and `internal/cli/` when the change is broad      |
| `cli/onboard` | Onboarding command and workflow, including `cmd/onboard.go` and `internal/cli/onboard/*` |
| `cli/auth`    | Authentication flow under `internal/cli/auth/*`                                          |
| `config`      | Configuration schema, defaults, and loaders under `internal/config/*`                    |
| `version`     | Version metadata under `internal/version/*`                                              |
| `root`        | Repository entrypoints such as `main.go` and `cmd/root.go`                               |

Examples:

- `fix(cli/onboard): wire onboard command to config creation`
- `refactor(config): centralize default profile values`
- `docs(cli): document command behavior`

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
