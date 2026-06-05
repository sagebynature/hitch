# Hitch Events

Hitch turns source hook payloads from Codex, Hermes, Pi, and OMP into one stable event envelope. Handlers receive the normalized envelope on stdin and return one handler result on stdout.

## Normalized envelope

Every harness normalizer creates the same envelope shape:

| Field | Source | Notes |
| --- | --- | --- |
| `hitch_version` | Hitch runtime | Current protocol version. |
| `event_id` | Hitch runtime | Unique `evt_...` identifier generated when Hitch receives the event. |
| `received_at` | Hitch runtime | UTC timestamp generated at receipt. |
| `harness` | Adapter selection | One of `codex`, `hermes`, `pi`, or `omp`. |
| `source_event_type` | Harness source event name | The exact source hook or callback name Hitch mapped. |
| `source_payload` | Harness source payload | The original JSON payload received by Hitch. Pi unwraps the installed extension transport envelope before normalization. |
| `hitch_event_type` | Server event map | One of the normalized event names below. |
| `session_id`, `turn_id`, `cwd`, `model`, `transcript_path` | Server normalizer | Optional. Normalizers copy these from source payloads when available; Hermes also uses `extra.task_id` as a `session_id` fallback when it is meaningful. |
| `payload` | Server normalizer | Hitch-normalized handler payload. Current normalizers conservatively preserve the source payload after any adapter transport unwrapping. |

Unsupported source event types are rejected unless configured in the harness event map. Hitch does not silently coerce unknown source events into a generic type.


## Configurable source event mapping

`internal/config/default.config.toml` carries the supported source-event mappings for each harness. Edit or extend the relevant table in user config to change how a source hook maps to a Hitch event:
```toml
[harness.codex.event_map]
PreToolUse = "tool.permission_requested"
Stop = ["turn.completed", "turn.assistant_completed"]
CustomHook = "turn.started"
```

Keys are source hook/callback names. Values may be a single Hitch event name or an ordered list of Hitch event names from the taxonomy below. The first event is the primary mapping used for sync dispatch and native-response translation; additional events are persisted as secondary audit/query rows tied to the same inbound event. Unknown source events are rejected unless they appear in the configured event map. The tables below include the full known source-event catalog; `Default = No` means Hitch deliberately excludes the event from the seeded server map to avoid duplicate or high-volume audit records, but operators can opt in by adding that row to their own `[harness.<name>.event_map]`.

## Hitch event taxonomy

| Hitch event | Meaning |
| --- | --- |
| `session.started` | A harness session or top-level agent lifecycle starts. |
| `session.resumed` | A session switch, fork, or resume-like transition is about to occur. |
| `session.ended` | A session or agent lifecycle ends. |
| `session.compacted` | Context compaction starts, completes, or is about to run. |
| `turn.started` | A model or agent turn begins. |
| `turn.user_prompt` | User input or gateway dispatch is submitted. |
| `turn.assistant_started` | Assistant output starts; retained for compatibility with earlier configs. |
| `turn.assistant_completed` | Assistant output is available. Hitch may persist this as a secondary audit row in addition to the primary source-event mapping. |
| `turn.completed` | A model or agent turn completes. This is distinct from assistant-output completion when a harness exposes both boundaries. |
| `llm.requested` | A provider/LLM request is about to be sent. High-volume provider payloads are opt-in for Pi/OMP defaults. |
| `llm.completed` | A provider/LLM response or transformed model output is available. |
| `tool.requested` | A tool command, shell command, or tool call is requested before execution. |
| `tool.permission_requested` | A harness-specific permission decision is requested. |
| `tool.completed` | A final tool call, tool result, terminal output, or transformed tool result is available. |
| `tool.progress` | Incremental tool output is available before final completion. Intended for opt-in streaming/progress handlers. |
| `retry.started` | A harness retry cycle starts. |
| `retry.completed` | A harness retry cycle completes. |
| `subagent.started` | A subagent starts. |
| `subagent.completed` | A subagent completes or stops. |
| `error.reported` | A normalized harness error or credential failure is reported. |

## Secondary normalized events

Most source events produce one primary normalized event from `[harness.<name>.event_map]`. When a source event maps to an ordered list, Hitch also persists secondary normalized audit rows so queries can interpret the same source event through multiple cross-harness lifecycle meanings:

