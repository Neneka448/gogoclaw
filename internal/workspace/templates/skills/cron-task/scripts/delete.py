#!/usr/bin/env python3
"""Delete a workspace cron task by ID."""

import argparse
import json
import shutil
import sys
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description="Delete a workspace cron task")
    parser.add_argument("--workspace", required=True, help="Workspace root directory")
    parser.add_argument("--cron-id", required=True, help="Cron identifier to delete")
    args = parser.parse_args()

    cron_dir = Path(args.workspace) / "crons" / args.cron_id
    if not cron_dir.exists():
        print(json.dumps({"error": f"cron {args.cron_id!r} not found"}))
        sys.exit(1)

    shutil.rmtree(cron_dir)
    print(json.dumps({"cronID": args.cron_id, "deleted": True}))


if __name__ == "__main__":
    main()
