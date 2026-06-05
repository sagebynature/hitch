#!/usr/bin/env python3
"""Example Hitch handler.

This handler demonstrates common sync decisions against real Hitch envelopes:
- block dangerous tool requests
- allow safe tool requests
- inject context for user prompts
- rewrite Hermes gateway messages
- replace transform hook output

It writes a single handler-result JSON object to stdout. Logs go to stderr.
"""

from __future__ import annotations

import json
import sys
from typing import Any


def result(behavior: str, **decision_fields: Any) -> None:
    print(json.dumps({"status": "ok", "decision": {"behavior": behavior, **decision_fields}}, separators=(",", ":")))


def none() -> None:
    result("none")


def text_from_payload(payload: dict[str, Any]) -> str:
    for key in ("prompt", "input", "message", "text"):
        value = payload.get(key)
        if isinstance(value, str):
            return value
    return ""


def command_from_payload(payload: dict[str, Any]) -> str:
    tool_input = payload.get("tool_input")
    if isinstance(tool_input, dict):
        command = tool_input.get("command")
        if isinstance(command, str):
            return command

    input_value = payload.get("input")
    if isinstance(input_value, dict):
        command = input_value.get("command")
        if isinstance(command, str):
            return command

    for key in ("command", "cmd"):
        value = payload.get(key)
        if isinstance(value, str):
            return value
    return ""


def main() -> int:
    try:
        envelope = json.load(sys.stdin)
    except json.JSONDecodeError as exc:
        print(json.dumps({"status": "error", "decision": {"behavior": "none"}, "logs": [{"level": "error", "message": str(exc)}]}))
        return 0

    event_type = envelope.get("hitch_event_type")
    source_event_type = envelope.get("source_event_type")
    payload = envelope.get("payload")
    if not isinstance(payload, dict):
        none()
        return 0

    if event_type == "tool.requested":
        command = command_from_payload(payload)
        if "rm -rf /" in command or "mkfs" in command:
            result("block", reason="Dangerous shell command blocked by Hitch example policy")
            return 0
        result("allow")
        return 0

    if event_type == "turn.user_prompt":
        text = text_from_payload(payload)
        if "production" in text.lower():
            result("inject_context", context="Before production work: confirm environment, rollback plan, and approval ticket.")
            return 0
        if source_event_type == "pre_gateway_dispatch" and text.startswith("rewrite:"):
            result("transform", updated_input=text.removeprefix("rewrite:").strip())
            return 0
        none()
        return 0

    if source_event_type in {"transform_tool_result", "transform_terminal_output", "transform_llm_output"}:
        output = payload.get("output")
        if isinstance(output, str):
            result("replace_result", updated_output=f"[reviewed by hitch] {output}")
            return 0

    none()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
