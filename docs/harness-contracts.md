# Harness Contracts

Hitch supports two dispatch modes:

- Async observer events use `POST /v1/events`.
- Sync control events use `POST /v1/dispatch-sync` and return a native response.

Initial verified harness contracts:

- Codex: maps native command hook payloads and translates decisions to Codex stdout JSON. Verified E2E: every supported Codex lifecycle event dispatches through the public API to the seeded `noop_observer` handler, produces no control-flow change, and persists the inbound event, normalized event, handler invocation, and native response. `PreToolUse` `deny` translation is also covered.
- Hermes: maps shell hook payloads and translates decisions to Hermes stdout JSON. Verified E2E: `pre_tool_call` `block` returns `{"action":"block","message":"..."}`.
- Pi/OMP: maps extension callback events in Go and shares a TypeScript adapter-response contract for callback return values or in-place event mutation. Verified adapter helper behavior includes nested mutation paths such as `["input", "command"]`, return values, patch values, and recursion guard no-op.

Adapter fail-open behavior:

- Async adapter calls print nothing and ignore Hitch network failure.
- Sync adapter calls emit a harness-native no-op response when Hitch is unreachable or returns no native response.

Unsupported native event types are rejected unless a future passthrough mode is configured.
