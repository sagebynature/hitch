# ADR-0002: Unified Events Endpoint and Dispatch Semantics

## Status

Accepted

## Date

2026-06-06

## Context

ADR-0001 separated Hitch ingress into two HTTP endpoints:

```text
POST /v1/events
POST /v1/dispatch-sync
```

That split reflected two real execution paths, but it encoded dispatch semantics in the URL rather than in the event request. Hitch now needs a cleaner API contract for harness adapters and developers:

- every harness forwards source lifecycle events to Hitch;
- some source events are observer-only and cannot consume a native response;
- some source events are control-capable and may consume a native response;
- passive observers are still useful on control-capable events, such as logging a `PreToolUse` request that also has a policy decision;
- control handlers must never run for observer-only requests or fire-and-forget calls;
- sync hook clients must fail open when Hitch is unavailable or returns no usable native response.

The previous endpoint split also made the term `mode` ambiguous. A request mode means what this specific harness call needs. A handler mode means whether a handler is an observer or a control participant. A source event capability means whether the native harness event can consume a response at all. These are related but distinct concepts.

## Decision Drivers

- Keep the public HTTP API small and easy for generated adapters to call.
- Make sync versus async dispatch explicit in the request payload.
- Preserve the ability to attach passive observers to control-capable events.
- Prevent control handlers from affecting observer-only or async requests.
- Preserve fail-open behavior for harness hooks when Hitch cannot provide a valid sync response.
- Make the model understandable to handler authors and adapter developers.
- Support clean cutover; Hitch is still in development and does not need `/v1/dispatch-sync` compatibility.

## Considered Options

### Option 1: Keep separate endpoints

Continue using `/v1/events` for async observer ingestion and `/v1/dispatch-sync` for sync control dispatch.

**Pros**

- Existing implementation already matches this shape.
- URL alone reveals the dispatch path.
- No request schema change required.

**Cons**

- Duplicates event ingestion API surface.
- Encodes dispatch semantics in transport routing instead of the event envelope.
- Makes adapter code branch on endpoints rather than setting a request property.
- Keeps reinforcing the overloaded `mode` terminology.

### Option 2: Single endpoint with request `mode`, keep handler `mode`

Use `POST /v1/events` for all event ingress. Add top-level request `mode: "async" | "sync"`, defaulting to `"async"`. Keep handler config as `mode = "async" | "sync"`.

**Pros**

- Small API surface.
- Minimal internal config churn.
- Request mode becomes explicit in the payload.

**Cons**

- `mode` still means two different things depending on whether it is on a request or handler.
- Handler docs must repeatedly clarify that handler mode is a capability/role, not request dispatch mode.

### Option 3: Single endpoint with request `mode`, handler `kind`, and source event capability

Use `POST /v1/events` for all event ingress. Add top-level request `mode: "async" | "sync"`, defaulting to `"async"`. Rename handler config to `kind = "observer" | "control"`. Classify source events as `observer_only` or `control_capable` inside harness adapters.

**Pros**

- Separates the three concepts cleanly:
  - request `mode`: what this call asks Hitch to do;
  - handler `kind`: whether a handler observes or controls;
  - source event capability: whether the harness event can consume a response.
- Allows observer handlers on sync/control-capable events without letting them affect the native response.
- Prevents control handlers on async requests by construction.
- Easier for future developers to reason about and validate.

**Cons**

- Requires config, tests, docs, and examples to move from handler `mode` to `kind`.
- Requires a source event capability catalog in the harness layer.
- Requires store/replay/log naming updates or a deliberate persistence compatibility choice.

## Decision

Adopt **Option 3**.

Hitch will expose one event ingress endpoint:

```text
POST /v1/events
```

The request payload controls dispatch:

```json
{
  "mode": "sync",
  "harness": "codex",
  "source_event_type": "PreToolUse",
  "source_payload": {},
  "hitch_client_version": "0.1.0"
}
```

Rules:

