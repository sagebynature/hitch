# ADR-0001: Hitch Hook Adapter Architecture and Technology Stack

## Status

Proposed

## Date

2026-06-04

## Context

Hitch is a spin-off from Agent Pulse / sentiment-viewer. Agent Pulse proved that local agent harnesses can emit useful lifecycle information, but its event model was intentionally narrow: classify user intent, user reaction, and agent action for a desktop status panel.

Hitch has a broader goal: provide a universal hook adapter for agent harnesses, initially Codex, Claude Code, Pi, Oh My Pi (OMP), Hermes, OpenCode, and Antigravity. Source harnesses have different hook mechanisms, event names, payload schemas, return contracts, trust models, and installation surfaces. Hitch must normalize those into a homogeneous event protocol while preserving the original native payload as JSON.

The key architectural complication is that harness hooks are not all fire-and-forget. Some are observational, while others are control points that expect a synchronous decision, transformed input, injected context, or rewritten output. Hitch therefore needs to handle both asynchronous event ingestion and synchronous native-response translation.

## Decision Drivers

- Preserve native harness payloads exactly enough for audit, replay, and future remapping.
- Provide a stable normalized event envelope for portable handlers.
- Support one native event mapping to many Hitch handlers.
- Execute independent handlers concurrently without nondeterministic final behavior.
- Support synchronous control hooks where the source harness expects a return value.
- Fail open for observability paths so Hitch downtime does not break normal agent work.
- Allow configurable fail-open or fail-closed behavior for security/control hooks.
- Avoid inventing logging infrastructure; use established logging and telemetry frameworks.
- Persist inbound events, normalized events, handler outputs, and emitted native responses.
- Keep the daemon simple, local-first, auditable, and safe by default.

## Harness Return Contract Findings

### Codex

Codex command hooks receive JSON on stdin and may return JSON or text on stdout, depending on event type.

Return-capable events include:

- `SessionStart`: may add developer context.
- `SubagentStart`: may add subagent context.
- `UserPromptSubmit`: may add context or block the prompt.
- `PreToolUse`: may deny a tool, add context, or rewrite supported tool input.
- `PermissionRequest`: may allow or deny an approval request.
- `PostToolUse`: may add context or block/replace model-visible tool result, but cannot undo side effects.
- `PreCompact` / `PostCompact`: may stop compaction.
- `SubagentStop`: expects JSON on stdout on success and may request continuation.
- `Stop`: supports common JSON control fields.

Codex already defines useful aggregation semantics for some events, such as deny winning over allow for `PermissionRequest`.

### Hermes

Hermes has gateway hooks, plugin hooks, and shell hooks. Shell hooks receive JSON on stdin and read JSON from stdout. Plugin hooks return Python values directly.

Return-capable events include:

- `pre_tool_call`: may block a tool.
- `pre_llm_call`: may inject context.
- `transform_tool_result`: may replace result text.
- `transform_terminal_output`: may replace raw terminal output.
- `transform_llm_output`: may replace the final response.
- `pre_gateway_dispatch`: may skip, rewrite, or allow dispatch.

Most other Hermes hooks are observers whose return values are ignored.

### Pi / OMP

Pi and OMP use an extension callback model. Handlers return JavaScript objects or mutate event objects directly; they do not write shell-style stdout responses.

Return-capable events include:

- `input`: may continue, transform input, or handle input without running the agent.
- `tool_call`: may block a tool; tool input changes are made by mutating `event.input` in place.
- `tool_result`: may patch `content`, `details`, or `isError`.
- `context`: may modify messages.
- `before_provider_request`: may replace the provider request payload.
- `user_bash`: may replace bash operations or return a result directly.
- session pre-events: may cancel or customize some session operations.

OMP should be treated as Pi-compatible until its extensions diverge in tested behavior.

## Decision

Hitch will be built as a local daemon exposing both asynchronous and synchronous REST endpoints. `hitch-client` is the lightweight harness hook shim: it reads source JSON from stdin, posts to the local Hitch HTTP API, and writes only the native sync response to stdout.

Hitch will use a stable normalized envelope and preserve the source-native payload as JSON.

Hitch will distinguish two execution modes:

