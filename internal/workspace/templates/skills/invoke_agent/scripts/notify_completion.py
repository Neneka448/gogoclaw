#!/usr/bin/env python3
"""Send invocation completion back to the caller agent through agent_bus."""

import argparse
import json
import sys
from pathlib import Path

from skills.agent_bus.scripts.common import build_envelope, enqueue_envelope, load_runtime_config, resolve_workspace


def load_json(path: Path):
    if not path.exists():
        raise FileNotFoundError(f"missing file: {path}")
    return json.loads(path.read_text())


def existing_relative_path(path: Path, workspace: Path) -> str:
    try:
        relative = path.relative_to(workspace)
    except ValueError:
        return ""
    if not path.exists() or not path.is_file():
        return ""
    return str(relative)


def source_instance_id_for_profile(runtime, profile: str) -> str:
    suffix = ""
    if "@" in runtime.source_instance_id:
        suffix = runtime.source_instance_id.split("@", 1)[1].strip()
    if suffix:
        return f"{profile}@{suffix}"
    return profile


def main():
    parser = argparse.ArgumentParser(description="Notify the caller profile that an invocation completed")
    parser.add_argument("--workspace", required=True, help="Workspace root directory")
    parser.add_argument("--invocation-dir", required=True, help="Invocation directory path")
    args = parser.parse_args()

    workspace = resolve_workspace(args.workspace)
    invocation_dir = Path(args.invocation_dir)

    manifest = load_json(invocation_dir / "manifest.json")
    status = load_json(invocation_dir / "status.json")

    runtime = None
    try:
        runtime = load_runtime_config(workspace)
    except FileNotFoundError:
        fallback_workspace = str(manifest.get("return_workspace", "")).strip()
        if fallback_workspace:
            runtime = load_runtime_config(resolve_workspace(fallback_workspace))
        else:
            raise

    caller_profile = str(manifest.get("caller_profile", "")).strip()
    invocation_id = str(manifest.get("invocation_id", "")).strip()
    target_profile = str(manifest.get("target_profile", "")).strip()
    if not caller_profile:
        raise ValueError("manifest missing caller_profile")
    if not invocation_id:
        raise ValueError("manifest missing invocation_id")
    if not target_profile:
        raise ValueError("manifest missing target_profile")

    report_path = f"invocations/{invocation_id}/reports/final.md"
    result_path = existing_relative_path(invocation_dir / "result.txt", workspace)
    state = str(status.get("status", "")).strip() or "unknown"
    error = str(status.get("error", "")).strip()

    body_lines = [
        "SYSTEM EVENT: A delegated task you previously started for this user conversation has completed.",
        "",
        f"Invocation ID: {invocation_id}",
        f"Target profile: {target_profile}",
        f"Status: {state}",
        f"Final report: {report_path}",
    ]
    if result_path:
        body_lines.append(f"Result file: {result_path}")
    if error:
        body_lines.extend(["", f"Error: {error}"])
    body_lines.extend([
        "",
        "Read the final report first.",
        "If a result file is listed, read it before replying.",
        "Do not infer or invent exact numbers when the report is vague.",
        "Then continue the user conversation in this session with a concise update.",
    ])

    metadata = {
        "invocation_id": invocation_id,
        "completion_kind": "invoke_agent",
        "report_path": report_path,
        "result_path": result_path,
        "task_summary": str(manifest.get("task_summary", "")).strip(),
        "target_profile": target_profile,
        "return_channel_id": str(manifest.get("return_channel_id", "")).strip(),
        "return_chat_id": str(manifest.get("return_chat_id", "")).strip(),
        "return_message_id": str(manifest.get("return_message_id", "")).strip(),
        "return_message_type": str(manifest.get("return_message_type", "")).strip(),
        "return_sender_id": str(manifest.get("return_sender_id", "")).strip(),
        "return_reply_to": str(manifest.get("return_reply_to", "")).strip(),
        "return_correlation_id": str(manifest.get("return_correlation_id", "")).strip(),
        "return_session_id": str(manifest.get("return_session_id", "")).strip(),
        "status": state,
    }
    if error:
        metadata["error"] = error

    envelope = build_envelope(
        runtime,
        message_type="direct",
        source_profile=target_profile,
        source_instance_id=source_instance_id_for_profile(runtime, target_profile),
        target_profile=caller_profile,
        body="\n".join(body_lines),
        conversation_id=invocation_id,
        correlation_id=invocation_id,
        metadata=metadata,
    )
    path = enqueue_envelope(runtime, envelope)
    print(json.dumps({
        "status": "queued",
        "file": str(path),
        "message_id": envelope["message_id"],
        "target_profile": caller_profile,
        "invocation_id": invocation_id,
    }))


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(json.dumps({"error": str(exc)}))
        sys.exit(1)
