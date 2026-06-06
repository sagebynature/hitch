# Harness Contracts

For the full normalized event taxonomy and native source-event mapping, see [Hitch Events](events.md).

Hitch supports two dispatch modes:

- Async observer events use `POST /v1/events`.
- Sync control events use `POST /v1/dispatch-sync` and return a native response.

Initial verified harness contracts:

- Codex: maps native command hook payloads and translates decisions to Codex stdout JSON. Verified E2E: every supported Codex lifecycle event dispatches through the public API to the seeded `noop_observer` handler, produces no control-flow change, and persists the inbound event, normalized event, handler invocation, and native response. `PreToolUse` `deny` translation is also covered.
- Hermes: maps shell hook payloads and translates decisions to Hermes stdout JSON. Verified E2E: `pre_tool_call` `block` returns `{"action":"block","message":"..."}`.
- Pi / OMP: map extension callback events in Go and install managed TypeScript extensions that post observer callbacks plus extension `ctx` metadata to `/v1/events`, post return-capable control callbacks to `/v1/dispatch-sync`, apply `adapter_action` return values or mutations only for sync responses, and fail open when Hitch is unavailable. OMP uses its native `~/.omp/agent/extensions/hitch/index.ts` discovery path and current extension event names such as `session_before_branch`, `session.compacting`, and `auto_retry_*`.
- OpenCode: installs a managed TypeScript plugin into `~/.config/opencode/plugins/hitch.ts`; forwards typed plugin hooks to `/v1/dispatch-sync` and selected SDK events to `/v1/events`; translates normalized decisions into plugin-owned adapter actions (`noop`, `throw`, `set`, `append`, `inject_context`) for typed hooks; and fails open if Hitch is unavailable.

Adapter fail-open behavior:

- Async adapter calls print nothing and ignore Hitch network failure.
- Sync adapter calls emit a harness-native no-op response when Hitch is unreachable or returns no native response.

Unsupported source event types are rejected unless configured in the per-harness event map.
