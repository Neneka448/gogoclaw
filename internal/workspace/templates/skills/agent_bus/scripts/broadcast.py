#!/usr/bin/env python3
"""Queue a broadcast agent_bus message for MQ delivery."""

import argparse
import json

from .common import build_envelope, enqueue_envelope, load_runtime_config, parse_metadata, resolve_workspace


def main():
    parser = argparse.ArgumentParser(description="Broadcast an agent_bus message")
    parser.add_argument("--workspace", default=".", help="Workspace root directory")
    parser.add_argument("--body", required=True, help="Message body")
    parser.add_argument("--conversation-id", default="", help="Optional conversation id")
    parser.add_argument("--correlation-id", default="", help="Optional root correlation id")
    parser.add_argument("--metadata", default=None, help="Optional JSON metadata object")
    args = parser.parse_args()

    workspace = resolve_workspace(args.workspace)
    runtime = load_runtime_config(workspace)
    envelope = build_envelope(
        runtime,
        message_type="broadcast",
        body=args.body,
        conversation_id=args.conversation_id,
        correlation_id=args.correlation_id,
        metadata=parse_metadata(args.metadata),
    )
    path = enqueue_envelope(runtime, envelope)
    print(json.dumps({"status": "queued", "file": str(path), "message_id": envelope["message_id"], "conversation_id": envelope["conversation_id"], "correlation_id": envelope["correlation_id"]}))


if __name__ == "__main__":
    main()
