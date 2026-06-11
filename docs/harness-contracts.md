# Harness Contracts

For the full normalized event taxonomy and native source-event mapping, see [Hitch Events](events.md).

Hitch uses one ingress endpoint:

- `POST /v1/events` with request `mode:"async"` for observer ingestion.
- `POST /v1/events` with request `mode:"sync"` for control-capable source events that return a native response.

Initial verified harness contracts:

- Codex: maps native command hook payloads and translates decisions to Codex stdout JSON. Verified E2E: every supported Codex lifecycle event dispatches through the public API, produces no control-flow change when no sync handler matches, and persists the inbound event, normalized event, and native response. `PreToolUse` `deny` translation is also covered.
- Claude Code: maps native command hook payloads and translates decisions to Claude Code stdout JSON through `~/.claude/settings.json` command hooks. Verified coverage includes supported Claude Code lifecycle events, `PreToolUse` and `PermissionRequest` permission decisions, `PostToolUse` result replacement, and secondary audit rows for stop and failure events.
- Hermes: maps shell hook payloads and translates decisions to Hermes stdout JSON. Verified E2E: `pre_tool_call` `block` returns `{"action":"block","message":"..."}`.
- Pi / OMP: map extension callback events in Go and install managed TypeScript extensions that post observer callbacks plus extension `ctx` metadata with `mode:"async"`, post return-capable control callbacks with `mode:"sync"`, apply `adapter_action` return values or mutations only for sync responses, and fail open when Hitch is unavailable. OMP uses its native `~/.omp/agent/extensions/hitch/index.ts` discovery path and current extension event names such as `session_before_branch`, `session.compacting`, and `auto_retry_*`.
- OpenCode: installs a managed TypeScript plugin into `~/.config/opencode/plugins/hitch.ts`; posts typed plugin hooks with `mode:"sync"` and selected SDK events with `mode:"async"` to `/v1/events`; translates normalized decisions into plugin-owned adapter actions (`noop`, `throw`, `set`, `append`, `inject_context`) for typed hooks; and fails open if Hitch is unavailable.
- Antigravity: installs managed hooks into `~/.gemini/config/hooks.json`; translates normalized decisions (`allow`, `deny`, `injectSteps`, `terminationBehavior`) to Antigravity stdout JSON.

Adapter fail-open behavior:

- Async adapter calls print nothing and ignore Hitch network failure.
- Sync adapter calls emit a harness-native no-op response when Hitch is unreachable or returns no native response.

Unsupported source event types are rejected unless configured in the per-harness event map.
