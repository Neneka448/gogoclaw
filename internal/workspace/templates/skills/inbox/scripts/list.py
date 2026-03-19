#!/usr/bin/env python3
"""List messages in a workspace inbox."""

import argparse
import json
import sys
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description="List messages in a workspace inbox")
    parser.add_argument("--workspace", required=True, help="Workspace root directory")
    parser.add_argument("--status", default="unread", help="Filter by status (default: unread)")
    parser.add_argument("--type", default=None, help="Filter by message type")
    parser.add_argument("--source", default=None, help="Filter by source")
    args = parser.parse_args()

    inbox_dir = Path(args.workspace) / "inbox"
    if not inbox_dir.exists():
        print("[]")
        return

    messages = []
    for f in sorted(inbox_dir.glob("*.json")):
        try:
            msg = json.loads(f.read_text())
        except (json.JSONDecodeError, OSError):
            continue

        if args.status and msg.get("status") != args.status:
            continue
        if args.type and msg.get("type") != args.type:
            continue
        if args.source and msg.get("source") != args.source:
            continue

        msg["_file"] = str(f)
        messages.append(msg)

    print(json.dumps(messages, indent=2))


if __name__ == "__main__":
    main()
