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

When the task is done, deliver a completion notice to the caller's inbox:

```bash
python -m skills.inbox.scripts.send \
  --workspace {workspace} \
  --source invocation \
  --type completion \
  --subject "Invocation {invocation_id} completed" \
  --body "Status: {status}. See reports/final.md for details." \
  --metadata '{{"invocation_id": "{invocation_id}", "report_path": "invocations/{invocation_id}/reports/final.md"}}'
```

Then disable this heartbeat cron:

```bash
python -m skills.cron_task.scripts.update \
  --workspace {workspace} \
  --cron-id {invocation_id}-heartbeat \
  --disabled
```

Then call the `sync_crons` tool to apply the change.
