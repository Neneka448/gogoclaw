#!/usr/bin/env python3
"""Update an existing workspace cron task (merge semantics)."""

import argparse
import json
import re
import sys
from pathlib import Path

from skills.cron_task.scripts.common import (
    resolve_config_path,
    validate_invocation_mode,
    validate_profile_name,
)

CRON_ID_PATTERN = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9._-]*$")


def main():
    parser = argparse.ArgumentParser(description="Update an existing workspace cron task")
    parser.add_argument("--workspace", required=True, help="Workspace root directory")
    parser.add_argument("--cron-id", required=True, help="Cron identifier to update")
    parser.add_argument("--cron-expression", default=None, help="New cron expression")
    task_group = parser.add_mutually_exclusive_group()
    task_group.add_argument("--task", default=None, help="New task definition text")
    task_group.add_argument("--task-file", default=None, help="Path to a file containing the new task definition text")
    parser.add_argument("--enabled", dest="enabled", action="store_true", default=None)
    parser.add_argument("--disabled", dest="enabled", action="store_false")
    parser.add_argument("--profile-name", default=None, help="New target agent profile")
    parser.add_argument("--invocation-mode", default=None, help="New invocation mode")
    parser.add_argument("--config-path", default="", help="Path to config.json for profile validation")
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

    if args.cron_expression is not None:
        config["cronExpression"] = args.cron_expression
    if args.enabled is not None:
        config["enabled"] = args.enabled
    if args.profile_name is not None:
        try:
            validate_profile_name(args.profile_name, resolve_config_path(args.config_path))
        except Exception as exc:
            print(json.dumps({"error": str(exc)}))
            sys.exit(1)
        config["profileName"] = args.profile_name
    if args.invocation_mode is not None:
        try:
            validate_invocation_mode(args.invocation_mode)
        except Exception as exc:
            print(json.dumps({"error": str(exc)}))
            sys.exit(1)
        config["invocationMode"] = args.invocation_mode

    config_path.write_text(json.dumps(config, indent=2))

    task_content = args.task
    if args.task_file is not None:
        task_content = Path(args.task_file).read_text()
    if task_content is not None:
        task_path.write_text(task_content)

    config["path"] = str(cron_dir)
    print(json.dumps(config, indent=2))


if __name__ == "__main__":
    main()
