#!/usr/bin/env python3
"""Initialize an invocation directory with manifest and status files."""

import argparse
import json
import os
import sys
from datetime import datetime, timezone
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description="Create invocation directory and metadata")
    parser.add_argument("--workspace", required=True, help="Workspace root directory")
    parser.add_argument("--caller-profile", required=True, help="Profile that delegated the task")
    parser.add_argument("--target-profile", required=True, help="Profile that will execute the task")
    parser.add_argument("--task-summary", required=True, help="One-line task summary")
    args = parser.parse_args()

    now = datetime.now(timezone.utc)
    date_str = now.strftime("%Y%m%d")
    random_suffix = os.urandom(3).hex()
    invocation_id = f"inv-{date_str}-{random_suffix}"

    invocation_dir = Path(args.workspace) / "invocations" / invocation_id
    (invocation_dir / "reports").mkdir(parents=True, exist_ok=True)

    manifest = {
        "invocation_id": invocation_id,
        "caller_profile": args.caller_profile,
        "target_profile": args.target_profile,
        "task_summary": args.task_summary,
        "created_at": now.isoformat(),
        "task_cron_id": f"{invocation_id}-task",
        "heartbeat_cron_id": f"{invocation_id}-heartbeat",
    }
    (invocation_dir / "manifest.json").write_text(json.dumps(manifest, indent=2))

    status = {
        "status": "pending",
        "started_at": "",
        "finished_at": "",
        "error": "",
    }
    (invocation_dir / "status.json").write_text(json.dumps(status, indent=2))

    # Ensure inbox directory exists
    (Path(args.workspace) / "inbox").mkdir(parents=True, exist_ok=True)

    print(json.dumps({
        "invocation_id": invocation_id,
        "invocation_dir": str(invocation_dir),
    }))


if __name__ == "__main__":
    main()