| Source event | Configured Hitch events |
| --- | --- |
| Codex `Stop` | `["turn.completed", "turn.assistant_completed"]` |
| Hermes `pre_llm_call` | `["llm.requested", "turn.user_prompt"]` |
| Hermes `transform_llm_output` | `["llm.completed", "turn.assistant_completed"]` |
| Pi `turn_end` | `["turn.completed", "turn.assistant_completed"]` |

Secondary rows share the same inbound event and payload as the primary row. They are for audit/query consistency and are not dispatched to live handlers; sync handler decisions and native responses are evaluated against the first configured event only.

## Codex source events

| Codex source event | Hitch event | Normalized payload | Native response behavior |
| --- | --- | --- | --- |
| `SessionStart` | `session.started` | Original Codex payload | `inject_context` adds `additionalContext`; `block` or `deny` returns `decision: "block"` with `reason`. |
| `SubagentStart` | `subagent.started` | Original Codex payload | `inject_context` adds `additionalContext`; `block` or `deny` returns `decision: "block"` with `reason`. |
| `UserPromptSubmit` | `turn.user_prompt` | Original Codex payload | `inject_context` adds `additionalContext`; `block` or `deny` returns `decision: "block"` with `reason`. |
| `PreToolUse` | `tool.requested` | Original Codex payload | `deny`, `block`, or `stop` returns `permissionDecision: "deny"` and `permissionDecisionReason`; `allow` or `transform` returns `permissionDecision: "allow"`; `transform` may include `updatedInput`. |
| `PermissionRequest` | `tool.permission_requested` | Original Codex payload | `deny`, `block`, or `stop` returns `hookSpecificOutput.decision.behavior: "deny"`; `allow` returns `hookSpecificOutput.decision.behavior: "allow"`. |
| `PostToolUse` | `tool.completed` | Original Codex payload | `inject_context` adds `additionalContext`; `block` or `deny` returns `decision: "block"` with `reason`. |
| `PreCompact` | `session.compacted` | Original Codex payload | `stop` or `block` returns `decision: "stop"` with `reason`. |
| `PostCompact` | `session.compacted` | Original Codex payload | `stop` or `block` returns `decision: "stop"` with `reason`. |
| `SubagentStop` | `subagent.completed` | Original Codex payload | `continue` returns `continue: true` with `reason`. |
| `Stop` | `turn.completed`, `turn.assistant_completed` | Original Codex payload | Primary `turn.completed` handles `stop` or `block` as `decision: "stop"` with `reason`; secondary `turn.assistant_completed` is audit-only. |

Codex handlers may also return `decision.native_response`. When present, Hitch returns that JSON directly instead of translating the normalized behavior.

## Hermes source events

| Hermes source event | Hitch event | Default | Normalized payload | Native response behavior |
| --- | --- | --- | --- | --- |
| `pre_tool_call` | `tool.requested` | Yes | Original Hermes payload | `block`, `deny`, or `stop` returns `action: "block"` and `message`. |
| `post_tool_call` | `tool.completed` | No | Original Hermes payload | Observer-only duplicate of `transform_tool_result` in observed runs; configure explicitly when that distinct callback is needed. Hitch returns `{}` unless `native_response` is supplied. |
| `pre_llm_call` | `llm.requested`, `turn.user_prompt` | Yes | Original Hermes payload | Primary `llm.requested` handles `inject_context` as `context`; secondary `turn.user_prompt` is audit-only. |
| `post_llm_call` | `llm.completed` | No | Original Hermes payload | Observer-only and potentially large; configure explicitly when raw post-call telemetry is needed. Hitch returns `{}` unless `native_response` is supplied. |
| `on_session_start` | `session.started` | Yes | Original Hermes payload | No special translation; Hitch returns `{}` unless `native_response` is supplied. |
| `on_session_end` | `session.ended` | Yes | Original Hermes payload | No special translation; Hitch returns `{}` unless `native_response` is supplied. |
| `subagent_stop` | `subagent.completed` | Yes | Original Hermes payload | No special translation; Hitch returns `{}` unless `native_response` is supplied. |
| `transform_tool_result` | `tool.completed` | Yes | Original Hermes payload | `replace_result` or `transform` returns `result` from `updated_output`. |
| `transform_terminal_output` | `tool.completed` | Yes | Original Hermes payload | `replace_result` or `transform` returns `result` from `updated_output`. |
| `transform_llm_output` | `llm.completed`, `turn.assistant_completed` | Yes | Original Hermes payload | Primary `llm.completed` handles `replace_result` or `transform` by returning `result` from `updated_output`; secondary `turn.assistant_completed` is audit-only. |
| `pre_gateway_dispatch` | `turn.user_prompt` | Yes | Original Hermes payload | `handled` returns `action: "skip"`; `transform` returns `action: "rewrite"` and `message` from `updated_input`; `allow` returns `action: "allow"`. |

