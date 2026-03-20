#!/usr/bin/env python3
"""Mark an invocation task as succeeded."""

import argparse
import json
import sys
from datetime import datetime, timezone
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description="Mark task status as succeeded")
    parser.add_argument("--invocation-dir", required=True, help="Invocation directory path")
    args = parser.parse_args()

    status_path = Path(args.invocation_dir) / "status.json"
    if not status_path.exists():
        print(json.dumps({"error": "status.json not found"}))
        sys.exit(1)

    status = json.loads(status_path.read_text())
    status["status"] = "succeeded"
    status["finished_at"] = datetime.now(timezone.utc).isoformat()
    status["error"] = ""

    status_path.write_text(json.dumps(status, indent=2))
    print(json.dumps(status))


if __name__ == "__main__":
    main()
