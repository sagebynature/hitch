#!/usr/bin/env python3
"""Run a local end-to-end Hitch handler test drive.

From the repository root:

    python3 examples/test_drive.py

The script starts `go run ./cmd/hitch serve`, sends real sync event requests,
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
PORT = int(os.environ.get("HITCH_TEST_DRIVE_PORT", "8798"))
BASE_URL = f"http://127.0.0.1:{PORT}"
SOURCE_CONFIG = ROOT / "examples" / "test-drive.config.toml"
RUNTIME_DIR = ROOT / "tmp" / "hitch-test-drive"
CONFIG = RUNTIME_DIR / "test-drive.runtime.config.toml"
DB = RUNTIME_DIR / "events.sqlite"


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

def assert_port_is_free() -> None:
    try:
        request("GET", "/v1/health")
    except (urllib.error.URLError, TimeoutError, ConnectionError, RuntimeError):
        return
    raise RuntimeError(f"{BASE_URL} is already serving Hitch; stop it or set HITCH_TEST_DRIVE_PORT to a free port")


def write_runtime_config() -> None:
    text = SOURCE_CONFIG.read_text()
    text = text.replace("port = 8798", f"port = {PORT}", 1)
    text = text.replace('path = "tmp/hitch-test-drive/events.sqlite"', f"path = {json.dumps(str(DB))}", 1)
    text = text.replace('path = "tmp/hitch-test-drive/hitch.log"', f"path = {json.dumps(str(RUNTIME_DIR / 'hitch.log'))}", 1)
    text = text.replace('working_dir = "."', f"working_dir = {json.dumps(str(ROOT))}")
    CONFIG.write_text(text)


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


def dispatch(harness: str, source_event_type: str, source_payload: dict[str, Any]) -> tuple[dict[str, Any], str]:
    data = json.dumps({
        "mode": "sync",
        "harness": harness,
        "harness_version": "",
        "source_event_type": source_event_type,
        "source_payload": source_payload,
        "hitch_client_version": "example-test-drive",
    }).encode()
    req = urllib.request.Request(
        f"{BASE_URL}/v1/events",
        data=data,
        method="POST",
        headers={"content-type": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=5) as response:
            normalized_id = response.headers.get("X-Hitch-Normalized-Event-ID", "")
            return json.loads(response.read().decode()), normalized_id
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode()
        raise RuntimeError(f"POST /v1/events failed with HTTP {exc.code}: {detail}") from exc


def main() -> int:
    RUNTIME_DIR.mkdir(parents=True, exist_ok=True)
    assert_port_is_free()
    for suffix in ("", "-shm", "-wal"):
        path = Path(f"{DB}{suffix}")
        if path.exists():
            path.unlink()
    write_runtime_config()

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

        blocked, blocked_id = dispatch(
            "codex",
            "PreToolUse",
            {"hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": {"command": "rm -rf /"}},
        )
        print("codex PreToolUse block response:")
        print(json.dumps(blocked, indent=2, sort_keys=True))
        assert blocked["permissionDecision"] == "deny"

        injected, _ = dispatch(
            "codex",
            "UserPromptSubmit",
            {"hook_event_name": "UserPromptSubmit", "prompt": "Deploy this to production"},
        )
        print("\ncodex UserPromptSubmit context response:")
        print(json.dumps(injected, indent=2, sort_keys=True))
        assert "production work" in injected["additionalContext"]

        rewritten, _ = dispatch(
            "hermes",
            "pre_gateway_dispatch",
            {"message": "rewrite: summarize the last handler invocation"},
        )
        print("\nhermes pre_gateway_dispatch rewrite response:")
        print(json.dumps(rewritten, indent=2, sort_keys=True))
        assert rewritten["action"] == "rewrite"

        inspected = request("GET", f"/v1/events/{blocked_id}")
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
