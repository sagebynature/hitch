#!/usr/bin/env python3
"""Run a local end-to-end Hitch handler test drive.

From the repository root:

    python3 examples/test_drive.py

The script starts `go run ./cmd/hitch serve`, sends real sync dispatch requests,
prints the native harness responses, inspects the persisted audit record, and
then stops the server.
"""

from __future__ import annotations

import json
import os
import signal
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
BASE_URL = "http://127.0.0.1:8798"
CONFIG = ROOT / "examples" / "test-drive.config.toml"
DB = ROOT / "tmp" / "hitch-test-drive" / "events.sqlite"


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


def dispatch(harness: str, source_event_type: str, source_payload: dict[str, Any]) -> dict[str, Any]:
    return request(
        "POST",
        "/v1/dispatch-sync",
        {
            "harness": harness,
            "harness_version": "",
            "source_event_type": source_event_type,
            "source_payload": source_payload,
            "hitch_client_version": "example-test-drive",
        },
    )


def main() -> int:
    DB.parent.mkdir(parents=True, exist_ok=True)
    for suffix in ("", "-shm", "-wal"):
        path = Path(f"{DB}{suffix}")
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

        blocked = dispatch(
            "codex",
            "PreToolUse",
            {"hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": {"command": "rm -rf /"}},
        )
        print("codex PreToolUse block response:")
        print(json.dumps(blocked["native_response"], indent=2, sort_keys=True))
        assert blocked["aggregate"]["decision"]["behavior"] == "block"
        assert blocked["native_response"]["permissionDecision"] == "deny"

        injected = dispatch(
            "codex",
            "UserPromptSubmit",
            {"hook_event_name": "UserPromptSubmit", "prompt": "Deploy this to production"},
        )
        print("\ncodex UserPromptSubmit context response:")
        print(json.dumps(injected["native_response"], indent=2, sort_keys=True))
        assert "production work" in injected["native_response"]["additionalContext"]

        rewritten = dispatch(
            "hermes",
            "pre_gateway_dispatch",
            {"message": "rewrite: summarize the last handler invocation"},
        )
        print("\nhermes pre_gateway_dispatch rewrite response:")
        print(json.dumps(rewritten["native_response"], indent=2, sort_keys=True))
        assert rewritten["native_response"]["action"] == "rewrite"

        inspected = request("GET", f"/v1/events/{blocked['normalized_event_id']}")
        print("\ninspection summary:")
        print(json.dumps({
            "hitch_event_type": inspected["normalized"]["Envelope"]["hitch_event_type"] if "Envelope" in inspected["normalized"] else inspected["normalized"].get("envelope", {}).get("hitch_event_type"),
            "handler_invocations": len(inspected["handler_invocations"]),
            "native_responses": len(inspected["native_responses"]),
        }, indent=2, sort_keys=True))

        print("\nHitch test drive completed.")
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
