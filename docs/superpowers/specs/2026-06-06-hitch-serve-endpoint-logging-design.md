# Hitch Serve Endpoint Logging Design

## Status

Approved for spec writing after design review in chat.

## Goal

Add meaningful operational logging around `hitch serve` HTTP endpoints so operators can see request acceptance, rejection, dispatch mode, inspection reads, and sync completion outcomes without logging request payloads.

## Scope

In scope:

- request-level logging for:
  - `GET /v1/health`
  - `POST /v1/events`
  - `GET /v1/events/:id`
- hybrid log levels:
  - `info` for sync successes and request failures
  - `debug` for routine async success, health success, inspection success, and observer dispatch summaries
- safe metadata fields only:
  - `session_id`, `turn_id`, `cwd`, `model`
  - request mode, harness, source event type, Hitch event type, Hitch IDs, status, duration, handler counts, error text
- tests for log content and level behavior

Out of scope:

- logging raw `source_payload`
- generic HTTP middleware refactor
- changing handler-execution logging in `dispatch.Runner`
- changing sink configuration semantics in `internal/logging`

## Context

Current state:

- `cmd/hitch serve` logs startup and fatal server failure.
- `internal/api/server.go` serves the Hitch API but emits almost no endpoint-level request logs.
- `dispatch.Runner` already logs per-handler invocation start/completion details.

That leaves a gap between server startup and handler execution logs. Operators cannot easily tell:

- whether a request reached the server;
- whether it was accepted or rejected;
- whether it ran async or sync;
- which Hitch IDs were created;
- whether inspection reads succeeded;
- how many control or observer handlers ran at the request level.

## Decision

Add explicit endpoint lifecycle logs directly in `internal/api/server.go` using a small request-summary helper instead of generic HTTP middleware.

## Why this design

This keeps request logging close to the code that knows:

- the parsed request mode;
- harness and source event type;
- normalized Hitch event type;
- generated `event_id` and `normalized_event_id`;
- promoted safe metadata fields;
- whether the request took the async or sync path;
- handler counts and translated response size.

Middleware would add more plumbing for little gain because Hitch-specific fields become available only after request parsing and normalization.

## Logging model

### Request summary helper

Add a small helper in `internal/api/server.go` that accumulates request attributes and emits one structured log entry for the request outcome.

Expected responsibilities:

- start timestamp capture;
- store request basics (`method`, `path`);
- store parsed event request fields when available;
- store normalized envelope fields when available;
- store outcome fields (`status`, `duration_ms`, handler counts, native response bytes, error);
- emit at a caller-chosen level.

This helper should be local to the API package and should not become a reusable framework abstraction.

### Fields

Always include when available:

- `method`
- `path`
- `status`
- `duration_ms`

For `POST /v1/events`, include when available:

- `mode`
- `harness`
- `source_event_type`
- `hitch_event_type`
- `event_id`
- `normalized_event_id`
- `session_id`
- `turn_id`
- `cwd`
- `model`
- `control_handler_count`
- `observer_handler_count`
- `native_response_bytes`
- `error`

For `GET /v1/events/:id`, include when available:

- `normalized_event_id` or requested `id`
- `status`
- `duration_ms`
- `error`

Never include:

- `source_payload`
- normalized payload body
- request headers other than data already persisted elsewhere
- native response body

## Endpoint behavior

### `GET /v1/health`

On success:

- log at `debug`
- include `method`, `path`, `status`, `duration_ms`

No additional payload or config detail should be logged.

### `POST /v1/events`

#### Request rejection / validation failure

Examples:

- invalid JSON
- invalid request mode
- unsupported harness
- missing or invalid source payload
- unsupported source event
- sync request for observer-only source event
- normalize failure mapped to `400`

Behavior:

- log at `info`
- include request fields available before failure:
  - `method`, `path`, `mode` if parsed, `harness`, `source_event_type`, `status`, `duration_ms`, `error`

#### Async success

Behavior:

