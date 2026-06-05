# Server-Side Source Event Normalization Design

## Status

Approved on 2026-06-04.

## Problem

Hitch needs a sharper source boundary. Hook clients should report source facts, and the server should own all Hitch interpretation.

Current server ingress uses `EventRequest` fields named `native_event_type`, `native_payload`, and `source_adapter_version` in `internal/api/server.go:33-39`. The server already chooses harness mappers and calls `mapper.Map(...)` during ingest in `internal/api/server.go:100-116`, but each mapper owns a hard-coded native-to-Hitch event table such as `internal/harness/codex/codex.go:13-24`, `internal/harness/hermes/hermes.go:13-25`, `internal/harness/pi/pi.go:38-56`, and `internal/harness/omp/omp.go:13-22`.

This makes the mapping server-side in code, but not configurable. It also leaves handler envelope names and audit fields using `native_*`, while the intended contract is source-oriented and harness-neutral.

## Decision

Make a clean source-to-Hitch normalization cutover:

1. `hitch-client` sends a thin source-event request with source vocabulary.
2. The Hitch server maps `source_event_type` to `hitch_event_type` using per-harness default mappings merged with user config.
3. The Hitch server normalizes source-specific data into a consistent handler envelope before dispatch.
4. The handler envelope preserves exact source data in `source_payload` and exposes server-normalized data in `payload`.
5. Response translation remains harness-specific and keyed by `source_event_type`.

## Wire Request Contract

Both `POST /v1/events` and `POST /v1/dispatch-sync` accept this body:

```json
{
  "harness": "codex",
  "harness_version": "0.45.0",
  "source_event_type": "PreToolUse",
  "source_payload": {
    "session_id": "s1",
    "cwd": "/repo",
    "hook_event_name": "PreToolUse",
    "tool_name": "Bash",
    "tool_input": {
      "command": "go test ./..."
    }
  },
  "hitch_client_version": "0.1.0"
}
```

Required fields:

- `harness`
- `harness_version`
- `source_event_type`
- `source_payload`
- `hitch_client_version`

The client does not send `hitch_event_type`, event IDs, timestamps, session metadata, or normalized handler payloads.

## Handler Envelope Contract

Handlers receive a uniform Hitch envelope with source fields and normalized fields:

```json
{
  "hitch_version": "0.1.0",
  "event_id": "evt_...",
  "received_at": "2026-06-04T00:00:00Z",
  "harness": "codex",
  "harness_version": "0.45.0",
  "source_event_type": "PreToolUse",
  "source_payload": {
    "session_id": "s1",
    "cwd": "/repo",
    "hook_event_name": "PreToolUse",
    "tool_name": "Bash",
    "tool_input": {
      "command": "go test ./..."
    }
  },
  "hitch_event_type": "tool.requested",
  "session_id": "s1",
  "cwd": "/repo",
  "payload": {
    "tool": {
      "name": "Bash",
      "input": {
        "command": "go test ./..."
      }
    },
    "cwd": "/repo"
  }
}
```

Semantics:

- `source_payload` is the exact harness/source payload after only transport-envelope handling needed to expose the actual source event. Pi currently unwraps its installed extension transport envelope in `internal/harness/pi/pi.go:63-70`; that behavior should become explicit source normalization.
- `payload` is the Hitch-normalized handler payload. It should become stable by `hitch_event_type`, not by source harness.
- `source_event_type` identifies the source hook/callback name. It replaces `native_event_type` everywhere in public API and handler envelope JSON.
- `hitch_client_version` belongs to ingress/audit metadata, not the handler envelope unless a later schema revision needs it.

## Configuration Contract

Add event maps beneath each harness table:

```toml
[harness.codex]
enabled = true

[harness.codex.event_map]
PreToolUse = "tool.requested"
PermissionRequest = "tool.permission_requested"
UserPromptSubmit = "turn.user_prompt"

[harness.hermes.event_map]
pre_tool_call = "tool.requested"
pre_gateway_dispatch = "turn.user_prompt"

[harness.pi.event_map]
tool_call = "tool.requested"
input = "turn.user_prompt"
```

