#!/usr/bin/env python3
"""Shared helpers for cron_task scripts."""

from __future__ import annotations

import json
import os
from pathlib import Path


VALID_INVOCATION_MODES = {"foreground", "background", "cron"}


def resolve_config_path(raw: str) -> Path:
    config_path = (raw or "").strip() or os.environ.get("GOGOCLAW_CONFIG", "").strip()
    if config_path:
        return Path(config_path).expanduser()
    return Path.home() / ".gogoclaw" / "config.json"


def validate_profile_name(profile_name: str, config_path: Path) -> None:
    profile_name = (profile_name or "").strip()
    if not profile_name:
        return
    if not config_path.exists():
        raise FileNotFoundError(f"config file not found: {config_path}")

    payload = json.loads(config_path.read_text())
    profiles = payload.get("agents", {}).get("profiles", {})
    if not isinstance(profiles, dict):
        raise ValueError(f"invalid profiles section in config: {config_path}")

    names = sorted(str(name).strip() for name in profiles.keys() if str(name).strip())
    if profile_name not in names:
        available = ", ".join(names) if names else "(none)"
        raise ValueError(
            f"unknown profile {profile_name!r} in {config_path}; available profiles: {available}"
        )


def validate_invocation_mode(invocation_mode: str) -> None:
    invocation_mode = (invocation_mode or "").strip()
    if not invocation_mode:
        return
    if invocation_mode not in VALID_INVOCATION_MODES:
        raise ValueError(
            f"invalid invocation mode {invocation_mode!r}: "
            f"must be one of {', '.join(sorted(VALID_INVOCATION_MODES))}"
        )