1. **Async observer mode** for lifecycle and telemetry events whose native return value is ignored or optional.
2. **Sync control mode** for hooks where the harness expects or accepts a decision, transformation, context injection, or result replacement.

Handler execution will be concurrent where safe, but final control decisions will be aggregated deterministically by Hitch, not by completion race.

Hitch will separate operational logging from event persistence:

- Operational logs use a standard logging framework and OpenTelemetry-compatible export path.
- Event persistence stores inbound events, normalized events, handler invocations, handler outputs, and native responses in SQLite initially, with optional JSONL audit output.


## Hook Client Boundary Decision

Hitch will keep hook dispatch out of the daemon binary. `hitch-client` is the official harness-facing shim for command-hook harnesses.

The client is not the policy engine. Its role is transport and source-boundary forwarding:

```text
agent harness lifecycle hook
  -> executes hitch-client ...
  -> hitch-client reads source JSON from stdin
  -> hitch-client sends the source event to the local Hitch HTTP API
  -> Hitch normalizes the event and runs handlers
  -> Hitch translates the aggregate decision to harness-native JSON
  -> hitch-client prints only the native JSON response to stdout
  -> harness consumes stdout and continues, blocks, rewrites, or no-ops
```

This keeps handlers portable. Handlers depend on the normalized Hitch envelope and result protocol, not on each harness hook runner's stdin/stdout details.

Decision:

- Remove `hitch adapter` from the daemon command surface.
- Use `hitch-client` for command-hook harnesses such as Codex, Claude Code, Hermes, and Antigravity.
- Prefer direct HTTP integration only for harnesses that natively support synchronous HTTP hooks with reliable timeout and response semantics.
- Treat external harness-specific adapters as thin, tested compatibility layers, not as places for policy logic.
- Resolve installed hook server URLs through an explicit waterfall: pinned `--url`, `HITCH_URL`, user config, then the local default.

Rationale:

Keeping the hook shim in `hitch-client` gives users one dedicated dispatch binary without duplicating the same behavior in `hitch`. It also keeps the daemon CLI focused on serving, inspection, replay, and diagnostics.

## Normalized Event Envelope

Every inbound hook should be represented internally as:

```ts
type HitchEventEnvelope = {
  hitch_version: string
  event_id: string
  received_at: string

  harness: "codex" | "pi" | "omp" | "hermes" | "opencode" | "antigravity"
  source_event_type: string
  source_payload: unknown

  hitch_event_type: string
  session_id?: string
  turn_id?: string
  cwd?: string
  model?: string
  transcript_path?: string

  payload: unknown
}
```

Initial normalized event taxonomy:

```text
session.started
session.resumed
session.ended
session.compacted

turn.started
turn.user_prompt
turn.assistant_started
turn.completed

tool.requested
tool.permission_requested
tool.completed

subagent.started
subagent.completed

error.reported
```

Semantic classifications such as intent, sentiment, or trust trend are out of scope for the base protocol. They should be implemented as handlers/plugins.

## Handler Output Model

Hitch handlers return a normalized result:

```ts
type HitchHandlerResult = {
  status: "ok" | "error" | "timeout"

  decision?: {
    behavior:
      | "none"
      | "allow"
      | "deny"
      | "block"
      | "continue"
      | "stop"
      | "transform"
      | "replace_result"
      | "inject_context"
      | "handled"

    reason?: string
    context?: string
    updated_input?: unknown
    updated_output?: unknown
    native_response?: unknown
  }

  logs?: HitchLogRecord[]
  metrics?: Record<string, number>
}
```

Each harness mapper owns native translation, while the hook shim owns only the transport/native boundary:

```text
stdin source payload -> hitch-client -> Hitch HTTP API -> stdout native response
HitchHandlerResult -> Codex stdout JSON
HitchHandlerResult -> Hermes stdout JSON
HitchHandlerResult -> Pi/OMP callback return object or event mutation
```

The client shim must not run handlers, open the audit database, initialize server logging, or make policy decisions. `native_response` exists as an escape hatch, not as the normal portable API.

## Deterministic Aggregation Rules

For sync control events, Hitch will aggregate handler results in configured handler order, not completion order.

Decision precedence:

```text
deny/block/stop > handled > transform/replace_result > inject_context > allow > none
```

Rules:

- Each handler has a hard timeout.
- Each sync dispatch has a hard total deadline.
- Handler crash is recorded as `status = "error"` and ignored for decision aggregation unless the event is configured fail-closed.
- Timeout is recorded as `status = "timeout"` and handled according to event policy.
- Multiple context injections are concatenated deterministically in configured handler order.
- Multiple transforms are disallowed by default unless a handler chain is explicitly configured.
- Async observers must not affect native control flow.

## REST API Shape

Initial API:

```text
POST /v1/events
POST /v1/dispatch-sync
GET  /v1/health
GET  /v1/events/:id
POST /v1/replay
```

`/v1/events` accepts observer events and returns quickly after validation and durable enqueue/persist.

`/v1/dispatch-sync` accepts control events and returns a normalized Hitch decision that native adapters translate into harness-specific response formats.

## Persistence Model

Initial backend: SQLite.

Tables:

```text
inbound_events
  id
  received_at
  harness
  source_event_type
  source_payload
  request_headers
  hitch_client_version

normalized_events
  id
  hitch_version
  event_id
  received_at
  harness
  source_event_type
  source_payload
  hitch_event_type
  session_id
  turn_id
  cwd
  model
  transcript_path
  payload
  inbound_event_id
  mapping_version

handler_invocations
  id
  normalized_event_id
  handler_name
  mode
  started_at
  completed_at
  status
  stdout
  stderr
  output_json
  decision_json
  error

native_responses
  id
  normalized_event_id
  response_json
  emitted_at
```

Persistence is append-first. Mutating derived state can be added later, but the audit trail should retain the original inbound and normalized records.

## Logging Decision

Hitch will not implement custom log rolling, remote shipping, retry buffering, or vendor-specific logging integrations.

Hitch operational logs will use the standard logging stack for the selected implementation language and expose OpenTelemetry-compatible export. Remote logging should be handled by the OpenTelemetry Collector or existing log infrastructure.

Logging configuration should support:

```toml
[log]
level = "info"
format = "json" # json | console
include_native_payload = false

[log.stdout]
enabled = true

[log.file]
enabled = true
path = "~/.local/state/hitch/hitch.log"
max_size_mb = 100
max_backups = 10
max_age_days = 14
compress = true

[log.otlp]
enabled = false
endpoint = "http://127.0.0.1:4318"
protocol = "http/protobuf"
headers = {}

[audit]
enabled = true
backend = "sqlite,jsonl"

[audit.sqlite]
path = "~/.local/share/hitch/events.sqlite"

[audit.jsonl]
path = "~/.local/state/hitch/events.jsonl"
```

Operational logs are not the event journal. Event journal records go to SQLite and optional JSONL audit output.

## Technology Stack Options

### Option 1: Go

Representative libraries:

- HTTP: standard `net/http`, Chi, or Echo.
- SQLite: `modernc.org/sqlite` for pure Go or `mattn/go-sqlite3` for CGO.
- Logging: `slog` or `zap`.
- Rolling file writer: `lumberjack.v2`.
- Telemetry: OpenTelemetry Go SDK/exporters.
- Config: BurntSushi TOML or `pelletier/go-toml`.

Pros:

- Excellent fit for a long-running local daemon.
- Small static binaries are straightforward, especially with pure-Go SQLite.
- Strong standard library for HTTP, subprocesses, timeouts, signals, and concurrency.
- Goroutines make parallel handler execution simple without bringing in a runtime server framework.
- Easy cross-platform distribution.
- Mature structured logging and OpenTelemetry ecosystem.
- Operational behavior is predictable and boring.
- Lower implementation complexity than Rust for this problem.

Cons:

- Type system is less expressive than Rust for protocol modeling.
- Error handling is verbose.
- Pure-Go SQLite may lag CGO SQLite in some edge capabilities/performance.
- Plugin story is weaker than Node/Python; external process handlers are preferred.

Assessment:

Go is the recommended implementation language for the Hitch daemon and installers if the priority is reliable local service behavior, simple deployment, good concurrency, and low operational surprise.

### Option 2: Rust

Representative libraries:

