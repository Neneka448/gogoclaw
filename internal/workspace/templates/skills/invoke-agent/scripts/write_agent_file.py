#!/usr/bin/env python3
"""Write a task, bootstrap, or heartbeat file into an invocation directory."""

import argparse
import json
import sys
from pathlib import Path

TEMPLATE_MAP = {
    "task": None,  # task has no default template — content is always required
    "bootstrap": "references/task-agent/BOOTSTRAP.md",
    "heartbeat": "references/heartbeat-agent/HEARTBEAT.md",
}

FILENAME_MAP = {
    "task": "task.md",
    "bootstrap": "bootstrap.md",
    "heartbeat": "heartbeat.md",
}


def main():
    parser = argparse.ArgumentParser(description="Write an agent file into an invocation directory")
    parser.add_argument("--invocation-dir", required=True, help="Invocation directory path")
    parser.add_argument("--type", required=True, choices=["task", "bootstrap", "heartbeat"],
                        help="File type to write")
    parser.add_argument("--content", default=None, help="Direct content to write")
    parser.add_argument("--template-vars", default=None,
                        help="JSON object of template variables for substitution")
    args = parser.parse_args()

    invocation_dir = Path(args.invocation_dir)
    file_type = args.type
    filename = FILENAME_MAP[file_type]

    if args.content:
        content = args.content
    else:
        template_rel = TEMPLATE_MAP.get(file_type)
        if template_rel is None:
            print(json.dumps({"error": f"No default template for type '{file_type}'. Provide --content."}))
            sys.exit(1)

        # Resolve template path relative to the skill directory
        skill_dir = Path(__file__).resolve().parent.parent
        template_path = skill_dir / template_rel
        if not template_path.exists():
            print(json.dumps({"error": f"Template not found: {template_path}"}))
            sys.exit(1)

        content = template_path.read_text()

    if args.template_vars:
        template_vars = json.loads(args.template_vars)
        for key, value in template_vars.items():
            content = content.replace(f"{{{key}}}", str(value))

    output_path = invocation_dir / filename
    output_path.write_text(content)

    print(json.dumps({"file": str(output_path)}))


if __name__ == "__main__":
    main()
