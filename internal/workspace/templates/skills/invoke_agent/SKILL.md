---
name: invoke_agent
description: "Delegate a task to another agent profile through cron-driven execution with heartbeat monitoring. Completion is returned to the caller profile through agent_bus so the original conversation can continue."
trigger: "When the user asks to delegate, hand off, assign, or dispatch a task to another agent or profile, or when a long-running task should execute independently in the background."
---

# Invoke Agent

Delegate tasks to other agent profiles through cron-driven, file-oriented execution. The delegated task runs independently — no blocking, no direct coupling. Progress is monitored by a heartbeat cron, and completion is returned to the caller profile through agent_bus so the original conversation can resume.

## When to Use

- User asks to delegate a task to a specific agent profile
- A task needs to run independently in the background
- Long-running work that should not block the current conversation
- Coordinating work across multiple agent profiles

Do NOT use for:

- Simple tasks you can handle directly
- Tasks that need immediate synchronous results
- Work within the current profile that does not require delegation

## File Contract

All invocation state lives under `{workspace}/invocations/{invocation-id}/`. See `references/SCHEMAS.md` for manifest.json and status.json schemas.

Completion notifications are delivered back to the caller profile via the **agent_bus** skill — see `skills/agent_bus/`.

## Protocol

### Step 1: Initialize Invocation

```bash
python3 -m skills.invoke_agent.scripts.init_invocation \
  --workspace {workspace} \
  --caller-profile {caller} \
  --target-profile {target} \
  --task-summary "One-line summary"
```

Output: `{"invocation_id": "inv-YYYYMMDD-XXXXXX", "invocation_dir": "..."}`

Capture `invocation_id` and `invocation_dir` for subsequent steps.

### Step 2: Write Task File

```bash
python3 -m skills.invoke_agent.scripts.write_agent_file \
  --invocation-dir {invocation_dir} \
  --type task \
  --content "Full task definition from user request"
```

### Step 3: Write Bootstrap File

Use the default template (recommended):

```bash
python3 -m skills.invoke_agent.scripts.write_agent_file \
  --invocation-dir {invocation_dir} \
  --type bootstrap
```

Or provide custom content:

```bash
python3 -m skills.invoke_agent.scripts.write_agent_file \
  --invocation-dir {invocation_dir} \
  --type bootstrap \
  --content "Custom bootstrap instructions"
```

### Step 4: Write Heartbeat File

```bash
python3 -m skills.invoke_agent.scripts.write_agent_file \
  --invocation-dir {invocation_dir} \
  --type heartbeat \
  --template-vars '{"invocation_id": "{invocation_id}", "workspace": "{workspace}"}'
```

### Step 5: Create Task Cron

Render the task prompt into a file:

```bash
python3 -m skills.invoke_agent.scripts.render_prompt \
  --type task \
  --invocation-id {invocation_id} \
  --invocation-dir {invocation_dir} \
  --workspace {workspace} \
  --output-file {invocation_dir}/task.prompt.md
```

Use the rendered file with `--task-file`:

```bash
python3 -m skills.cron_task.scripts.create \
  --workspace {workspace} \
  --cron-id {invocation_id}-task \
  --cron-expression "*/1 * * * *" \
  --task-file {invocation_dir}/task.prompt.md \
  --profile-name {target_profile} \
  --invocation-mode cron \
  --enabled
```

### Step 6: Create Heartbeat Cron

Render the heartbeat prompt into a file:

```bash
python3 -m skills.invoke_agent.scripts.render_prompt \
  --type heartbeat \
  --invocation-id {invocation_id} \
  --invocation-dir {invocation_dir} \
  --workspace {workspace} \
  --output-file {invocation_dir}/heartbeat.prompt.md
```

Use the rendered file with `--task-file`:

```bash
python3 -m skills.cron_task.scripts.create \
  --workspace {workspace} \
  --cron-id {invocation_id}-heartbeat \
  --cron-expression "*/5 * * * *" \
  --task-file {invocation_dir}/heartbeat.prompt.md \
  --profile-name {target_profile} \
  --invocation-mode cron \
  --enabled
```

Adapt the heartbeat interval to the expected task duration.

### Step 6b: Sync Crons

Call the `sync_crons` tool to load both new crons into the runtime scheduler.

### Step 7: Trigger Immediate Execution

Use `execute_cron` to start the task immediately:

- `cron_id`: `{invocation_id}-task`
- `async`: true

### Step 8: Report Receipt to User

```
Invocation created: {invocation_id}
Target profile: {profile}
Invocation directory: {invocation_dir}
Task cron: {invocation_id}-task (triggered immediately)
Heartbeat cron: {invocation_id}-heartbeat (every N minutes)

The task is now running independently. When it completes, the caller profile will
receive a completion message through agent_bus. You can also inspect
{invocation_dir}/status.json and reports/ for current progress.
```

## Important Rules

1. Always generate a unique invocation ID. Never reuse IDs.
2. Write ALL files before creating crons. The task agent must find files ready on first execution.
3. Always create both task and heartbeat crons. The heartbeat is essential for progress tracking and completion notification.
4. Always trigger immediate execution with `execute_cron` after creating the task cron.
5. Use the target profile for both task and heartbeat crons so they share the same workspace and can read each other's files.
6. Do not create under-specified task.md files. The task definition must be self-contained and actionable.
7. Adapt the heartbeat interval to the expected task duration. Short tasks: `*/2 * * * *`. Long tasks: `*/10 * * * *`.
8. If the user explicitly asks you to delegate or names a target profile, you must use this skill even when the task itself is simple.
9. The delegated task cron is a transport/execution primitive for this workflow. Create it even for one-shot delegated tasks, then disable it after completion.

## Task Lifecycle Scripts

The task and heartbeat agents use these scripts to manage task status instead of writing status.json directly:

| Script | Usage | Description |
|--------|-------|-------------|
| `task_start` | `python3 -m skills.invoke_agent.scripts.task_start --invocation-dir <dir>` | Mark task as running |
| `task_complete` | `python3 -m skills.invoke_agent.scripts.task_complete --invocation-dir <dir>` | Mark task as succeeded |
| `task_fail` | `python3 -m skills.invoke_agent.scripts.task_fail --invocation-dir <dir> --reason "..."` | Mark task as failed |
| `task_status` | `python3 -m skills.invoke_agent.scripts.task_status --invocation-dir <dir>` | Query current status |

### Concurrency

The runtime uses an exclusive file lock (`.lock` in the cron directory) to prevent the same cron from executing concurrently. If a cron tick fires while a previous execution is still running, it is silently skipped. No action is needed from the skill — this is handled automatically.

## References

| File                                      | Description                              |
| ----------------------------------------- | ---------------------------------------- |
| `references/SCHEMAS.md`                   | manifest.json and status.json schemas          |
| `references/task-agent/BOOTSTRAP.md`      | Default bootstrap template                     |
| `references/task-agent/PROMPT.md`         | Task cron prompt template                      |
| `references/heartbeat-agent/HEARTBEAT.md` | Default heartbeat template                     |
| `references/heartbeat-agent/PROMPT.md`    | Heartbeat cron prompt template                 |
| `assets/status_schema.json`               | status.json JSON schema                        |
| `skills/agent_bus/`                       | MQ-backed completion delivery to caller agent  |