1. `mode` is top-level request metadata, not part of `source_payload`.
2. Missing or empty `mode` defaults to `"async"`.
3. Valid request modes are `"async"` and `"sync"`; other values return `400`.
4. `/v1/dispatch-sync` is removed.
5. Async requests run only `observer` handlers and return `202 Accepted` with Hitch event metadata.
6. Sync requests are valid only for `control_capable` source events.
7. Sync requests run `control` handlers first, aggregate their decisions, translate the aggregate into harness-native JSON, persist the native response, then run `observer` handlers for the same normalized event.
8. Sync responses return the harness-native JSON body directly. Hitch metadata moves to response headers such as `X-Hitch-Event-ID` and `X-Hitch-Normalized-Event-ID`.
9. A sync response with an empty body, invalid JSON body, or non-2xx status is treated by clients/adapters as fail-open passthrough.
10. Handler config uses `kind = "observer" | "control"`. `observer` handlers never affect native responses. `control` handlers may affect native responses and are never run for async requests.

Source event capability is not the same as normalized Hitch event type. A normalized event such as `tool.requested` may be emitted from both observer-only and control-capable native source events in different harnesses. Capability belongs to the harness source event contract.

## Dispatch Matrix

| Source event capability | Request `mode` | Control handlers | Observer handlers | Response |
| --- | --- | --- | --- | --- |
| `observer_only` | absent / `async` | No | Yes | `202` Hitch metadata JSON |
| `observer_only` | `sync` | No | No | `400` invalid request |
| `control_capable` | absent / `async` | No | Yes | `202` Hitch metadata JSON |
| `control_capable` | `sync` | Yes | Yes, after control dispatch | `200` harness-native JSON body |

## Rationale

An observer handler bound to a control-capable event is often correct. For example, a policy handler may block or allow Codex `PreToolUse`, while a logger observes the same attempted tool call for audit. The logger must not participate in the response decision, but it should still be able to subscribe to the event.

A control handler bound to an observer-only request is not useful. There is no native response channel to influence, so a `deny`, `transform`, or `replace_result` decision would be misleading at best. Hitch should reject sync requests for observer-only source events and should not run control handlers during async requests.

Keeping request mode, handler kind, and source event capability separate gives Hitch a small API without hiding important safety boundaries.

## Consequences

### Positive

- One event endpoint is easier to document, generate, and call.
- Handler authors get clearer vocabulary: observers watch; control handlers decide.
- Passive audit/log/index handlers remain usable for control-capable events.
- Control decisions cannot accidentally run on fire-and-forget events.
- Sync clients can process the response body directly as harness-native JSON.

### Negative

- Existing implementation and docs must be updated together.
- Existing development configs using `mode = "sync"` or `mode = "async"` must change to `kind = "control"` or `kind = "observer"`.
- Existing tests and examples referring to `/v1/dispatch-sync` must be rewritten.

### Risks and Mitigations

- **Risk:** developers confuse request `mode` and handler `kind` during migration.
  **Mitigation:** document the dispatch matrix in handler docs and harness contracts; reject old handler `mode` keys through strict config validation.

- **Risk:** a control-capable source event is misclassified as observer-only.
  **Mitigation:** keep capability definitions near each harness mapper and cover return-capable events with tests.

- **Risk:** clients lose Hitch metadata when sync responses become native-body-only.
  **Mitigation:** include `X-Hitch-Event-ID` and `X-Hitch-Normalized-Event-ID` headers and keep persisted inspection available via `GET /v1/events/:id`.

## Implementation Notes

- Add `Mode string `json:"mode"`` to the event request model.
- Add a request-mode parser that defaults missing/empty mode to `async` and rejects unknown modes.
- Replace `handleDispatchSync` with a sync branch inside `handleEvent`.
- For sync responses, write the native response bytes directly instead of wrapping them in `DispatchResponse`.
- Rename handler configuration from `Mode`/`mode` to `Kind`/`kind` and map values:
  - `async` → `observer`
  - `sync` → `control`
- Rename dispatch runner filtering from mode-based to kind-based.
- Add source event capability methods or tables in the harness layer.
- Keep the store field as either `kind` after a schema-version reset, or retain `mode` only as a historical column name if avoiding storage churn. Because Hitch is still in development, prefer a clean rename to `kind`.

## Related Decisions

- Refines ADR-0001, which originally separated `/v1/events` and `/v1/dispatch-sync`.
