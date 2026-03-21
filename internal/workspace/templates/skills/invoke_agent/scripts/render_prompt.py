#!/usr/bin/env python3
"""Render a cron prompt from a template."""

import argparse
import sys
from datetime import datetime, timezone
from pathlib import Path


TEMPLATE_MAP = {
    "task": "references/task-agent/PROMPT.md",
    "heartbeat": "references/heartbeat-agent/PROMPT.md",
}


def main():
    parser = argparse.ArgumentParser(description="Render a cron prompt from template")
    parser.add_argument("--type", required=True, choices=["task", "heartbeat"],
                        help="Prompt type to render")
    parser.add_argument("--invocation-id", required=True, help="Invocation ID")
    parser.add_argument("--invocation-dir", required=True, help="Invocation directory path")
    parser.add_argument("--workspace", required=True, help="Workspace root directory")
    parser.add_argument("--output-file", default="", help="Optional file path to write the rendered prompt")
    args = parser.parse_args()

    skill_dir = Path(__file__).resolve().parent.parent
    template_path = skill_dir / TEMPLATE_MAP[args.type]

    if not template_path.exists():
        print(f"Error: template not found: {template_path}", file=sys.stderr)
        sys.exit(1)

    now = datetime.now(timezone.utc).isoformat()
    content = template_path.read_text()
    content = content.replace("{invocation_id}", args.invocation_id)
    content = content.replace("{invocation_dir}", args.invocation_dir)
    content = content.replace("{workspace}", args.workspace)
    content = content.replace("{now}", now)

    if args.output_file:
        output_path = Path(args.output_file)
        output_path.write_text(content)
        print(f'{{"file":"{output_path}"}}')
        return

    print(content)


if __name__ == "__main__":
    main()
