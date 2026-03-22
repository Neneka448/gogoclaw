#!/usr/bin/env python3
"""Get details of a workspace cron task by ID."""

import argparse
import json
import sys
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description="Get details of a workspace cron task")
    parser.add_argument("--workspace", required=True, help="Workspace root directory")
    parser.add_argument("--cron-id", required=True, help="Cron identifier to look up")
    args = parser.parse_args()

    cron_dir = Path(args.workspace) / "crons" / args.cron_id
    config_path = cron_dir / "config.json"
    task_path = cron_dir / "task.md"

    if not config_path.exists():
        print(json.dumps({"error": f"cron {args.cron_id!r} not found"}))
        sys.exit(1)

    try:
        config = json.loads(config_path.read_text())
    except (json.JSONDecodeError, OSError) as exc:
        print(json.dumps({"error": f"read config: {exc}"}))
        sys.exit(1)

    task = ""
    if task_path.exists():
        task = task_path.read_text()

    config["task"] = task
    config["path"] = str(cron_dir)
    print(json.dumps(config, indent=2))


if __name__ == "__main__":
    main()
