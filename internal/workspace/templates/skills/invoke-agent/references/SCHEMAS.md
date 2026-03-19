# Invoke Agent Schemas

## Invocation Directory Structure

```
invocations/{invocation-id}/
  manifest.json       # Invocation metadata
  task.md             # Task definition
  bootstrap.md        # Initialization instructions for task agent
  heartbeat.md        # Progress check template for heartbeat agent
  status.json         # Task status (updated by task agent)
  reports/            # Heartbeat progress reports
```

## manifest.json

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

## status.json

```json
{
  "status": "pending | running | succeeded | failed",
  "started_at": "RFC3339 timestamp or empty",
  "finished_at": "RFC3339 timestamp or empty",
  "error": "error message or empty"
}
```