Hermes handlers may also return `decision.native_response`. When present, Hitch returns that JSON directly.

## Pi source events

Pi uses a TypeScript adapter response contract:

```json
{
  "adapter_action": "noop"
}
```

`adapter_action` may be:

| Action | Adapter behavior |
| --- | --- |
| `noop` | Return nothing to Pi and leave the source event unchanged. |
| `return` | Return `return_value` to Pi. |
| `mutate_and_return` | Apply each `mutations[].path` to the source event, then return `return_value`. |

| Pi source event | Hitch event | Default | Normalized payload | Adapter response behavior |
| --- | --- | --- | --- | --- |
| `input` | `turn.user_prompt` | Yes | Original Pi payload | `transform` returns `{action:"transform", text:<updated_input>}`; `handled` returns `{action:"handled"}`; `continue` or `allow` returns `{action:"continue"}`. |
| `before_agent_start` | `turn.started` | No | Original Pi payload | Large prompt/system-prompt snapshot; configure explicitly when a handler needs to inspect or replace agent-start context. No special default translation. |
| `agent_start` | `turn.started` | No | Original Pi payload | Observer-only lifecycle marker; configure explicitly when start telemetry is needed. |
| `turn_start` | `turn.started` | Yes | Original Pi payload | No special translation; `adapter_action:"noop"`. |
| `context` | `turn.started` | No | Original Pi payload | Large conversation context snapshot; configure explicitly for context-rewrite handlers. `transform` returns `updated_input` as `return_value`. |
| `before_provider_request` | `llm.requested` | No | Original Pi payload | Large provider request snapshot; configure explicitly for provider-request policy or rewrite handlers. `transform` returns `updated_input` as `return_value`. |
| `tool_call` | `tool.requested` | Yes | Original Pi payload | `block`, `deny`, or `stop` returns `{block:true, reason}`; `transform` mutates the source event at path `input` to `updated_input`. |
| `tool_result` | `tool.completed` | Yes | Original Pi payload | `replace_result` or `transform` returns `updated_output` as `return_value`. |
| `turn_end` | `turn.completed`, `turn.assistant_completed` | Yes | Original Pi payload | Primary `turn.completed` has no special translation; secondary `turn.assistant_completed` is audit-only. |
| `agent_end` | `turn.completed` | No | Original Pi payload | Large final message snapshot; configure explicitly when full agent-end transcript capture is needed. |
| `session_start` | `session.started` | Yes | Original Pi payload | No special translation; `adapter_action:"noop"`. |
| `session_shutdown` | `session.ended` | Yes | Original Pi payload | No special translation; `adapter_action:"noop"`. |
| `session_before_switch` | `session.resumed` | Yes | Original Pi payload | `block` or `stop` returns `{cancel:true}`. |
| `session_before_fork` | `session.resumed` | Yes | Original Pi payload | `block` or `stop` returns `{cancel:true}`. |
| `session_before_compact` | `session.compacted` | Yes | Original Pi payload | `block` or `stop` returns `{cancel:true}`. |
| `session_compact` | `session.compacted` | Yes | Original Pi payload | No special translation; `adapter_action:"noop"`. |
| `user_bash` | `tool.requested` | Yes | Original Pi payload | No special translation; `adapter_action:"noop"`. |

Pi handlers may also return `decision.native_response`. When present, Hitch returns that adapter response JSON directly.

## OMP source events

OMP reuses the Pi adapter response contract for translated native responses.

