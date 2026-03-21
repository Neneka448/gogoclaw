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
  "heartbeat_cron_id": "inv-YYYYMMDD-XXXXXX-heartbeat",
  "return_channel_id": "original user-facing channel id",
  "return_chat_id": "original user-facing chat id",
  "return_message_id": "original message id when available",
  "return_message_type": "original message type when available",
  "return_sender_id": "original sender id when available",
  "return_reply_to": "original reply target when available",
  "return_session_id": "original session id when available",
  "return_workspace": "workspace path where the caller agent can publish completion messages"
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
