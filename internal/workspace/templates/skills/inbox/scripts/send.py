#!/usr/bin/env python3
"""Send a message to a workspace inbox."""

import argparse
import json
import os
import sys
from datetime import datetime, timezone
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description="Send a message to a workspace inbox")
    parser.add_argument("--workspace", required=True, help="Workspace root directory")
    parser.add_argument("--source", required=True, help="Message source (e.g., invocation, user, system)")
    parser.add_argument("--type", required=True, help="Message type (e.g., completion, error, notification)")
    parser.add_argument("--subject", required=True, help="One-line summary")
    parser.add_argument("--body", required=True, help="Full message content")
    parser.add_argument("--metadata", default=None, help="Optional JSON metadata object")
    args = parser.parse_args()

    inbox_dir = Path(args.workspace) / "inbox"
    inbox_dir.mkdir(parents=True, exist_ok=True)

    now = datetime.now(timezone.utc)
    timestamp = now.strftime("%Y%m%d%H%M%S")
    short_id = os.urandom(3).hex()
    msg_id = f"{args.source}-{short_id}"
    filename = f"{args.source}-{short_id}-{timestamp}.json"

    metadata = {}
    if args.metadata:
        metadata = json.loads(args.metadata)

    message = {
        "id": msg_id,
        "source": args.source,
        "type": args.type,
        "status": "unread",
        "subject": args.subject,
        "body": args.body,
        "metadata": metadata,
        "created_at": now.isoformat(),
    }

    filepath = inbox_dir / filename
    filepath.write_text(json.dumps(message, indent=2))

    print(json.dumps({"file": str(filepath), "id": msg_id}))


if __name__ == "__main__":
    main()
