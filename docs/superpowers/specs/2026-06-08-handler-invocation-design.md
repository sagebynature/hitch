# Handler Invocation Design

## Status

Approved design baseline for implementation planning.

## Context

Hitch currently invokes configured handlers by matching handler `kind` and normalized `hitch_event_type`. Handlers receive the full normalized Hitch event envelope on stdin. This is useful, but it does not cover these required behaviors:

- handlers can filter by source hook identity: `harness + source_event_type`;
- handlers can filter by normalized Hitch event type;
- handlers can choose whether the primary argument is the original source payload or Hitch's transformed payload;
- a handler must not execute the same hook more than once for a given inbound event;
- handlers need two invocation types: shell commands and Python native extensions discovered from `~/.config/hitch/extensions`.

## Goals

- Keep existing shell handlers working with a clean cutover path.
- Add source-event and Hitch-event filters without introducing parallel matching semantics.
- Make payload selection explicit and testable.
- Make native extensions ergonomic for Python developers through a small Hitch SDK.
- Enforce dedupe in Hitch, not in handler scripts.
- Preserve auditability: every executed or skipped handler remains inspectable.

## Non-goals

- No in-process Python embedding in the Go daemon.
- No long-lived worker pool in the first design.
- No remote extension marketplace.
- No transform chaining changes beyond current dispatch aggregation behavior.

## Handler configuration

`handlers.<name>` remains the server-side unit of routing and audit. Existing configs continue to load through compatibility aliases.

New canonical fields:

```toml
[handlers.block_dangerous_shell]
type = "shell"
command = ["python3", "policy.py"]
kind = "control"
hitch_events = ["tool.requested"]
source_events = [{ harness = "codex", source_event_type = "PreToolUse" }]
payload = "hitch"
timeout_ms = 1000
on_error = "fail_closed"
on_timeout = "fail_closed"

[handlers.audit_native]
type = "native"
extension = "audit_logger"
kind = "observer"
hitch_events = ["*"]
payload = "source"
timeout_ms = 1000
on_error = "fail_open"
on_timeout = "fail_open"
```

Compatibility rules:

- Missing `type` means `shell`.
- Existing `events = ["tool.requested"]` is accepted as an alias for `hitch_events = ["tool.requested"]`.
- `command` is required for `type = "shell"`.
- `extension` is required for `type = "native"` when referenced from the main config.
- `payload` defaults to `hitch`.

Valid `payload` values:

- `hitch`: primary argument is `EventEnvelope.payload`.
- `source`: primary argument is `EventEnvelope.source_payload`.

## Native extension discovery

Hitch discovers native extensions under:

```text
~/.config/hitch/extensions/<extension-name>/
```

Each extension directory contains:

```text
config.toml
handler.py
```

Example extension config:

```toml
name = "audit_logger"
entrypoint = "handler:handle"
kind = "observer"
hitch_events = ["tool.completed"]
source_events = []
payload = "hitch"
timeout_ms = 1000
on_error = "fail_open"
on_timeout = "fail_open"
```

Discovery rules:

- `name` must match the directory name unless explicitly overridden by a main-config handler entry.
- `entrypoint` uses `module:function` syntax relative to the extension directory.
- Extension config defines the same routing properties as a normal handler.
- Main config may either reference an extension by name or override discovered properties through `[handlers.<name>]`.
- Invalid extension configs fail server config validation with a precise error.

Native execution is an isolated Python subprocess. Hitch does not embed Python and does not keep native workers alive between invocations in the first implementation.

## Matching semantics

A handler matches an event only when every configured filter passes:

1. `handler.kind == requested dispatch kind`.
2. Hitch-event filter passes:
   - empty is invalid;
   - `"*"` matches every `env.hitch_event_type`;
   - otherwise exact match on `env.hitch_event_type`.
3. Source-event filter passes:
   - empty means every source event;
   - otherwise exact match on both `env.harness` and `env.source_event_type`.

This supports three common policies without special cases:

- all normalized tool requests: `hitch_events = ["tool.requested"]`;
- only Codex `PreToolUse`: `source_events = [{ harness = "codex", source_event_type = "PreToolUse" }]`;
- only Codex `PreToolUse` after normalization to tool requested: both filters configured.

Handler selection remains sorted by handler name for deterministic aggregation.

## Invocation context

Hitch invokes both shell and native handlers with a versioned context wrapper.

```json
{
  "hitch_version": "0.1.0",
  "handler_name": "block_dangerous_shell",
  "handler_type": "shell",
  "kind": "control",
  "inbound_event_id": "in_example",
  "normalized_event_id": "norm_example",
  "payload_kind": "hitch",
  "payload": {"tool": {"name": "Bash", "command": "pwd"}},
  "event": {
    "harness": "codex",
    "source_event_type": "PreToolUse",
    "source_payload": {"tool_name": "Bash", "tool_input": {"command": "pwd"}},
    "hitch_event_type": "tool.requested",
    "payload": {"tool": {"name": "Bash", "command": "pwd"}}
  }
}
```

`payload` is the selected primary payload. `event.source_payload` and `event.payload` are always present so native handlers can inspect both source and Hitch payloads without changing configuration.

## Shell handler execution

