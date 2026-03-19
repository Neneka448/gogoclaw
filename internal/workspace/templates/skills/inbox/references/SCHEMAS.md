# Inbox Message Schema

## Directory Convention

- Inbox path: `{workspace}/inbox/`
- Archive path: `{workspace}/inbox/_archive/`
- Message filename: `{source}-{id}-{timestamp}.json`
  - `source`: originating system or agent (e.g., `invocation`, `user`, `system`)
  - `id`: unique identifier from the source
  - `timestamp`: `YYYYMMDDHHMMSS` format

## Message JSON Schema

```json
{
  "id": "string — unique message identifier",
  "source": "string — originating system (e.g., invocation, user, system)",
  "type": "string — message category (e.g., completion, error, notification)",
  "status": "unread | read | archived",
  "subject": "string — one-line summary",
  "body": "string — full message content",
  "metadata": "object — optional, arbitrary key-value pairs",
  "created_at": "string — RFC3339 timestamp"
}
```

### Field Details

| Field | Required | Description |
|-------|----------|-------------|
| `id` | yes | Unique identifier, typically `{source}-{short-random}` |
| `source` | yes | Origin system or agent name |
| `type` | yes | Message category for filtering |
| `status` | yes | One of: `unread`, `read`, `archived` |
| `subject` | yes | Brief summary |
| `body` | yes | Full content |
| `metadata` | no | Extra context as key-value pairs |
| `created_at` | yes | RFC3339 timestamp of creation |