| OMP source event | Hitch event | Default | Normalized payload | Adapter response behavior |
| --- | --- | --- | --- | --- |
| `input` | `turn.user_prompt` | Yes | Unwrapped OMP extension `event` payload when posted by the managed extension; bare payloads remain compatible. | Same translation as Pi `input`. |
| `before_agent_start` | `turn.started` | No | OMP payload | Large prompt/system-prompt snapshot; configure explicitly when agent-start context policy is needed. |
| `agent_start` | `turn.started` | No | OMP payload | Observer-only lifecycle marker. |
| `agent_end` | `turn.completed` | No | OMP payload | Large final message snapshot; configure explicitly when full agent-end transcript capture is needed. |
| `turn_start` | `turn.started` | Yes | OMP payload | No special translation; `adapter_action:"noop"`. |
| `turn_end` | `turn.completed` | Yes | OMP payload | No special translation; `adapter_action:"noop"`. |
| `context` | `turn.started` | No | OMP payload | Large conversation context snapshot; configure explicitly for context-rewrite handlers. `transform` returns `updated_input` as `return_value`. |
| `before_provider_request` | `llm.requested` | No | OMP payload | Large provider request snapshot; configure explicitly for provider-request policy or rewrite handlers. `transform` returns `updated_input` as `return_value`. |
| `after_provider_response` | `llm.completed` | No | OMP payload | Provider response observer; configure explicitly when raw provider-response telemetry is needed. |
| `message_start` | `turn.assistant_started` | No | OMP payload | Assistant-message start marker; configure explicitly when start timing is needed. |
| `message_update` | `turn.assistant_started` | No | OMP payload | Streaming assistant update; intentionally excluded by default because it is high-volume. |
| `message_end` | `turn.assistant_completed` | Yes | OMP payload | No special translation; `adapter_action:"noop"`. |
| `tool_call` | `tool.requested` | Yes | OMP payload | Uses Pi block/transform translation. |
| `tool_execution_start` | `tool.requested` | No | OMP payload | Observer-only execution start; configure explicitly when start timing is needed. |
| `tool_result` | `tool.completed` | Yes | OMP payload | Uses Pi result replacement translation. |
| `tool_execution_update` | `tool.progress` | No | OMP payload | Incremental tool output; configure explicitly for streaming/progress handlers. |
| `tool_execution_end` | `tool.completed` | No | OMP payload | Observer-only duplicate of `tool_result` in observed runs; configure explicitly when that distinct callback is needed. |
| `session_start` | `session.started` | Yes | OMP payload | No special translation; `adapter_action:"noop"`. |
| `session_before_switch`, `session_switch`, `session_before_branch`, `session_branch`, `session_before_tree`, `session_tree` | `session.resumed` | Yes | OMP payload | Cancelable pre-events return `{cancel:true}` for `block` or `stop`. |
| `session_before_compact`, `session.compacting`, `session_compact`, `auto_compaction_start`, `auto_compaction_end` | `session.compacted` | Yes | OMP payload | `session_before_compact` can return `{cancel:true}`; compaction customization uses `native_response`. |
| `session_shutdown` | `session.ended` | Yes | OMP payload | No special translation; `adapter_action:"noop"`. |
| `auto_retry_start` | `retry.started` | No | OMP payload | Retry lifecycle marker; configure explicitly for retry telemetry. |
| `auto_retry_end` | `retry.completed` | No | OMP payload | Retry lifecycle marker; configure explicitly for retry telemetry. |
| `ttsr_triggered` | `turn.started` | No | OMP payload | Product-specific observer event; configure explicitly if relevant. |
| `todo_reminder` | `turn.started` | No | OMP payload | Product-specific observer event; configure explicitly if relevant. |
| `goal_updated` | `turn.started` | No | OMP payload | Product-specific observer event; configure explicitly if relevant. |
| `credential_disabled` | `error.reported` | Yes | OMP payload | No special translation; `adapter_action:"noop"`. |
| `user_bash` | `tool.requested` | Yes | OMP payload | Observational unless `native_response` is supplied. |
| `user_python` | `tool.requested` | Yes | OMP payload | Observational unless `native_response` is supplied. |

OMP handlers may also return `decision.native_response`. When present, Hitch returns that adapter response JSON directly.

## Decision translation rules

1. Handler decisions are aggregated before harness translation. See [`handler-protocol.md`](handler-protocol.md) for precedence and timeout behavior.
2. `decision.native_response` is an escape hatch. It bypasses Hitch's behavior translation and returns harness-native JSON unchanged.
3. A translated empty object or `adapter_action:"noop"` means Hitch made no control-flow change.
4. Async observer dispatch uses `POST /v1/events` and ignores native responses. Sync control dispatch uses `POST /v1/dispatch-sync` and returns the translated native response.
