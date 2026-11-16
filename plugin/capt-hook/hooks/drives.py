from __future__ import annotations

import json
import os
import sqlite3
import subprocess
from contextlib import closing
from pathlib import Path
from typing import Any

from captain_hook.events import BaseHookEvent, ToolHookEvent
from captain_hook.util import reqenv

from .common import BIN
from .writes import pending

__capt_hook_skip__ = True

DRIVE_ENV = ("CLAUDE_LONG_RUNNING_DRIVE", "CLAUDE_LONG_RUNNING_LANE")
BINDINGS = "SELECT collection, key, body FROM rows WHERE collection = '' AND key IN ('driver', 'roles', 'subcoordinators', 'communicators', 'slack') OR collection = 'handlers'"
UNREAD = (OSError, sqlite3.Error, ValueError, LookupError, AttributeError, TypeError, subprocess.SubprocessError)
WHOAMI_SECONDS = 2


def drive_root() -> Path:
    if explicit := os.environ.get("LONG_RUNNING_STATE"):
        return Path(explicit)
    if xdg := os.environ.get("XDG_STATE_HOME"):
        return Path(xdg) / "long-running"
    return Path.home() / ".local" / "state" / "long-running"


def saved_drive(folder: Path) -> dict[str, Any]:
    if (db := folder / "drive.db").is_file():
        with closing(sqlite3.connect(f"{db.as_uri()}?mode=ro", uri=True, timeout=1)) as conn:
            rows = conn.execute(BINDINGS).fetchall()
        return {key: json.loads(body) for collection, key, body in rows if not collection} | {"handlers": {key: json.loads(body) for collection, key, body in rows if collection}}
    return json.loads((folder / "drive.json").read_text())


def drive_sessions(saved: dict[str, Any]) -> set[str]:
    roles = {"coordinator": saved["driver"]} if "driver" in saved else saved.get("roles", {})
    return {entry.get("session") for entry in (*roles.values(), *saved.get("subcoordinators", {}).values(), *saved.get("communicators", {}).values(), *saved.get("handlers", {}).values())}


def drive_channels(saved: dict[str, Any]) -> set[str]:
    return {made["id"] for made in (saved.get("slack") or {}).get("channels", {}).values()}


def bound_drive(session: str) -> dict[str, Any] | None:
    if not session or not (root := drive_root()).is_dir():
        return None
    for folder in root.iterdir():
        try:
            if session in drive_sessions(saved := saved_drive(folder)):
                return saved
        except UNREAD:
            continue
    return None


def bound_to_drive(session: str) -> bool:
    return bound_drive(session) is not None


def named_drive() -> dict[str, Any] | None:
    if not (name := reqenv.getenv("CLAUDE_LONG_RUNNING_DRIVE")):
        return None
    try:
        return saved_drive(drive_root() / name)
    except UNREAD:
        return None


def drive_session(evt: BaseHookEvent) -> bool:
    return any(reqenv.getenv(name) for name in DRIVE_ENV) or bound_to_drive(evt._raw.get("session_id") or "")


def his_dm() -> set[str]:
    try:
        done = subprocess.run([str(BIN), "whoami"], capture_output=True, text=True, timeout=WHOAMI_SECONDS, check=False)
        identity = json.loads(done.stdout) if done.returncode == 0 else {}
    except UNREAD:
        return set()
    return {found for found in (identity.get("dm_channel_id"), identity.get("user_id")) if found}


def drive_home(evt: BaseHookEvent) -> bool:
    if not isinstance(evt, ToolHookEvent) or not (writes := pending(evt)):
        return False
    if (saved := bound_drive(evt._raw.get("session_id") or "") or named_drive()) is None:
        return False
    home = drive_channels(saved) | his_dm()
    return all(write.channel in home for write in writes)


class DriveHome:
    def check(self, evt: BaseHookEvent) -> bool:
        return drive_home(evt)
