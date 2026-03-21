#!/usr/bin/env python3
"""Shared helpers for agent_bus scripts."""

from __future__ import annotations

import json
import os
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Dict, Optional


RUNTIME_ROOT = Path(".gogoclaw") / "agent_bus"
CONFIG_PATH = RUNTIME_ROOT / "config.json"
OUTBOX_DIRNAME = "outbox"


@dataclass
class RuntimeConfig:
    workspace: Path
    runtime_dir: Path
    outbox_dir: Path
    sent_dir: Path
    failed_dir: Path
    source_profile: str
    source_instance_id: str


def resolve_workspace(raw: str) -> Path:
    workspace = Path(raw or ".").resolve()
    return workspace


def load_runtime_config(workspace: Path) -> RuntimeConfig:
    config_path = workspace / CONFIG_PATH
    if not config_path.exists():
        raise FileNotFoundError(
            f"agent_bus runtime config not found at {config_path}; "
            "start the gateway with MQ enabled first"
        )
    payload = json.loads(config_path.read_text())
    return RuntimeConfig(
        workspace=workspace,
        runtime_dir=Path(payload["runtime_dir"]),
        outbox_dir=Path(payload["outbox_dir"]),
        sent_dir=Path(payload["sent_dir"]),
        failed_dir=Path(payload["failed_dir"]),
        source_profile=payload["source_profile"],
        source_instance_id=payload["source_instance_id"],
    )


def parse_metadata(raw: Optional[str]) -> Dict[str, str]:
    if not raw:
        return {}
    payload = json.loads(raw)
    return {str(k): str(v) for k, v in payload.items()}


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat()


def new_message_id(prefix: str = "msg") -> str:
    return f"{prefix}-{int(datetime.now(timezone.utc).timestamp() * 1000)}-{os.urandom(4).hex()}"


def build_envelope(
    runtime: RuntimeConfig,
    *,
    message_type: str,
    body: str,
    source_profile: str = "",
    source_instance_id: str = "",
    target_profile: str = "",
    conversation_id: str = "",
    correlation_id: str = "",
    in_reply_to_message_id: str = "",
    metadata: Optional[Dict[str, str]] = None,
) -> Dict[str, object]:
    body = body.strip()
    if not body:
        raise ValueError("body is required")

    message_id = new_message_id()
    if not conversation_id:
        conversation_id = message_id
    if not correlation_id:
        correlation_id = message_id

    envelope = {
        "version": 1,
        "message_id": message_id,
        "message_type": message_type,
        "source_profile": (source_profile or runtime.source_profile).strip(),
        "source_instance_id": (source_instance_id or runtime.source_instance_id).strip(),
        "target_profile": target_profile.strip(),
        "conversation_id": conversation_id.strip(),
        "correlation_id": correlation_id.strip(),
        "in_reply_to_message_id": in_reply_to_message_id.strip(),
        "created_at": utc_now(),
        "body": body,
        "metadata": metadata or {},
    }

    if message_type == "broadcast":
        envelope["target_profile"] = ""
    if message_type == "reply" and not envelope["in_reply_to_message_id"]:
        raise ValueError("reply messages require in_reply_to_message_id")

    return envelope


def enqueue_envelope(runtime: RuntimeConfig, envelope: Dict[str, object]) -> Path:
    runtime.outbox_dir.mkdir(parents=True, exist_ok=True)
    filename = f"{envelope['created_at'].replace(':', '').replace('+00:00', 'Z')}-{envelope['message_id']}.json"
    path = runtime.outbox_dir / filename
    path.write_text(json.dumps(envelope, indent=2) + "\n")
    return path
