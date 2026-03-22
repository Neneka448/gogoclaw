#!/usr/bin/env python3
"""Mark an invocation task as failed."""

import argparse
import json
import sys
from datetime import datetime, timezone
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description="Mark task status as failed")
    parser.add_argument("--invocation-dir", required=True, help="Invocation directory path")
    parser.add_argument("--reason", default="", help="Failure reason")
    args = parser.parse_args()

    status_path = Path(args.invocation_dir) / "status.json"
    if not status_path.exists():
        print(json.dumps({"error": "status.json not found"}))
        sys.exit(1)

    status = json.loads(status_path.read_text())
    status["status"] = "failed"
    status["finished_at"] = datetime.now(timezone.utc).isoformat()
    status["error"] = args.reason

    status_path.write_text(json.dumps(status, indent=2))
    print(json.dumps(status))


if __name__ == "__main__":
    main()
