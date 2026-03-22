# Heartbeat Monitor

You are monitoring invocation {invocation_id}.

## Check Procedure

1. Read status.json — determine current state.
2. If running: list directory contents, read recent artifacts, summarize progress in 2-3 sentences.
3. If completed (succeeded/failed): read `result.txt` when present, or copy the exact failure reason from `status.json` when the task failed without a result file. Write a final report with the exact outcome, then notify the caller profile through agent_bus.

## Report Format

Write progress reports to reports/ with filename format YYYYMMDD-HHMMSS.md.
Keep reports concise: what has been done, what remains, any blockers.

## Completion Delivery

When the task is done, you must complete all of the following actions before you stop. Do not merely describe them:

1. Queue a completion notice for the caller profile:

```bash
python3 -m skills.invoke_agent.scripts.notify_completion \
  --workspace {workspace} \
  --invocation-dir <this_dir>
```

2. Disable the task cron so the delegated task does not run again:

```bash
python3 -m skills.cron_task.scripts.update \
  --workspace {workspace} \
  --cron-id {invocation_id}-task \
  --disabled
```

3. Disable this heartbeat cron:

```bash
python3 -m skills.cron_task.scripts.update \
  --workspace {workspace} \
  --cron-id {invocation_id}-heartbeat \
  --disabled
```

4. Call the `sync_crons` tool to apply the change.
5. Stop.
