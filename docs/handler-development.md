# Building Hitch Event Handlers

This guide is for developers who build Hitch handlers: local commands that receive normalized Hitch events, enforce policy, add context, rewrite inputs, or replace outputs.

## What a handler is

A Hitch handler is any executable command listed in the Hitch config. Hitch runs matching handlers with:

- one normalized event envelope on stdin;
- `HITCH_CHILD=1` in the environment;
- stdout reserved for one optional handler-result JSON object;
- stderr captured as handler diagnostic output;
- a per-handler timeout.

Handlers can be shell scripts, Python programs, Node scripts, Go binaries, or any command that reads stdin and writes JSON to stdout.

## Handler types

Hitch separates request mode, handler kind, and source event capability. Request `mode` is a top-level HTTP field (`async` or `sync`, default `async`). Handler `kind` is a config field (`observer` or `control`). Source event capability is the harness contract: some source events are observer-only, while others can consume a native response.

| Handler kind | Configure with | Triggered by | Use for | Response path |
| --- | --- | --- | --- | --- |
| Observer | `kind = "observer"` | Async requests, and after sync control dispatch for the same normalized event | Audit, metrics, passive logging, background indexing | Hitch ignores handler decisions for native response purposes. |
| Control | `kind = "control"` | Sync requests for control-capable source events | Blocking tools, allowing tools, adding prompt context, rewriting input, replacing output | Hitch aggregates decisions and returns harness-native JSON. |

Observer handlers may subscribe to control-capable events, such as logging a tool request while a separate control handler decides allow/deny. Control handlers never run during async requests, and sync requests for observer-only source events are rejected.

## Step 1: Pick the Hitch event to handle

Handlers subscribe to normalized Hitch event names, not native harness names. Use [`events.md`](events.md) to map native harness events to Hitch events.

Common mappings:

| Developer goal | Hitch event | Example source events |
| --- | --- | --- |
| Inspect or block a tool before it runs | `tool.requested` | Codex `PreToolUse`, Hermes `pre_tool_call`, Pi/OMP `tool_call`, OpenCode `tool.execute.before` |
| Inspect a completed tool result | `tool.completed` | Codex `PostToolUse`, Hermes `transform_tool_result`, Pi/OMP `tool_result`, OpenCode `tool.execute.after` |
| Add context to a user request | `turn.user_prompt` | Codex `UserPromptSubmit`, Hermes `pre_gateway_dispatch`, Pi/OMP `input`, OpenCode `chat.message` |
| Inspect or augment an LLM request | `llm.requested` | Hermes `pre_llm_call`; Pi/OMP `before_provider_request`; OpenCode `chat.params` or `chat.headers` when opted in |
| Run at model or agent turn start | `turn.started` | Pi `turn_start`, OMP `turn_start` |
| Run after model or agent turn completion | `turn.completed` | Codex `Stop`, Pi/OMP `turn_end`, OpenCode `session.idle` |
| Query assistant output completion consistently | `turn.assistant_completed` | OMP `message_end`; secondary audit rows configured for Codex `Stop`, Hermes `transform_llm_output`, Pi `turn_end`, and OpenCode `session.idle` |
| Handle compaction lifecycle | `session.compacted` | Codex `PreCompact`, Pi `session_before_compact`, OMP `auto_compaction_start`, OpenCode `experimental.session.compacting` |

Secondary audit rows from multi-event source mappings are not used for sync native-response translation or live handler dispatch. If a handler must block, transform, replace, or observe a live source event, register for the primary Hitch event. Query secondary events such as `turn.assistant_completed` when you need a consistent audit signal across harnesses.

You may also subscribe to `"*"` during development. For production handlers, prefer explicit event names.

## Step 2: Add the handler to config

Add a `[handlers.<name>]` table to your Hitch config.

```toml
[handlers.block_dangerous_shell]
command = ["python3", "examples/handlers/policy_handler.py"]
working_dir = "."
events = ["tool.requested"]
kind = "control"
timeout_ms = 1000
on_error = "fail_closed"
on_timeout = "fail_closed"
```

Fields:

