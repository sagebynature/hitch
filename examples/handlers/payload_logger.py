#!/usr/bin/env python3
"""Append Hitch event payloads to a JSONL log file.

The handler keeps stdout reserved for the Hitch handler-result JSON. It writes
payload records to a file instead of printing them to stdout, because arbitrary
stdout text is parsed as a handler result by Hitch.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


def emit_ok() -> None:
    print(json.dumps({"status": "ok", "decision": {"behavior": "none"}}, separators=(",", ":")))


def emit_error(message: str) -> None:
    print(json.dumps({"status": "error", "decision": {"behavior": "none"}, "logs": [{"level": "error", "message": message}]}, separators=(",", ":")))


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Log Hitch event payloads as JSONL")
    parser.add_argument(
        "--log",
        default=os.environ.get("HITCH_PAYLOAD_LOG_PATH", "tmp/hitch-payload-logger/payloads.jsonl"),
        help="JSONL log path. Defaults to HITCH_PAYLOAD_LOG_PATH or tmp/hitch-payload-logger/payloads.jsonl.",
    )
    return parser.parse_args()


def compact_record(envelope: dict[str, Any]) -> dict[str, Any]:
    return {
        "logged_at": datetime.now(timezone.utc).isoformat(),
        "event_id": envelope.get("event_id"),
        "harness": envelope.get("harness"),
        "source_event_type": envelope.get("source_event_type"),
        "hitch_event_type": envelope.get("hitch_event_type"),
        "payload": envelope.get("payload"),
    }


def main() -> int:
    args = parse_args()
    try:
        envelope = json.load(sys.stdin)
    except json.JSONDecodeError as exc:
        emit_error(f"invalid Hitch envelope JSON: {exc}")
        return 0

    if not isinstance(envelope, dict):
        emit_error("Hitch envelope must be a JSON object")
        return 0

    path = Path(args.log)
    try:
        path.parent.mkdir(parents=True, exist_ok=True)
        with path.open("a", encoding="utf-8") as f:
            f.write(json.dumps(compact_record(envelope), separators=(",", ":"), sort_keys=True))
            f.write("\n")
    except OSError as exc:
        emit_error(f"could not write payload log {path}: {exc}")
        return 0

    emit_ok()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
