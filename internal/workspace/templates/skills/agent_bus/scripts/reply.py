#!/usr/bin/env python3
"""Queue an explicit reply agent_bus message for MQ delivery."""

import argparse
import json

from .common import build_envelope, enqueue_envelope, load_runtime_config, parse_metadata, resolve_workspace


def main():
    parser = argparse.ArgumentParser(description="Reply to an existing agent_bus message")
    parser.add_argument("--workspace", default=".", help="Workspace root directory")
    parser.add_argument("--target-profile", required=True, help="Profile that sent the original message")
    parser.add_argument("--conversation-id", required=True, help="Conversation id from the original message")
    parser.add_argument("--correlation-id", required=True, help="Root correlation id from the original message")
    parser.add_argument("--in-reply-to-message-id", required=True, help="Exact message_id being answered")
    parser.add_argument("--body", required=True, help="Reply body")
    parser.add_argument("--metadata", default=None, help="Optional JSON metadata object")
    args = parser.parse_args()

    workspace = resolve_workspace(args.workspace)
    runtime = load_runtime_config(workspace)
    envelope = build_envelope(
        runtime,
        message_type="reply",
        target_profile=args.target_profile,
        body=args.body,
        conversation_id=args.conversation_id,
        correlation_id=args.correlation_id,
        in_reply_to_message_id=args.in_reply_to_message_id,
        metadata=parse_metadata(args.metadata),
    )
    path = enqueue_envelope(runtime, envelope)
    print(json.dumps({"status": "queued", "file": str(path), "message_id": envelope["message_id"], "conversation_id": envelope["conversation_id"], "correlation_id": envelope["correlation_id"], "in_reply_to_message_id": envelope["in_reply_to_message_id"]}))


if __name__ == "__main__":
    main()
