#!/usr/bin/env python3
"""Query current task status from an invocation directory."""

import argparse
import json
import sys
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description="Read task status")
    parser.add_argument("--invocation-dir", required=True, help="Invocation directory path")
    args = parser.parse_args()

    status_path = Path(args.invocation_dir) / "status.json"
    if not status_path.exists():
        print(json.dumps({"error": "status.json not found"}))
        sys.exit(1)

    status = json.loads(status_path.read_text())
    print(json.dumps(status))


if __name__ == "__main__":
    main()