- HTTP: Axum or Actix Web.
- SQLite: `sqlx`, `rusqlite`, or SeaORM.
- Logging/tracing: `tracing`, `tracing-subscriber`, `tracing-appender`.
- Telemetry: `opentelemetry` crates.
- Config: `config`, `toml`, `serde`.

Pros:

- Best compile-time modeling of protocol envelopes, handler results, and native response translators.
- Excellent performance and memory safety.
- Strong fit for a security-sensitive local daemon.
- Good single-binary distribution story.
- Strong async ecosystem with Tokio.

Cons:

- Higher implementation cost and maintenance burden.
- Async subprocess orchestration, cancellation, SQLite, and HTTP are all reliable but require more careful design.
- Faster to over-engineer with types and traits.
- Fewer contributors are comfortable modifying Rust than Go/TypeScript/Python in many teams.
- Some OpenTelemetry/logging integrations are less frictionless than Go’s.

Assessment:

Rust is technically excellent but likely too expensive for the first implementation unless Hitch’s security boundary becomes the dominant requirement. It is the best choice if we later need sandboxed in-process policy engines, WASM integrations, or very strict memory/resource guarantees.

### Option 3: TypeScript / Node.js

Representative libraries:

- HTTP: Fastify or Hono.
- SQLite: `better-sqlite3`, `sqlite`, or Drizzle.
- Logging: Pino.
- Rotation/transport: Pino transports or external logrotate/Collector.
- Telemetry: OpenTelemetry JS.
- Config: `@iarna/toml`, `zod` validation.

Pros:

- Natural fit for existing harness adapters, many of which are already JS/TS.
- Fastest iteration on adapter scripts and config installers.
- Strong JSON ergonomics.
- Strong npm ecosystem for CLI tooling.
- Easy to write user handler SDKs in the same language.
- Pi/OMP extension model is already TypeScript-friendly.

Cons:

- Requires Node runtime unless bundled with a tool like pkg/nexe/Bun, which adds distribution complexity.
- Long-running local daemon has more dependency and supply-chain surface.
- OpenTelemetry JS logs remain less mature than Go/Java/.NET per OTel status.
- Worker/thread/process timeout semantics need care.
- Single-file binary distribution is weaker than Go/Rust.

Assessment:

TypeScript is excellent for native harness adapters and SDKs. It is less ideal for the core daemon if we want Hitch to feel like infrastructure rather than an npm app. A hybrid model is attractive: Go daemon, TypeScript adapters/SDK initially where harnesses require JS.

### Option 4: Python

Representative libraries:

- HTTP: FastAPI, Starlette, or aiohttp.
- SQLite: standard `sqlite3`, `aiosqlite`, SQLAlchemy.
- Logging: standard `logging`, structlog, loguru.
- Rotation: `RotatingFileHandler`, `TimedRotatingFileHandler`, external Collector/logrotate.
- Telemetry: OpenTelemetry Python.
- Config: `tomllib` plus Pydantic.

Pros:

- Very fast to prototype.
- Strong scripting ergonomics for adapters and handlers.
- Good standard library for SQLite and logging.
- Natural fit for LLM/policy handler experiments.
- Easy for users to write custom handlers.

Cons:

- Packaging a reliable cross-platform daemon is more cumbersome.
- Runtime dependency management is more fragile than Go/Rust binaries.
- Async subprocess cancellation and parallel execution are workable but easier to get subtly wrong.
- Performance is sufficient for Hitch, but tail-latency predictability under many concurrent handlers is weaker.
- Larger operational surface for users who just want a local service.

Assessment:

Python is good for handler authoring and prototypes, but not the best core daemon language for a local infrastructure tool distributed to users across machines.

## Technology Stack Decision

Use **Go for the Hitch daemon, `hitch-client` hook shim, persistence layer, installer/doctor CLI, native response aggregation, and operational logging**.

Use **TypeScript for harness adapters where the harness integration surface is JavaScript/TypeScript-native**, especially Pi and OMP extensions and the OpenCode plugin. Codex, Claude Code, Hermes, and Antigravity shell-hook adapters should use the shipped `hitch-client` binary.

Support user handlers as external commands first. This avoids committing to an in-process plugin ABI and lets handlers be written in any language.

