#!/usr/bin/env python3
"""Run the payload-logger handler against all Hitch harness mappers.

From the repository root:

    python3 examples/test_payload_logger.py

The script starts Hitch with examples/payload-logger.config.toml, sends events
for Codex, Hermes, Pi, and OMP, and verifies that each payload was appended to
tmp/hitch-payload-logger/payloads.jsonl.
"""

from __future__ import annotations

import json
import os
import signal
import subprocess
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
BASE_URL = "http://127.0.0.1:8797"
CONFIG = ROOT / "examples" / "payload-logger.config.toml"
RUNTIME_DIR = ROOT / "tmp" / "hitch-payload-logger"
DB = RUNTIME_DIR / "events.sqlite"
LOG = RUNTIME_DIR / "payloads.jsonl"


def request(method: str, path: str, body: dict[str, Any] | None = None) -> dict[str, Any]:
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(
        f"{BASE_URL}{path}",
        data=data,
        method=method,
        headers={"content-type": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=5) as response:
            return json.loads(response.read().decode())
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode()
        raise RuntimeError(f"{method} {path} failed with HTTP {exc.code}: {detail}") from exc


def wait_for_health(proc: subprocess.Popen[str]) -> None:
    deadline = time.monotonic() + 20
    while time.monotonic() < deadline:
        if proc.poll() is not None:
            stdout, _ = proc.communicate(timeout=1)
            raise RuntimeError(f"hitch server exited early with {proc.returncode}:\n{stdout}")
        try:
            if request("GET", "/v1/health").get("status") == "ok":
                return
        except (urllib.error.URLError, TimeoutError, ConnectionError):
            time.sleep(0.2)
    raise TimeoutError("hitch server did not become healthy")


def read_log_lines() -> list[dict[str, Any]]:
    if not LOG.exists():
        return []
    return [json.loads(line) for line in LOG.read_text().splitlines() if line.strip()]


def wait_for_log_count(count: int) -> list[dict[str, Any]]:
    deadline = time.monotonic() + 10
    while time.monotonic() < deadline:
        lines = read_log_lines()
        if len(lines) >= count:
            return lines
        time.sleep(0.1)
    raise TimeoutError(f"expected at least {count} payload log lines, found {len(read_log_lines())}")


def dispatch_sync(harness: str, native_event_type: str, native_payload: dict[str, Any]) -> None:
    request(
        "POST",
        "/v1/dispatch-sync",
        {
            "harness": harness,
            "native_event_type": native_event_type,
            "native_payload": native_payload,
            "source_adapter_version": "payload-logger-example",
        },
    )


def dispatch_async(harness: str, native_event_type: str, native_payload: dict[str, Any]) -> None:
    request(
        "POST",
        "/v1/events",
        {
            "harness": harness,
            "native_event_type": native_event_type,
            "native_payload": native_payload,
            "source_adapter_version": "payload-logger-example",
        },
    )


def main() -> int:
    RUNTIME_DIR.mkdir(parents=True, exist_ok=True)
    for path in (DB, Path(f"{DB}-shm"), Path(f"{DB}-wal"), LOG):
        if path.exists():
            path.unlink()

    proc = subprocess.Popen(
        ["go", "run", "./cmd/hitch", "serve", "--config", str(CONFIG)],
        cwd=ROOT,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        start_new_session=True,
        text=True,
    )
    try:
        wait_for_health(proc)

        dispatch_sync("codex", "PreToolUse", {"hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": {"command": "pwd"}})
        dispatch_sync("hermes", "pre_tool_call", {"tool_name": "bash", "input": {"command": "pwd"}})
        dispatch_sync("pi", "tool_call", {"name": "bash", "input": {"command": "pwd"}})
        dispatch_sync("omp", "tool_call", {"name": "bash", "input": {"command": "pwd"}})
        dispatch_async("codex", "UserPromptSubmit", {"hook_event_name": "UserPromptSubmit", "prompt": "log this payload asynchronously"})

        lines = wait_for_log_count(5)
        harnesses = {line["harness"] for line in lines}
        expected = {"codex", "hermes", "pi", "omp"}
        if harnesses != expected:
            raise AssertionError(f"expected harnesses {expected}, found {harnesses}")
        if not all("payload" in line for line in lines):
            raise AssertionError("every payload log line must include payload")

        print(f"payload log: {LOG}")
        for line in lines:
            print(json.dumps({"harness": line["harness"], "native_event_type": line["native_event_type"], "hitch_event_type": line["hitch_event_type"], "payload": line["payload"]}, indent=2, sort_keys=True))
        print("\nPayload logger test drive completed.")
        return 0
    finally:
        if proc.poll() is None:
            os.killpg(proc.pid, signal.SIGTERM)
            try:
                proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                os.killpg(proc.pid, signal.SIGKILL)
                proc.wait(timeout=5)


if __name__ == "__main__":
    raise SystemExit(main())
