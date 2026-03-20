---
name: cron_task
description: "Create, list, update, and delete workspace cron tasks that run the agent on a schedule. Use when the user wants a repeating background task, periodic inbox triage, scheduled reports, recurring agent workflow, or any task that should execute automatically on a timer. Also use when the user asks to manage existing crons — view, pause, resume, change schedule, or remove them."
---

# Cron Task

Manage scheduled agent tasks stored under the workspace `crons/` directory. Each cron is a folder containing a `config.json` (schedule, flags) and a `task.md` (the prompt the agent executes). Scripts handle all CRUD; the `sync_crons` tool reloads the in-memory scheduler after any change.

## When to Use

- User wants something to happen on a recurring schedule
- User asks to create, list, update, pause, resume, or delete a cron
- A skill or workflow needs to register a background cron as part of its protocol

Do NOT use for one-shot work. Run one-time tasks directly.

## Scene Detection — Roleplay Cron

Before creating a task, check whether the cron involves the agent acting under a defined identity. This includes acting as a specific character, maintaining a professional role (programmer, analyst, moderator, support agent), keeping a consistent personality or speaking style across runs, or replying to messages in character.

If the task matches any of the above, call `get_skill("roleplay-cron")` and follow that skill instead. The roleplay-cron skill defines the correct file architecture (SOUL.md / SKILL.md / task.md separation) and anti-patterns to avoid. Do not proceed with the generic flow below.

## Scripts

All scripts live under `skills/cron_task/scripts/`. Run them with `python -m skills.cron_task.scripts.<name>`. Every script requires `--workspace` and outputs JSON to stdout.

### create.py — Create a new cron

```bash
python -m skills.cron_task.scripts.create \
  --workspace {workspace} \
  --cron-id <id> \
  --cron-expression "<expr>" \
  --task "<full task definition>" \
  --enabled \
  --profile-name <profile> \
  --invocation-mode <mode>
```

| Arg | Required | Notes |
|-----|----------|-------|
| `--cron-id` | yes | Stable identifier. Must match `[a-zA-Z0-9][a-zA-Z0-9._-]*` |
| `--cron-expression` | yes | Standard 5-field cron: `minute hour dom month dow` |
| `--task` | yes | Complete task definition (see "Writing a Good Task" below) |
| `--enabled` / `--disabled` | no | Defaults to enabled |
| `--profile-name` | no | Target agent profile for execution |
| `--invocation-mode` | no | `foreground`, `background`, or `cron` |

### list.py — List all crons

```bash
python -m skills.cron_task.scripts.list \
  --workspace {workspace} \
  --enabled-only   # optional: filter to enabled crons
```

### get.py — Get cron details

```bash
python -m skills.cron_task.scripts.get \
  --workspace {workspace} \
  --cron-id <id>
```

Returns config + task content.

### update.py — Update an existing cron (merge semantics)

```bash
python -m skills.cron_task.scripts.update \
  --workspace {workspace} \
  --cron-id <id> \
  --cron-expression "<new expr>" \
  --task "<new task>" \
  --disabled
```

Only the fields you pass are changed; everything else is preserved.

### delete.py — Delete a cron

```bash
python -m skills.cron_task.scripts.delete \
  --workspace {workspace} \
  --cron-id <id>
```

## After Any Change: Sync

After creating, updating, or deleting a cron, call the `sync_crons` tool to reload the in-memory scheduler from disk. Optionally follow up with `execute_cron` to trigger immediate execution.

## Writing a Good Task

The `--task` value becomes the prompt the agent receives on each scheduled run. A vague task produces vague results. Write it as a self-contained instruction block that a future agent — with no memory of this conversation — can execute correctly.

Every task should include:

1. Which skill to load first, if the task depends on one (`get_skill("<name>")`).
2. The execution steps in order.
3. Decision rules: when to act, when to skip, when to stop.
4. Any environment details the agent needs (paths, IDs, config).
5. What artifact or output to produce.

### Task template

```text
First call get_skill("<skill-name>") and follow that skill strictly.

Execution flow:
1. <data gathering or context loading>
2. <analysis or decision step>
3. <action step>

Decision rules:
- Act when <condition>.
- Skip when <condition>.
- Stop after one complete pass.

If no action is needed, say so and stop.
```

### Example: QQ inbox triage

```text
First call get_skill("qqinbox") and follow that skill strictly.

Execution flow:
1. Read recent conversations using the qqinbox workflow.
2. For each conversation, internally summarize: topic, intent, whether a reply is needed.
3. Draft a short reply plan for conversations that need a response.
4. Send replies only when justified by the conversation content.
5. If nothing needs a reply, explicitly say so and stop.

Decision rules:
- Reply when a message is directed at you or asks a question you can answer.
- Skip when the conversation is resolved or does not involve you.
- Stop after processing all unread conversations.
```

### What NOT to write

Under-specified tasks like these will produce poor results:

- "check qq"
- "run every 5 minutes"
- "look and reply if needed"

These lack execution steps, decision rules, and stop conditions. The scheduled agent has no conversation history to fill in the gaps — the task must stand on its own.

## Cron Expression Quick Reference

| Schedule | Expression |
|----------|-----------|
| Every 5 minutes | `*/5 * * * *` |
| Every hour at :15 | `15 * * * *` |
| Daily at 09:30 | `30 9 * * *` |
| Weekdays at 08:00 | `0 8 * * 1-5` |
| Every Sunday at midnight | `0 0 * * 0` |
