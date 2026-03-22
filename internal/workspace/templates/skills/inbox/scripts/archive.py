#!/usr/bin/env python3
"""Archive inbox messages by moving them to _archive/."""

import argparse
import json
import shutil
import sys
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description="Archive inbox messages")
    parser.add_argument("--workspace", required=True, help="Workspace root directory")
    parser.add_argument("--id", default=None, help="Archive a single message by ID")
    parser.add_argument("--all-read", action="store_true", help="Archive all read messages")
    args = parser.parse_args()

    if not args.id and not args.all_read:
        parser.error("Provide --id or --all-read")

    inbox_dir = Path(args.workspace) / "inbox"
    archive_dir = inbox_dir / "_archive"

    if not inbox_dir.exists():
        print(json.dumps({"archived": []}))
        return

    archive_dir.mkdir(parents=True, exist_ok=True)
    archived = []

    for f in inbox_dir.glob("*.json"):
        try:
            msg = json.loads(f.read_text())
        except (json.JSONDecodeError, OSError):
            continue

        should_archive = False
        if args.id and msg.get("id") == args.id:
            should_archive = True
        elif args.all_read and msg.get("status") == "read":
            should_archive = True

        if should_archive:
            msg["status"] = "archived"
            dest = archive_dir / f.name
            dest.write_text(json.dumps(msg, indent=2))
            f.unlink()
            archived.append(msg.get("id", f.name))

    print(json.dumps({"archived": archived}))


if __name__ == "__main__":
    main()
