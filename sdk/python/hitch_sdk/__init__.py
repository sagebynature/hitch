from __future__ import annotations

import importlib
import json
import sys
from dataclasses import dataclass
from typing import Any, Callable


@dataclass(frozen=True)
class Event:
    hitch_version: str
    event_id: str
    received_at: str
    harness: str
    source_event_type: str
    source_payload: dict[str, Any]
    hitch_event_type: str
    payload: dict[str, Any]
    session_id: str | None = None
    turn_id: str | None = None
    cwd: str | None = None
    model: str | None = None
    transcript_path: str | None = None


@dataclass(frozen=True)
class Context:
    hitch_version: str
    handler_name: str
    handler_type: str
    kind: str
    inbound_event_id: str
    normalized_event_id: str
    payload_kind: str
    payload: dict[str, Any]
    event: Event
    event_id: str | None = None
    received_at: str | None = None
    harness: str | None = None
    source_event_type: str | None = None
    source_payload: dict[str, Any] | None = None
    hitch_event_type: str | None = None
    session_id: str | None = None
    turn_id: str | None = None
    cwd: str | None = None
    model: str | None = None
    transcript_path: str | None = None

    @classmethod
    def from_json(cls, raw: bytes | str) -> "Context":
        data = json.loads(raw)
        event = Event(**data["event"])
        return cls(event=event, **{k: v for k, v in data.items() if k != "event"})

    @classmethod
    def from_stdin(cls) -> "Context":
        return cls.from_json(sys.stdin.read())


@dataclass(frozen=True)
class HandlerResult:
    status: str = "ok"
    decision: dict[str, Any] | None = None

    @staticmethod
    def none() -> "HandlerResult":
        return HandlerResult(decision={"behavior": "none"})

    @staticmethod
    def allow() -> "HandlerResult":
        return HandlerResult(decision={"behavior": "allow"})

    @staticmethod
    def deny(reason: str) -> "HandlerResult":
        return HandlerResult(decision={"behavior": "deny", "reason": reason})

    @staticmethod
    def block(reason: str) -> "HandlerResult":
        return HandlerResult(decision={"behavior": "block", "reason": reason})

    @staticmethod
    def inject_context(text: str) -> "HandlerResult":
        return HandlerResult(decision={"behavior": "inject_context", "context": text})

    @staticmethod
    def transform(updated_input: Any) -> "HandlerResult":
        return HandlerResult(decision={"behavior": "transform", "updated_input": updated_input})

    @staticmethod
    def replace_result(updated_output: Any) -> "HandlerResult":
        return HandlerResult(decision={"behavior": "replace_result", "updated_output": updated_output})

    def to_json(self) -> str:
        body: dict[str, Any] = {"status": self.status}
        if self.decision is not None:
            body["decision"] = self.decision
        return json.dumps(body, separators=(",", ":"))


def emit_result(result: HandlerResult) -> None:
    sys.stdout.write(result.to_json())


def run(entrypoint: str) -> None:
    module_name, function_name = entrypoint.split(":", 1)
    module = importlib.import_module(module_name)
    func: Callable[[Context], HandlerResult] = getattr(module, function_name)
    result = func(Context.from_stdin())
    emit_result(result)
