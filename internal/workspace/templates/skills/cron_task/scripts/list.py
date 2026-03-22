#!/usr/bin/env python3
"""List all workspace cron tasks."""

import argparse
import json
import sys
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description="List all workspace cron tasks")
    parser.add_argument("--workspace", required=True, help="Workspace root directory")
    parser.add_argument("--enabled-only", action="store_true", help="Only list enabled crons")
    args = parser.parse_args()

    crons_dir = Path(args.workspace) / "crons"
    if not crons_dir.exists():
        print("[]")
        return

    results = []
    for config_path in sorted(crons_dir.glob("*/config.json")):
        try:
            config = json.loads(config_path.read_text())
        except (json.JSONDecodeError, OSError):
            continue

        if args.enabled_only and not config.get("enabled", False):
            continue

        config["path"] = str(config_path.parent)
        results.append(config)

    print(json.dumps(results, indent=2))


if __name__ == "__main__":
    main()
