
## Commit Message Convention

All commits **must** follow this format:

```
predicate(scope): summary

- detail point one
- detail point two
```

- The **summary** line should be concise (≤ 72 characters), written in the imperative mood, lowercase, no trailing period.
- The **bullet points** are optional for trivial changes but required when the commit touches multiple concerns.

---

## Predicates

| Predicate  | When to use |
|------------|-------------|
| `feat`     | Introduces a new feature visible to users or callers |
| `fix`      | Fixes a bug or incorrect behavior |
| `chore`    | Maintenance tasks: dependency updates, config changes, scaffolding |
| `refactor` | Code restructuring with no behavior change |
| `perf`     | Performance improvement |
| `style`    | Formatting, whitespace, or lint-only changes (no logic change) |
| `test`     | Adding or updating tests |
| `docs`     | Documentation only |
| `build`    | Changes to build system, bundler config, or CI pipeline |
| `revert`   | Reverts a previous commit |