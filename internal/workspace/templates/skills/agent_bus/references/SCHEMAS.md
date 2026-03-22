# Agent Bus Envelope

All outbound files and RabbitMQ payloads use the same JSON envelope:

```json
{
  "version": 1,
  "message_id": "msg-...",
  "message_type": "direct | broadcast | reply",
  "source_profile": "writer",
  "source_instance_id": "writer@host-a",
  "target_profile": "planner",
  "conversation_id": "msg-...",
  "correlation_id": "msg-...",
  "in_reply_to_message_id": "msg-...",
  "created_at": "2026-03-21T12:00:00Z",
  "body": "message content",
  "metadata": {
    "optional": "string values"
  }
}
```

Rules:

- `message_id` is unique per message.
- `conversation_id` groups a thread.
- `correlation_id` points to the root request.
- `reply` messages must set `in_reply_to_message_id`.
- `broadcast` messages must not set `target_profile`.
