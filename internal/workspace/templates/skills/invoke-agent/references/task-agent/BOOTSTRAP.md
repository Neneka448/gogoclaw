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

- On success: run `python -m skills.invoke-agent.scripts.task_complete --invocation-dir <this_dir>`
- On failure: run `python -m skills.invoke-agent.scripts.task_fail --invocation-dir <this_dir> --reason "what went wrong"`
