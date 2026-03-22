---
name: inbox
description: >
  Workspace-level message delivery protocol. Any agent can send, list, read,
  and archive messages in a shared inbox. Polling is handled by each agent
  via its own cron schedule.
---

# Inbox

A workspace-level message delivery protocol. Any agent can send, list, read, and archive messages through a shared inbox directory. Each agent creates its own cron to poll for new messages.

## When to Use

- Delivering completion notifications between agents
- Sending status updates or error reports to a workspace inbox
- Any cross-agent communication that does not require synchronous response

## Message Schema

See `references/SCHEMAS.md` for the full JSON schema and directory conventions.

Quick summary:
- Inbox path: `{workspace}/inbox/`
- Archive path: `{workspace}/inbox/_archive/`
- Messages are JSON files named `{source}-{id}-{timestamp}.json`

## Scripts

All scripts are run from the workspace root. Use `python -m skills.inbox.scripts.<name>`.

### send.py — Send a message

```bash
python -m skills.inbox.scripts.send \
  --workspace /path/to/workspace \
  --source invocation \
  --type completion \
  --subject "Task finished" \
  --body "The task completed successfully." \
  --metadata '{"invocation_id": "inv-20260319-abc123"}'
```

Output: `{"file": "...", "id": "..."}`

### list.py — List messages

```bash
python -m skills.inbox.scripts.list \
  --workspace /path/to/workspace \
  --status unread \
  --type completion
```

Output: JSON array of matching messages.

### read.py — Read a single message

```bash
python -m skills.inbox.scripts.read \
  --workspace /path/to/workspace \
  --id "invocation-abc123"
```

Marks the message as `read` and outputs its full JSON.

### archive.py — Archive messages

```bash
# Archive a single message
python -m skills.inbox.scripts.archive \
  --workspace /path/to/workspace \
  --id "invocation-abc123"

# Archive all read messages
python -m skills.inbox.scripts.archive \
  --workspace /path/to/workspace \
  --all-read
```

Output: `{"archived": [...]}`

## Polling

Inbox does not include built-in polling. Each agent should create its own cron to check for new messages. Example cron task prompt:

```
Check the workspace inbox for new messages:
1. Run: python -m skills.inbox.scripts.list --workspace {workspace} --status unread
2. For each unread message, process it according to its type.
3. After processing, mark it as read: python -m skills.inbox.scripts.read --workspace {workspace} --id {id}
4. Optionally archive processed messages: python -m skills.inbox.scripts.archive --workspace {workspace} --all-read
```
