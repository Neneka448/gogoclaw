#!/usr/bin/env python3
"""Read a single inbox message and mark it as read."""

import argparse
import json
import sys
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description="Read a single inbox message by ID")
    parser.add_argument("--workspace", required=True, help="Workspace root directory")
    parser.add_argument("--id", required=True, help="Message ID to read")
    args = parser.parse_args()

    inbox_dir = Path(args.workspace) / "inbox"
    if not inbox_dir.exists():
        print(json.dumps({"error": "inbox directory not found"}))
        sys.exit(1)

    for f in inbox_dir.glob("*.json"):
        try:
            msg = json.loads(f.read_text())
        except (json.JSONDecodeError, OSError):
            continue

        if msg.get("id") == args.id:
            msg["status"] = "read"
            f.write_text(json.dumps(msg, indent=2))
            msg["_file"] = str(f)
            print(json.dumps(msg, indent=2))
            return

    print(json.dumps({"error": f"message {args.id} not found"}))
    sys.exit(1)


if __name__ == "__main__":
    main()