| Field | Required | Meaning |
| --- | --- | --- |
| `command` | Yes | Command and arguments passed to `exec`. Hitch writes the envelope to stdin. |
| `working_dir` | No | Directory Hitch runs the command from. Relative values in loaded config files resolve from the config file directory. |
| `events` | Yes | Normalized Hitch events to match. Use `"*"` to match all events. |
| `kind` | Yes | `"control"` or `"observer"`. Control handlers may affect sync native responses; observer handlers never do. |
| `timeout_ms` | Yes | Per-handler timeout. Hitch records `timeout` if exceeded. |
| `on_error` | No | `fail_open`, `fail_closed`, or `native_default`. Current dispatch treats `fail_closed` as a blocking aggregate decision; other values continue. |
| `on_timeout` | No | Same policy values as `on_error`, applied to timeout results. |

Handler names determine deterministic aggregation order. Hitch sorts matching handler names lexicographically before aggregating results. Use numeric prefixes when order matters, such as `00_context` and `10_policy`.

## Step 3: Read the event envelope

The handler receives the normalized envelope on stdin.

```json
{
  "hitch_version": "0.1.0",
  "event_id": "evt_...",
  "received_at": "2026-06-04T00:00:00Z",
  "harness": "codex",
  "source_event_type": "PreToolUse",
  "source_payload": {
    "hook_event_name": "PreToolUse",
    "tool_name": "Bash",
    "tool_input": {"command": "pwd"}
  },
  "hitch_event_type": "tool.requested",
  "payload": {
    "tool": {
      "name": "Bash",
      "kind": "shell",
      "input": {"command": "pwd"},
      "command": "pwd"
    }
  }
}
```

`payload` is the Hitch-normalized handler contract for the event type. Use `payload.tool`, `payload.turn`, or `payload.llm` for common fields; inspect `source_payload`, `harness`, and `source_event_type` only when a harness-specific field is required or the typed payload is marked `{"unparsed": true}`.

## Step 4: Return a handler result

A handler may write one JSON object to stdout.

```json
{
  "status": "ok",
  "decision": {
    "behavior": "none"
  }
}
```

If stdout is empty, Hitch records an `ok` result with `behavior: "none"`. If stdout is invalid JSON, Hitch records the invocation as `error`.

### Result fields

| Field | Type | Meaning |
| --- | --- | --- |
| `status` | `ok`, `error`, `timeout` | Handler execution status. Most handlers should return `ok`; Hitch sets `error` and `timeout` for process failures. |
| `decision.behavior` | Decision behavior | The normalized decision Hitch aggregates. Defaults to `none` when omitted. |
| `decision.reason` | String | Human-readable reason for block, deny, stop, or diagnostics. |
| `decision.context` | String | Additional context for `inject_context`. |
| `decision.updated_input` | Any JSON | Replacement or transformed input for `transform`. |
| `decision.updated_output` | Any JSON | Replacement output for `replace_result` or output transforms. |
| `decision.native_response` | Any JSON | Harness-native response escape hatch. Bypasses Hitch translation. |
| `logs` | Array | Structured handler logs persisted with the result. |
| `metrics` | Object of numbers | Numeric metrics persisted with the result. |

## Decision behavior reference

| Behavior | Use when | Aggregation precedence | Typical sync translation |
| --- | --- | --- | --- |
| `none` | The handler observes but does not change flow. | 0 | Empty native response or adapter `noop`. |
| `allow` | The handler explicitly permits a gated action. | 1 | Codex `PreToolUse` allow; Hermes gateway allow; Pi input continue. |
| `inject_context` | The handler adds context to a prompt or LLM call. | 2 | Codex `additionalContext`; Hermes `context`; aggregated contexts are joined with a blank line. |
| `transform` | The handler rewrites input or participates in a transform hook. | 3 | Codex `updatedInput`; Hermes gateway rewrite; Pi event mutation; Hermes transform result when `updated_output` is set. |
| `replace_result` | The handler replaces a tool, terminal, or LLM result. | 3 | Hermes transform hooks return `result`; Pi `tool_result` returns `updated_output`. |
| `handled` | The handler consumed the request and the harness should skip default processing. | 4 | Hermes gateway skip; Pi input handled. |
| `deny` | The handler denies a permission request or tool action. | 5 | Codex permission denial; blocking translations for tool events. |
| `block` | The handler blocks the action with a reason. | 5 | Codex block/deny/stop output; Hermes `action: "block"`; Pi block object. |
| `stop` | The handler stops a turn, compaction, or tool action. | 5 | Codex stop; Pi cancellation for session transition hooks. |

