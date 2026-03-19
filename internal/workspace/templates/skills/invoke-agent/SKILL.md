---
name: invoke-agent
description: "Delegate a task to another agent profile through cron-driven execution with heartbeat monitoring. Use when the user asks to hand off, delegate, or assign a task to another agent profile for independent background execution."
trigger: "When the user asks to delegate, hand off, assign, or dispatch a task to another agent or profile, or when a long-running task should execute independently in the background."
---

# Invoke Agent

Delegate tasks to other agent profiles through cron-driven, file-oriented execution. The delegated task runs independently — no blocking, no direct coupling. Progress is monitored by a heartbeat cron, and completion is reported through the inbox.

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

### Invocation Directory

All invocation state lives under `{workspace}/invocations/{invocation-id}/`:

```
invocations/{invocation-id}/
  manifest.json       # Invocation metadata (you write this)
  task.md             # Task definition (you write this)
  bootstrap.md        # Initialization instructions for task agent (you write this)
  heartbeat.md        # Progress check template for heartbeat agent (you write this)
  status.json         # Task status (task agent updates this)
  reports/            # Heartbeat progress reports (heartbeat agent writes here)
```

### manifest.json Schema

```json
{
  "invocation_id": "inv-YYYYMMDD-XXXXXX",
  "caller_profile": "profile-that-delegated",
  "target_profile": "profile-that-executes",
  "task_summary": "One-line summary of the task",
  "created_at": "RFC3339 timestamp",
  "task_cron_id": "inv-YYYYMMDD-XXXXXX-task",
  "heartbeat_cron_id": "inv-YYYYMMDD-XXXXXX-heartbeat"
}
```

### status.json Schema

```json
{
  "status": "pending | running | succeeded | failed",
  "started_at": "RFC3339 timestamp or empty",
  "finished_at": "RFC3339 timestamp or empty",
  "error": "error message or empty"
}
```

### Inbox Convention

Each workspace has an inbox at `{workspace}/inbox/`. Completion notifications are JSON files dropped here:

```json
{
  "invocation_id": "inv-YYYYMMDD-XXXXXX",
  "status": "succeeded | failed",
  "summary": "Brief result summary",
  "report_path": "invocations/inv-YYYYMMDD-XXXXXX/reports/final.md",
  "timestamp": "RFC3339 timestamp"
}
```

## Protocol

### Step 1: Generate Invocation ID

Format: `inv-{YYYYMMDD}-{6-char-random}`, e.g., `inv-20260319-a1b2c3`.

Generate the random suffix:

```bash
date +%Y%m%d | xargs -I{} echo "inv-{}-$(head -c 3 /dev/urandom | xxd -p)"
```

### Step 2: Create Invocation Directory and Files

Use `terminal` to create the full directory structure and write all files:

```bash
INVOCATION_ID="inv-YYYYMMDD-XXXXXX"
INVOCATION_DIR="{workspace}/invocations/$INVOCATION_ID"

mkdir -p "$INVOCATION_DIR/reports"

# Write manifest.json
cat > "$INVOCATION_DIR/manifest.json" << 'MANIFEST_EOF'
{manifest content}
MANIFEST_EOF

# Write status.json (initial state)
cat > "$INVOCATION_DIR/status.json" << 'STATUS_EOF'
{"status": "pending", "started_at": "", "finished_at": "", "error": ""}
STATUS_EOF

# Write task.md
cat > "$INVOCATION_DIR/task.md" << 'TASK_EOF'
{task definition from user request}
TASK_EOF

# Write bootstrap.md (use default template below if user did not specify)
cat > "$INVOCATION_DIR/bootstrap.md" << 'BOOTSTRAP_EOF'
{bootstrap content}
BOOTSTRAP_EOF

# Write heartbeat.md (use default template below if user did not specify)
cat > "$INVOCATION_DIR/heartbeat.md" << 'HEARTBEAT_EOF'
{heartbeat content}
HEARTBEAT_EOF

# Ensure inbox directory exists
mkdir -p "{workspace}/inbox"
```

### Step 3: Create Task Cron

Use `create_cron` with these parameters:

- `cron_id`: `{invocation-id}-task`
- `cron_expression`: `*/1 * * * *` (fallback schedule — primary execution is immediate via Step 5)
- `profile_name`: target agent profile
- `invocation_mode`: `cron`
- `enabled`: true
- `task`: A prompt that instructs the task agent to:
  1. Read `{invocation_dir}/bootstrap.md` for initialization
  2. Execute the task defined in `{invocation_dir}/task.md`
  3. Update `{invocation_dir}/status.json` throughout execution
  4. Write artifacts under `{invocation_dir}/`

Example task prompt:

```
You are executing a delegated task.

Invocation directory: {invocation_dir}

1. Read bootstrap.md in the invocation directory for initialization instructions.
2. Follow the initialization steps, then read task.md for the task definition.
3. Update status.json to {"status": "running", "started_at": "{now}"} before starting.
4. Execute the task. Write any output or artifacts under the invocation directory.
5. When finished, update status.json to {"status": "succeeded", "finished_at": "{now}"}.
6. If you encounter an unrecoverable error, update status.json to {"status": "failed", "finished_at": "{now}", "error": "description"}.
```

### Step 4: Create Heartbeat Cron

Use `create_cron` with these parameters:

- `cron_id`: `{invocation-id}-heartbeat`
- `cron_expression`: `*/5 * * * *` (every 5 minutes, or user-specified interval)
- `profile_name`: target agent profile (same workspace, can read invocation files)
- `invocation_mode`: `cron`
- `enabled`: true
- `task`: A prompt that instructs the heartbeat agent to monitor progress.

Example task prompt:

```
You are a progress monitor for invocation {invocation_id}.

Invocation directory: {invocation_dir}
Caller workspace inbox: {workspace}/inbox/

1. Read status.json in the invocation directory.
2. If status is "pending": report that the task has not started yet. Write a brief note to reports/{timestamp}.md.
3. If status is "running":
   - List files in the invocation directory to see what artifacts exist.
   - Read the most recent session or output files to understand progress.
   - Write a brief progress summary to reports/{timestamp}.md.
4. If status is "succeeded" or "failed":
   - Write a final summary to reports/final.md.
   - Write a completion notice to {workspace}/inbox/{invocation_id}-completed.json:
     {"invocation_id": "{invocation_id}", "status": "{status}", "summary": "brief result", "report_path": "invocations/{invocation_id}/reports/final.md", "timestamp": "{now}"}
   - Then disable this heartbeat by creating an updated cron with enabled: false.
```

### Step 5: Trigger Immediate Execution

Use `execute_cron` to start the task immediately without waiting for the cron schedule:

- `cron_id`: `{invocation-id}-task`
- `async`: true

### Step 6: Report Receipt to User

After all steps complete, report:

```
Invocation created: {invocation-id}
Target profile: {profile}
Invocation directory: {invocation_dir}
Task cron: {invocation-id}-task (triggered immediately)
Heartbeat cron: {invocation-id}-heartbeat (every 5 minutes)

The task is now running independently. Check {workspace}/inbox/ for completion notifications,
or read {invocation_dir}/status.json and reports/ for current progress.
```

## Default Bootstrap Template

Use this when the user does not provide specific bootstrap instructions:

```markdown
# Bootstrap

You are executing a delegated task in this invocation directory.

## Initialization

1. Read task.md in this directory for the full task definition.
2. Update status.json to mark the task as running with the current timestamp.
3. Create any working files you need under this invocation directory.

## Execution Rules

- Stay within the workspace. Use read_file and terminal for file operations.
- Write meaningful artifacts (results, logs, outputs) under this invocation directory.
- If the task references external files, read them via read_file.

## Completion

- On success: update status.json to {"status": "succeeded", "finished_at": "...", "error": ""}.
- On failure: update status.json to {"status": "failed", "finished_at": "...", "error": "what went wrong"}.
```

## Default Heartbeat Template

Use this when the user does not provide a specific heartbeat template:

```markdown
# Heartbeat Monitor

You are monitoring invocation {invocation_id}.

## Check Procedure

1. Read status.json — determine current state.
2. If running: list directory contents, read recent artifacts, summarize progress in 2-3 sentences.
3. If completed (succeeded/failed): write final report and deliver inbox notification.

## Report Format

Write progress reports to reports/ with filename format YYYYMMDD-HHMMSS.md.
Keep reports concise: what has been done, what remains, any blockers.

## Completion Delivery

When the task is done, write to the caller's inbox:
- File: {workspace}/inbox/{invocation_id}-completed.json
- Then update this heartbeat cron to enabled: false using create_cron.
```

## Important Rules

1. Always generate a unique invocation ID. Never reuse IDs.
2. Write ALL files before creating crons. The task agent must find files ready on first execution.
3. Always create both task and heartbeat crons. The heartbeat is essential for progress tracking and completion notification.
4. Always trigger immediate execution with `execute_cron` after creating the task cron.
5. Use the target profile for both task and heartbeat crons so they share the same workspace and can read each other's files.
6. Do not create under-specified task.md files. The task definition must be self-contained and actionable.
7. Adapt the heartbeat interval to the expected task duration. Short tasks: `*/2 * * * *`. Long tasks: `*/10 * * * *`.
