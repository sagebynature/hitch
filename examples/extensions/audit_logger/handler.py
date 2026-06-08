from __future__ import annotations

from pathlib import Path

from hitch_sdk import Context, HandlerResult


def handle(context: Context) -> HandlerResult:
    path = Path("tmp/hitch-native-audit.log")
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(
        f"{context.event.harness} {context.event.source_event_type} {context.event.hitch_event_type}\n",
        encoding="utf-8",
    )
    return HandlerResult.none()