Higher precedence wins. Multiple `inject_context` decisions concatenate in handler-name order. Multiple `transform` or `replace_result` decisions are rejected by the aggregator; design your config so only one handler owns each rewrite.

## Step 5: Write a handler

This minimal Python handler blocks dangerous shell commands and allows other tool requests.

```python
#!/usr/bin/env python3
import json
import sys


def emit(behavior, **fields):
    print(json.dumps({"status": "ok", "decision": {"behavior": behavior, **fields}}))


envelope = json.load(sys.stdin)
payload = envelope.get("payload", {})
command = payload.get("tool_input", {}).get("command", "")

if envelope.get("hitch_event_type") == "tool.requested" and "rm -rf /" in command:
    emit("block", reason="Dangerous shell command blocked")
else:
    emit("allow")
```

Rules for production handlers:

1. Write only the JSON result to stdout.
2. Write debugging text to stderr, not stdout.
3. Return quickly. Hitch enforces `timeout_ms`.
4. Treat missing fields as normal. Different harnesses use different source payload shapes.
5. Avoid calling Hitch recursively from inside a handler. Hitch sets `HITCH_CHILD=1` so adapters can no-op inside handler children.
6. Use `native_response` only when Hitch's normalized behaviors cannot express the harness response you need.

## Step 6: Test-drive the included example

The repository includes a runnable policy handler and an end-to-end test drive:

- `examples/handlers/policy_handler.py`
- `examples/test-drive.config.toml`
- `examples/test_drive.py`

Run it from the repository root:

```bash
python3 examples/test_drive.py
```

The script starts `go run ./cmd/hitch serve --config examples/test-drive.config.toml`, sends real `POST /v1/events` requests with `mode:"sync"`, verifies native responses, inspects the persisted audit record, and stops the server.

Expected output shape:

```text
codex PreToolUse block response:
{
  "permissionDecision": "deny",
  "permissionDecisionReason": "Dangerous shell command blocked by Hitch example policy"
}

codex UserPromptSubmit context response:
{
  "additionalContext": "Before production work: confirm environment, rollback plan, and approval ticket."
}

hermes pre_gateway_dispatch rewrite response:
{
  "action": "rewrite",
  "message": "summarize the last handler invocation"
}

Hitch test drive completed.
```

## Step 7: Log payloads for every harness

Use `examples/handlers/payload_logger.py` when you want an observer that records the normalized payload from every harness without changing control flow.

The handler appends JSON Lines records to a file and returns `behavior: "none"`. It does not print payloads to stdout because Hitch parses stdout as the handler result.

Example log record:

```json
{
  "logged_at": "2026-06-04T00:00:00+00:00",
  "event_id": "evt_...",
  "harness": "codex",
  "source_event_type": "PreToolUse",
  "hitch_event_type": "tool.requested",
  "payload": {
    "hook_event_name": "PreToolUse",
    "tool_name": "Bash",
    "tool_input": {"command": "pwd"}
  }
}
```

Use both control and observer entries when you deliberately want to log both stages of sync requests during development:

```toml
[handlers.payload_logger_control]
command = ["python3", "examples/handlers/payload_logger.py", "--log", "tmp/hitch-payload-logger/payloads.jsonl"]
events = ["*"]
kind = "control"
timeout_ms = 1000
on_error = "fail_open"
on_timeout = "fail_open"

[handlers.payload_logger_observer]
command = ["python3", "examples/handlers/payload_logger.py", "--log", "tmp/hitch-payload-logger/payloads.jsonl"]
events = ["*"]
kind = "observer"
timeout_ms = 1000
on_error = "fail_open"
on_timeout = "fail_open"
```

`examples/payload-logger.config.toml` enables Codex, Hermes, Pi, OMP, and OpenCode and includes both handler entries.

Run the all-harness local test drive:

```bash
python3 examples/test_payload_logger.py
```

The script starts Hitch on `127.0.0.1:8797`, sends one event through each harness mapper, sends one async observer event, verifies that `tmp/hitch-payload-logger/payloads.jsonl` contains payload records for `codex`, `hermes`, `pi`, `omp`, and `opencode`, and stops the server. If port `8797` is already in use, run with `HITCH_PAYLOAD_LOGGER_PORT=8796 python3 examples/test_payload_logger.py`.

To test manually, start Hitch:

```bash
go run ./cmd/hitch serve --config examples/payload-logger.config.toml
```

If you installed real Hermes shell hooks against this example server, include the same URL when installing:

```bash
hitch-client install --only hermes --url http://127.0.0.1:8797 --yes --json
```

Without `--url`, installed hooks resolve the Hitch API URL at runtime from `HITCH_URL`, `~/.config/hitch/config.toml`, then `http://127.0.0.1:8799`. Generated hook commands include `-url` only when `--url` pins one; handler config changes affect `hitch serve`, and runtime URL resolution lets unpinned hook shims follow later config/env changes.

Then send a sync event through the harness client:

```bash
printf '%s\n' '{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"pwd"}}' \
  | go run ./cmd/hitch-client \
      --harness codex \
      --event PreToolUse \
      --sync \
      --url http://127.0.0.1:8797
```

Inspect the JSON Lines payload log one record at a time:

```bash
python3 - <<'PY'
import json
from pathlib import Path
for line in Path("tmp/hitch-payload-logger/payloads.jsonl").read_text().splitlines():
    print(json.dumps(json.loads(line), indent=2, sort_keys=True))
PY
```

Use `fail_open` for payload logging so a logging failure does not block the harness.


## Step 8: Test a policy handler manually

You can call Hitch through the client shim. Start the policy test server:

```bash
go run ./cmd/hitch serve --config examples/test-drive.config.toml
```

In another terminal, send a Codex `PreToolUse` event:

```bash
printf '%s\n' '{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"rm -rf /"}}' \
  | go run ./cmd/hitch-client \
      --harness codex \
      --event PreToolUse \
      --sync \
      --url http://127.0.0.1:8798
```

Expected native Codex response:

```json
{"permissionDecision":"deny","permissionDecisionReason":"Dangerous shell command blocked by Hitch example policy"}
```

## Step 9: Inspect and replay

The sync dispatch response contains `normalized_event_id`. Use it to inspect the audit trail:

```bash
go run ./cmd/hitch inspect-event --config examples/test-drive.config.toml norm_...
```

The inspection output includes:

- inbound source event and headers;
- normalized Hitch envelope;
- handler invocation status, stdout, stderr, parsed output, and decision;
- native response emitted back to the harness.

Replay the same normalized event through current sync handlers:

```bash
go run ./cmd/hitch replay --config examples/test-drive.config.toml --dry-run norm_...
go run ./cmd/hitch replay --config examples/test-drive.config.toml norm_...
```

Use dry-run first when you only want to verify the stored event. Non-dry-run replay records new handler invocations linked to the original normalized event.

## Troubleshooting

### The handler never runs

Check all of these:

- The request `mode` and source event capability can reach the handler kind: async requests run observer handlers; sync requests for control-capable source events run control handlers first and observer handlers afterward.
- `events` contains the normalized Hitch event, not the source event.
- The source event maps to the Hitch event you expect. See [`events.md`](events.md).
- The command path is correct relative to the Hitch server working directory.

### The harness ignores my decision

Check the source event supports that behavior. For example, `inject_context` translates for Codex prompt/session/tool-complete events and Hermes `pre_llm_call`, but it is not meaningful for every event.

### My handler output is recorded as an error

Hitch parses stdout as one JSON object. Remove logging from stdout and send diagnostics to stderr.

### Two rewrite handlers conflict

Hitch accepts only one `transform` or `replace_result` decision in an aggregate. Split handlers by event, or put the rewrite decision in one handler and make the others observers.

### The handler blocks when it fails

That is expected when `on_error = "fail_closed"` or `on_timeout = "fail_closed"`. Use `fail_open` for handlers that should never block the harness.
