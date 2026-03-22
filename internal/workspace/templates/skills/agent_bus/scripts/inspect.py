#!/usr/bin/env python3
"""Inspect local agent_bus outbox state."""

import argparse
import json
from pathlib import Path

from .common import load_runtime_config, resolve_workspace


def collect(directory: Path, limit: int):
    if not directory.exists():
        return []
    files = sorted(directory.glob("*.json"), reverse=True)
    rows = []
    for path in files[:limit]:
        try:
            payload = json.loads(path.read_text())
        except (json.JSONDecodeError, OSError):
            payload = {"error": "unreadable"}
        rows.append(
            {
                "file": str(path),
                "message_id": payload.get("message_id"),
                "message_type": payload.get("message_type"),
                "target_profile": payload.get("target_profile"),
                "conversation_id": payload.get("conversation_id"),
                "created_at": payload.get("created_at"),
            }
        )
    return rows


def main():
    parser = argparse.ArgumentParser(description="Inspect agent_bus outbox state")
    parser.add_argument("--workspace", default=".", help="Workspace root directory")
    parser.add_argument("--limit", type=int, default=10, help="Maximum files per directory")
    args = parser.parse_args()

    workspace = resolve_workspace(args.workspace)
    runtime = load_runtime_config(workspace)
    payload = {
        "source_profile": runtime.source_profile,
        "source_instance_id": runtime.source_instance_id,
        "outbox": collect(runtime.outbox_dir, args.limit),
        "sent": collect(runtime.sent_dir, args.limit),
        "failed": collect(runtime.failed_dir, args.limit),
    }
    print(json.dumps(payload, indent=2))


if __name__ == "__main__":
    main()