Shell handlers execute the configured command as a child process.

- stdin receives the full invocation context JSON.
- argv receives one additional argument: the selected primary payload serialized as compact JSON.
- stdout remains reserved for one optional Hitch `HandlerResult` JSON object.
- stderr remains diagnostics.
- `HITCH_CHILD=1` remains set.
- Existing timeout, working directory, and error policy behavior remain unchanged.

This preserves command-handler simplicity while making the richer context available to handlers that want it.

## Native Hitch SDK

The Python SDK provides a small typed contract:

```python
from hitch_sdk import Context, HandlerResult

def handle(context: Context) -> HandlerResult:
    return HandlerResult.none()
```

`Context` contains:

- `hitch_version`
- `handler_name`
- `handler_type`
- `kind`
- `inbound_event_id`
- `normalized_event_id`
- `payload_kind`
- `payload`
- `event.harness`
- `event.source_event_type`
- `event.source_payload`
- `event.hitch_event_type`
- `event.payload`
- optional event metadata: `session_id`, `turn_id`, `cwd`, `model`, `transcript_path`, `received_at`, `event_id`

SDK helpers:

- `Context.from_stdin()`
- `Context.from_json(bytes | str)`
- `HandlerResult.none()`
- `HandlerResult.allow()`
- `HandlerResult.block(reason)`
- `HandlerResult.deny(reason)`
- `HandlerResult.inject_context(text)`
- `HandlerResult.transform(updated_input=value)`
- `HandlerResult.replace_result(updated_output=value)`
- `emit_result(result)`
- `run(entrypoint)` for script entrypoints

Native extension subprocess flow:

1. Hitch resolves extension directory and entrypoint.
2. Hitch launches Python with the SDK runner.
3. SDK imports the configured module and calls `handle(context)`.
4. SDK serializes the returned `HandlerResult` to stdout.
5. Hitch parses stdout using the existing handler-result normalization path.

## Dedupe model

Dedupe is enforced by Hitch before handler execution.

Definition: a handler may not execute the same hook more than once per inbound event.

Hook identity:

```text
<harness>:<source_event_type>:<hitch_event_type>:<kind>
```

Dedupe key:

```text
<inbound_event_id>:<handler_name>:<hook_identity>
```

Store changes:

- Add `inbound_event_id` to `handler_invocations`.
- Add `hook_key` to `handler_invocations`.
- Add unique index on `(inbound_event_id, handler_name, hook_key)`.

Execution flow:

1. Ingestion creates one inbound event and one or more normalized events.
2. Live dispatch uses only the primary normalized event.
3. Before running a handler, Hitch attempts to reserve `(inbound_event_id, handler_name, hook_key)`.
4. If reservation succeeds, Hitch runs the handler and updates the invocation row.
5. If reservation conflicts, Hitch skips execution and records or returns a skipped invocation status.

`HandlerStatus` gains `skipped` for dedupe skips. Skipped invocations are ignored by decision aggregation.

Replay behavior:

- Replay does not reuse the original inbound-event dedupe key for execution.
- Replay invocations keep `replay_source_id` and use the replay request's own dedupe scope.
- This preserves the ability to deliberately replay a stored event without violating live inbound-event dedupe.

## Dispatch and persistence flow

Sync request:

1. Normalize source event into primary Hitch envelope.
2. Persist inbound and normalized events.
3. Dispatch matching `control` handlers with dedupe reservation.
4. Persist invocation outputs.
5. Aggregate decisions deterministically.
6. Translate aggregate decision to harness-native response.
7. Persist native response.
8. Dispatch matching `observer` handlers for the same inbound event; dedupe prevents the same hook from running twice.

Async request:

1. Normalize and persist inbound/normalized events.
2. Dispatch matching `observer` handlers with dedupe reservation.
3. Return accepted response.

Secondary normalized events remain audit/query rows and are not live-dispatched.

## Validation

Config validation rejects:

- unknown handler `type`;
- invalid `payload` value;
- shell handler without `command`;
- native handler without resolvable extension or entrypoint;
- invalid Hitch event names in `hitch_events`;
- invalid harness names in `source_events`;
- empty `source_event_type` in `source_events`;
- both `events` and `hitch_events` with conflicting values.

## Testing plan

Required behavioral tests:

- existing handler configs using `events` still work;
- handler can match only by Hitch event;
- handler can match only by source event pair;
- handler with both filters requires both to match;
- shell handler receives selected source payload argument;
- shell handler receives selected Hitch payload argument;
- shell handler stdin context contains both payloads and metadata;
- native extension discovery loads `config.toml` and `handler.py`;
- native SDK `handle(context)` can return `HandlerResult` decisions;
- duplicate `(inbound_event_id, handler_name, hook_key)` skips execution;
- replay can execute handlers even when original inbound event already has invocations;
- skipped invocations do not affect aggregation;
- invalid extension configs fail validation clearly.

## Documentation updates

Update:

- `docs/configuration.md` for new handler fields;
- `docs/handler-development.md` for shell vs native handlers;
- `docs/handler-protocol.md` for invocation context and `skipped` status;
- `docs/events.md` only if terminology around source filters needs clarification;
- examples to include one shell handler using `payload = "source"` and one native extension.
