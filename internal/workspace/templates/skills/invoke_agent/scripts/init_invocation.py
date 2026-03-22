#!/usr/bin/env python3
"""Initialize an invocation directory with manifest and status files."""

import argparse
import json
import os
import sys
from datetime import datetime, timezone
from pathlib import Path


def env(name: str) -> str:
    return os.environ.get(name, "").strip()


def env_metadata() -> dict:
    raw = env("GOGOCLAW_MESSAGE_METADATA_JSON")
    if not raw:
        return {}
    try:
        payload = json.loads(raw)
    except json.JSONDecodeError:
        return {}
    if not isinstance(payload, dict):
        return {}
    return {str(k): str(v) for k, v in payload.items()}


def main():
    parser = argparse.ArgumentParser(description="Create invocation directory and metadata")
    parser.add_argument("--workspace", required=True, help="Workspace root directory")
    parser.add_argument("--caller-profile", required=True, help="Profile that delegated the task")
    parser.add_argument("--target-profile", required=True, help="Profile that will execute the task")
    parser.add_argument("--task-summary", required=True, help="One-line task summary")
    args = parser.parse_args()

    now = datetime.now(timezone.utc)
    date_str = now.strftime("%Y%m%d")
    random_suffix = os.urandom(3).hex()
    invocation_id = f"inv-{date_str}-{random_suffix}"
    metadata = env_metadata()

    invocation_dir = Path(args.workspace) / "invocations" / invocation_id
    (invocation_dir / "reports").mkdir(parents=True, exist_ok=True)

    manifest = {
        "invocation_id": invocation_id,
        "caller_profile": args.caller_profile,
        "target_profile": args.target_profile,
        "task_summary": args.task_summary,
        "created_at": now.isoformat(),
        "task_cron_id": f"{invocation_id}-task",
        "heartbeat_cron_id": f"{invocation_id}-heartbeat",
        "return_channel_id": env("GOGOCLAW_CHANNEL_ID"),
        "return_chat_id": env("GOGOCLAW_CHAT_ID"),
        "return_message_id": env("GOGOCLAW_MESSAGE_ID"),
        "return_message_type": env("GOGOCLAW_MESSAGE_TYPE"),
        "return_sender_id": env("GOGOCLAW_SENDER_ID"),
        "return_reply_to": env("GOGOCLAW_REPLY_TO"),
        "return_correlation_id": metadata.get("mq_correlation_id", ""),
        "return_session_id": env("GOGOCLAW_SESSION_ID"),
        "return_workspace": env("GOGOCLAW_WORKSPACE"),
    }
    (invocation_dir / "manifest.json").write_text(json.dumps(manifest, indent=2))

    status = {
        "status": "pending",
        "started_at": "",
        "finished_at": "",
        "error": "",
    }
    (invocation_dir / "status.json").write_text(json.dumps(status, indent=2))

    print(json.dumps({
        "invocation_id": invocation_id,
        "invocation_dir": str(invocation_dir),
    }))


if __name__ == "__main__":
    main()
