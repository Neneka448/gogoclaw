---
name: agent_bus
description: "Send direct, broadcast, and explicit reply messages to other agents through the workspace outbox sidecar backed by RabbitMQ."
trigger: "When the task requires agent-to-agent communication, cross-software messaging, announcement fan-out, or explicit replies that should travel through the MQ bus."
---

# Agent Bus

Use the workspace outbox sidecar to communicate with other agents through RabbitMQ. Do not implement ad-hoc message files yourself. Always use the scripts in this skill so the envelope stays consistent.

## When to Use

- Sending a direct message to another agent profile
- Broadcasting an announcement to all agent profiles
- Sending an explicit reply to a known message
- Inspecting local outbox delivery state while debugging coordination

Do NOT use for:

- Messages to the current user in the current chat channel
- Background cron orchestration inside the same workspace that does not need MQ

## Runtime Requirement

This skill depends on the gateway process running with the MQ channel enabled. The MQ channel writes runtime state to:

`{workspace}/.gogoclaw/agent_bus/config.json`

The scripts read that file and place outbound envelopes into:

`{workspace}/.gogoclaw/agent_bus/outbox/`

The MQ channel sidecar publishes those files to RabbitMQ and moves them to `sent/` or `failed/`.

## Protocol

1. Use `send.py` for normal direct messages.
2. Use `broadcast.py` for fan-out announcements.
3. Use `reply.py` only when you know the exact original `message_id`, `conversation_id`, and `correlation_id`.
4. Preserve `conversation_id` and `correlation_id` when continuing an existing thread.
5. For replies, always set `in_reply_to_message_id` to the original message you are answering.
6. If you need to confirm delivery state, inspect `sent/` and `failed/` with `inspect.py`.

## Scripts

Run from the workspace root:

`python -m skills.agent_bus.scripts.<name>`

### Direct message

```bash
python -m skills.agent_bus.scripts.send \
  --target-profile planner \
  --body "Please review the deployment plan." \
  --metadata '{"priority":"normal"}'
```

### Broadcast

```bash
python -m skills.agent_bus.scripts.broadcast \
  --body "Maintenance starts in 10 minutes."
```

### Explicit reply

```bash
python -m skills.agent_bus.scripts.reply \
  --target-profile planner \
  --conversation-id conv_123 \
  --correlation-id msg_root_123 \
  --in-reply-to-message-id msg_456 \
  --body "Plan approved."
```

### Inspect outbox state

```bash
python -m skills.agent_bus.scripts.inspect --limit 20
```

## References

- `references/SCHEMAS.md` for the message envelope
