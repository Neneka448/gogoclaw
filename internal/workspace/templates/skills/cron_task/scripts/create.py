#!/usr/bin/env python3
"""Create a new workspace cron task."""

import argparse
import json
import os
import re
import sys
from pathlib import Path

CRON_ID_PATTERN = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9._-]*$")


def main():
    parser = argparse.ArgumentParser(description="Create a new workspace cron task")
    parser.add_argument("--workspace", required=True, help="Workspace root directory")
    parser.add_argument("--cron-id", required=True, help="Stable cron identifier")
    parser.add_argument("--cron-expression", required=True, help="Standard 5-field cron expression")
    task_group = parser.add_mutually_exclusive_group(required=True)
    task_group.add_argument("--task", help="Complete task definition text")
    task_group.add_argument("--task-file", help="Path to a file containing the full task definition text")
    parser.add_argument("--enabled", dest="enabled", action="store_true", default=True)
    parser.add_argument("--disabled", dest="enabled", action="store_false")
    parser.add_argument("--profile-name", default="", help="Target agent profile")
    parser.add_argument("--invocation-mode", default="", help="Invocation mode (foreground, background, cron)")
    args = parser.parse_args()

    if not CRON_ID_PATTERN.match(args.cron_id):
        print(json.dumps({"error": f"invalid cron_id {args.cron_id!r}: must match {CRON_ID_PATTERN.pattern}"}))
        sys.exit(1)

    cron_dir = Path(args.workspace) / "crons" / args.cron_id
    if cron_dir.exists():
        print(json.dumps({"error": f"cron {args.cron_id!r} already exists at {str(cron_dir)}"}))
        sys.exit(1)

    cron_dir.mkdir(parents=True, exist_ok=True)

    config = {
        "cronID": args.cron_id,
        "cronExpression": args.cron_expression,
        "enabled": args.enabled,
    }
    if args.profile_name:
        config["profileName"] = args.profile_name
    if args.invocation_mode:
        config["invocationMode"] = args.invocation_mode

    task_content = args.task
    if args.task_file:
        task_content = Path(args.task_file).read_text()

    (cron_dir / "config.json").write_text(json.dumps(config, indent=2))
    (cron_dir / "task.md").write_text(task_content)

    print(json.dumps({
        "cronID": args.cron_id,
        "cronExpression": args.cron_expression,
        "enabled": args.enabled,
        "path": str(cron_dir),
    }))


if __name__ == "__main__":
    main()