Semantics:

- `config/default.config.toml` carries the supported source-event mappings.
- Runtime mappings come from the loaded config, not hard-coded harness package maps.
- A configured source event may change a supported mapping or add a new source event.
- Values must be valid Hitch event types from `internal/protocol/protocol.go:27-42`.
- Unknown source event types are rejected unless they are configured.

This extends the existing config shape where `Config` owns `HarnessConfig` in `internal/config/config.go:16-22`, `HarnessConfig` owns four harness tables in `internal/config/config.go:81-86`, and `HarnessToggle` currently only carries `enabled` in `internal/config/config.go:88-90`.

## Server Pipeline

Introduce an explicit normalization pipeline inside `internal/api` and `internal/harness`:

```text
HTTP request
  -> SourceEventRequest validation
  -> source audit record
  -> harness normalizer lookup
  -> configured source_event_type -> hitch_event_type resolution
  -> source-specific metadata extraction
  -> canonical payload normalization
  -> protocol.EventEnvelope validation
  -> handler dispatch
  -> harness-native response translation
  -> response audit record
```

The harness interface should move from `Map(nativeEventType, nativePayload)` toward source vocabulary and explicit event type injection:

```go
type Normalizer interface {
    Normalize(req SourceEventRequest, hitchEventType protocol.EventType) (protocol.EventEnvelope, error)
    Translate(sourceEventType string, aggregate protocol.AggregateDecision) (protocol.RawJSON, error)
}
```

The server, not the mapper, resolves configurable mapping. The normalizer focuses on source-payload transformation and response translation.

## Canonical Payload Strategy

Implement canonical payloads incrementally, but put the boundary in place immediately.

Initial implementation:

- Rename wire/envelope/audit fields to source vocabulary.
- Add configurable event mapping.
- Preserve exact source payload in `source_payload`.
- Keep `payload` conservative for events without a defined canonical extractor.
- Preserve existing Pi metadata extraction behavior.

Then define canonical payload extractors event by event. First target should be `tool.requested`, because all current harnesses map tool-request-like source events to it and handlers commonly target that event. Later targets can include `tool.completed`, `turn.user_prompt`, `turn.started`, and session lifecycle events.

## Storage and Schema Impact

The current audit schema stores `native_event_type`, `native_payload_json`, and `source_adapter_version` in `inbound_events`, and `native_event_type` in `native_responses` (`internal/store/store.go:20-29` and `internal/store/store.go:57-64`). The Go structs mirror those names in `internal/store/store.go:102-143`.

Required migration:

- Add or migrate to `source_event_type`.
- Add or migrate to `source_payload_json`.
- Add or migrate to `hitch_client_version`.
- Add or migrate native response column naming to `source_event_type`.
- Bump store schema version.

The normalized event schema currently requires `native_event_type` and `native_payload` in `schemas/hitch-event-envelope.schema.json:5-15`. Update it to require `source_event_type` and `source_payload` instead.

## Compatibility

Because this is a public API and envelope rename, default to a clean cutover in code and docs. Do not keep old JSON fields in handler envelopes.

Optional transitional behavior, if desired during implementation, may be limited to request decoding only:

- Accept old `native_event_type` and `native_payload` only when the new fields are absent.
- Emit only source-named fields in responses, stored normalized envelopes, and handler stdin.

If this compatibility shim is added, it should be short and isolated in request decoding, not spread through protocol, store, or handlers.

## Testing Requirements

Automated coverage must prove:

- New request fields are accepted by `/v1/events` and `/v1/dispatch-sync`.
- Old handler envelope fields are gone from new normalized envelopes.
- Default mapping still matches current Codex, Hermes, Pi, and OMP behavior.
- Per-harness config entries override default source-event mappings.
- Per-harness config entries add a previously unsupported source event.
- Invalid configured Hitch event types fail config validation.
- Unsupported source event types still fail when not configured.
- Pi source payload unwrapping and metadata extraction still work.
- Sync dispatch translates aggregate decisions back using `source_event_type`.
- Audit inspection returns source-named fields consistently.