## Rationale

Hitch is fundamentally a local control-plane daemon. It needs to be easy to install, reliable under timeout pressure, strict about request size and schemas, and boring to operate. Go fits that shape best.

Rust provides stronger guarantees but increases implementation complexity before the protocol has settled. TypeScript gives the fastest adapter development but makes the daemon feel more fragile and runtime-dependent. Python gives excellent handler ergonomics but is the weakest packaging and daemon choice.

A Go core with external command handlers and TypeScript harness adapters gives the best balance:

- Boring daemon.
- Strong local distribution story.
- Good concurrency and timeout handling.
- Existing logging/telemetry ecosystem.
- Freedom for users to write handlers in any language.
- Natural integration with JS-based harness extension systems.

## Consequences

### Positive

- Hitch can be distributed as small local binaries: `hitch` for the daemon/CLI and `hitch-client` for harness hook execution.
- Operational logging can use established Go logging and OpenTelemetry libraries.
- SQLite persistence and handler dispatch remain in the server process, not the hook client.
- Native adapter shims remain thin and harness-specific.
- User handlers remain language-agnostic.

### Negative

- The project will contain at least two implementation languages: Go core plus TypeScript adapters.
- Shared protocol definitions must be generated or duplicated carefully across Go and TypeScript.
- Users wanting in-process plugin APIs will wait until the external command handler contract stabilizes.
- Go’s type system is less precise than Rust for representing complex native response variants.

### Risks and Mitigations

- **Risk: Protocol drift between Go and TypeScript.**
  - Mitigation: define JSON schemas for event envelopes and handler results; generate or validate fixtures for both languages.

- **Risk: Sync handlers exceed harness deadlines.**
  - Mitigation: configure per-handler and total dispatch deadlines; record timeouts; deterministic fail-open/fail-closed policy.

- **Risk: Native payloads contain secrets.**
  - Mitigation: default `include_native_payload = false` for operational logs; persist native payloads only in audit store; add redaction hooks before durable persistence.

- **Risk: Multiple transforms conflict.**
  - Mitigation: disallow multiple transforms by default; require explicit ordered transform chains.

- **Risk: Hitch outage blocks harness work.**
  - Mitigation: native adapters fail open for async events; control events use configured fallback behavior.

## Implementation Notes

Recommended initial Go packages:

- HTTP: standard `net/http` initially; add router only if route complexity warrants it.
- SQLite: start with `modernc.org/sqlite` for pure-Go binary distribution; revisit CGO if performance or SQLite extension requirements demand it.
- Logging: `slog` for standard-library alignment or `zap` if performance/field ergonomics justify it.
- Rolling file: `lumberjack.v2` as an `io.Writer`; do not implement rotation manually.
- Telemetry: OpenTelemetry Go SDK with OTLP exporter.
- Config: strict TOML parser plus explicit validation.

Required test fixtures:

- Native Codex payload -> Hitch envelope.
- Native Hermes payload -> Hitch envelope.
- Native Pi/OMP/OpenCode event -> Hitch envelope.
- Hitch decision -> Codex stdout JSON.
- Hitch decision -> Hermes stdout JSON.
- Hitch decision -> Pi/OMP return object or event mutation.
- Hitch decision -> OpenCode plugin adapter action.
- Conflicting handler decisions aggregate deterministically.
- Handler timeout obeys configured fallback policy.
- LLM-backed handler recursion guard prevents re-entry.

## Related Decisions

Future ADRs should cover:

- Handler process protocol and SDK shape.
- SQLite schema and retention policy.
- Installer and managed integration strategy.
- Security/redaction model.
- OpenTelemetry resource naming and deployment guidance.

## References

- OpenAI Codex Hooks: https://developers.openai.com/codex/hooks
- Hermes Event Hooks: https://hermes-agent.nousresearch.com/docs/user-guide/features/hooks/
- Pi Extensions: https://pi.dev/docs/latest/extensions
- OpenTelemetry Logs: https://opentelemetry.io/docs/concepts/signals/logs/
- OpenTelemetry Collector: https://opentelemetry.io/docs/collector/
- Lumberjack rolling file writer: https://github.com/natefinch/lumberjack