- log at `debug`
- include:
  - request metadata
  - normalized metadata
  - `status=202`
  - `duration_ms`
  - `observer_handler_count` is not yet known at response time, so omit it from the acceptance log
- observer dispatch completion is logged separately by the goroutine after dispatch finishes

#### Sync success

Behavior:

- log at `info`
- include:
  - request metadata
  - normalized metadata
  - `status=200`
  - `duration_ms`
  - `control_handler_count`
  - `native_response_bytes`
- observer dispatch completion is logged separately by the goroutine after dispatch finishes

#### Internal failure after acceptance work starts

Examples:

- audit store insert failure
- native translation failure
- inspection lookup failure where server returns `500`

Behavior:

- log at `info`
- include whatever request/normalized metadata is already available
- include `status` and `error`

### Observer dispatch follow-up log

`dispatchObservers` runs after async acceptance and after sync control dispatch.

Behavior:

- emit a separate `debug` summary after observer dispatch completes
- include:
  - `normalized_event_id`
  - `harness`
  - `source_event_type`
  - `hitch_event_type`
  - `session_id`, `turn_id`, `cwd`, `model` when available
  - `observer_handler_count`
  - `duration_ms`

This keeps async response logging fast while still giving operators visibility into observer execution at the request level.

### `GET /v1/events/:id`

On success:

- log at `debug`
- include `method`, `path`, requested `id`, `status`, `duration_ms`

On failure:

- log at `info`
- include `method`, `path`, requested `id`, `status`, `duration_ms`, `error`

## Interaction with existing logs

This design intentionally complements existing `dispatch.Runner` logs.

- API logs answer: what request came in, what path it took, what IDs it produced, and whether it succeeded.
- dispatch logs answer: which handlers ran, how long they took, and what status each invocation returned.

The API layer should not duplicate handler stdout/stderr or individual handler status details already covered by `dispatch.Runner`.

## Testing strategy

Add or update API tests to assert log behavior for:

1. `GET /v1/health` success
   - debug log exists
   - contains `method`, `path`, `status`

2. async event success
   - debug log exists
   - contains `mode=async`, harness/source event, normalized IDs, and `status=202`
   - does not include payload content

3. sync event success
   - info log exists
   - contains `mode=sync`, normalized IDs, `status=200`, `control_handler_count`, `native_response_bytes`

4. invalid mode rejection
   - info log exists
   - contains request basics and `error`

5. observer-only sync rejection
   - info log exists
   - contains harness/source event and rejection reason

6. inspection success/failure
   - debug on success, info on failure

7. observer dispatch completion
   - debug summary includes `observer_handler_count`

Prefer log assertions with a JSON `slog` handler writing into a buffer, matching the existing logging-test pattern already used in `internal/api/server_test.go`.

## Implementation notes

Likely touch points:

- `internal/api/server.go`
  - add request-summary helper
  - thread summary state through `handleHealth`, `handleEvent`, `handleSyncEvent`, `dispatchObservers`, `handleGetEvent`, and `writeError` call sites
- `internal/api/server_test.go`
  - add request-level log assertions

Avoid widening the change into:

- `internal/logging/logging.go`
- `dispatch.Runner`
- config schema changes

## Risks and mitigations

### Risk: noisy logs for routine traffic

Mitigation:

- keep health, async success, inspection success, and observer summary at `debug`
- keep sync success and failures at `info`

### Risk: accidental payload leakage

Mitigation:

- whitelist safe fields explicitly instead of serializing request structs or envelopes wholesale
- add tests that assert payload values are absent from logs

### Risk: duplicated or inconsistent fields across request paths

Mitigation:

- centralize attribute assembly in one small helper inside `internal/api/server.go`
- keep field names stable across endpoints

## Acceptance criteria

The design is satisfied when:

- `hitch serve` emits meaningful request-level logs for all three live API endpoints;
- successful sync requests are visible at `info` with mode, IDs, and handler-count context;
- routine async success and health checks remain `debug` only;
- failures are visible at `info` with actionable context;
- logs include safe metadata fields but never raw payloads;
- tests cover both log content and level selection.
