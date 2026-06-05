# Hitch

Control every agent lifecycle from one event layer.

Hitch turns Codex, Hermes, Pi, OMP, and OpenCode hooks into one portable event protocol. Write a hook once, run it across harnesses, preserve the native payload, and route lifecycle events to local or remote handlers.

## Why Hitch?

AI coding agents are moving fast. Each harness has its own lifecycle events, payload shapes, hook configuration, response rules, and execution model. That fragmentation makes useful guardrails, loggers, policy checks, context injectors, and workflow automations harder to build and harder to share.

Hitch normalizes those harness-specific hooks into one event envelope while retaining the original source payload. It then routes the normalized event to your configured handlers, records what happened, and translates synchronous decisions back into the native response format each harness expects.

Use Hitch when you want to:

- write one policy, logger, or workflow hook and reuse it across multiple agent harnesses
- move between agent tools without rebuilding your automation for every hook format
- preserve native harness payloads while relying on stable normalized lifecycle fields
- expose local agent lifecycle events over a REST API for inspection, replay, or remote dispatch
- deploy Hitch as a service so teams can centralize hook routing, execution, audit, and governance

## For solo developers

Stop rewriting hooks for every agent CLI.

A single Hitch handler can watch tool calls, block risky commands, inject context, transform results, or append audit logs across supported harnesses. Handlers read one JSON envelope on stdin and return one JSON result on stdout, so you can write them in shell, Python, Node, Go, or any language that can run as a command.

## For teams

Let developers choose their harness. Keep lifecycle control centralized.

Your team may use Codex, Hermes, Pi, OMP, OpenCode, or a new harness next month. Hitch lets those tools emit lifecycle events into one REST API, locally or remotely, so platform, security, and developer-experience teams can manage routing and handler execution from one place without forcing every developer onto the same agent CLI.

## What it provides

- **Universal lifecycle event envelope** for native Codex, Hermes, Pi, OMP, and OpenCode hook payloads.
- **Original payload preservation** through `source_payload`, so handlers can inspect harness-specific detail when needed.
- **External-command handler protocol**: handlers read normalized JSON from stdin and return JSON decisions on stdout.
- **Synchronous decisions** translated back to each harness's native response format.
- **Local-first, remote-ready REST API** for event ingestion, synchronous dispatch, health checks, event inspection, and replay.
- **Central hook routing and execution** through server-side handler config.
- **SQLite audit trail** for inbound events, normalized events, handler invocations, and native responses.
- **hitch-client hook shim** for shell-based harness integrations.

Verified API endpoints:

- `GET /v1/health`
- `POST /v1/events`
- `POST /v1/dispatch-sync`
- `GET /v1/events/<id>`

## Quick start

Install from latest source:

```sh
curl -fsSL https://raw.githubusercontent.com/sagebynature/hitch/main/scripts/install.sh | sh
```

Or build the CLI locally:

```sh
make build
```

Run Hitch locally:

```sh
./bin/hitch serve --config internal/config/default.config.toml
```

In another shell, check the installation:

```sh
./bin/hitch doctor --json
./bin/hitch status --json
```

The default server listens on `127.0.0.1:8799` and stores audit records at `~/.local/share/hitch/events.sqlite`.

## Handler model

A Hitch handler is any executable command. It receives a normalized Hitch event envelope on stdin:

```json
{
  "hitch_version": "0.1.0",
  "event_id": "evt_...",
  "received_at": "2026-06-04T10:47:00Z",
  "harness": "codex",
  "source_event_type": "PreToolUse",
  "source_payload": {},
  "hitch_event_type": "tool.requested",
  "payload": {}
}
```

It can return a handler result on stdout:

```json
{
  "status": "ok",
  "decision": {
    "behavior": "deny",
    "reason": "This command is blocked by local policy."
  }
}
```

That same handler can be reused across every supported harness.

See `docs/handler-protocol.md` for the full handler result contract.

## Common commands

```sh
make test              # Run Go tests
make build             # Build bin/hitch
make serve             # Run the local Hitch server
make install-dry-run   # Preview hook installation
```

Run tests directly:

```sh
go test ./...
```

## Harness integration status

Hitch currently includes:

- Codex shell hook installation into `~/.codex/hooks.json`.
- Codex and Hermes shell hook shim entrypoints via `hitch-client`.
- Pi TypeScript extension hook installation into `~/.pi/agent/extensions/hitch/index.ts`.
- OMP TypeScript extension hook installation into `~/.omp/agent/extensions/hitch/index.ts`.
- OpenCode TypeScript plugin installation into `~/.config/opencode/plugins/hitch.ts`.
- Server config seeding via `hitch config init` for `~/.config/hitch/config.toml`.
- Harness detection for Codex, Hermes, Pi, OMP, and OpenCode.

The source installer creates missing Hitch user config with `hitch config init`, adds missing managed default sections to existing valid config, prompts for a Hitch server URL through `/dev/tty` even when installed with `curl ... | sh`, and can persist `HITCH_URL` for remote servers. `hitch-client install` installs supported Codex and Hermes hooks, installs managed Pi and OMP extensions, installs a managed OpenCode plugin, and prefers `hitch-client` in managed shell-hook commands when it is installed beside `hitch`. Generated hooks resolve the server URL at runtime unless `--url` pins one.

Preview hook installation:

```sh
./bin/hitch-client install --dry-run --json
```

Run the hook client directly from a harness hook configuration:

```sh
./bin/hitch-client -harness codex -event PreToolUse -sync
./bin/hitch-client -harness hermes -event pre_tool_call -sync
```

## Configuration

Hitch reads user configuration from `~/.config/hitch/config.toml` by default. Repository checkout examples pass `--config internal/config/default.config.toml` explicitly when they should use the development config.

Create the default user config without overwriting an existing file:

```sh
hitch config init
```

Configuration covers:

- server host, port, and request size limits
- logging sinks and payload visibility
- SQLite audit persistence
- handler commands, event filters, modes, timeouts, and error policy
- per-harness enable flags and source-event mappings

See `docs/configuration.md` for details.

## Documentation

- `docs/walkthrough.md`
- `docs/configuration.md`
- `docs/installation.md`
- `docs/handler-protocol.md`
- `docs/harness-contracts.md`
- `docs/replay.md`
- `docs/adr/0001-hitch-architecture-and-technology-stack.md`

## Contributing

Hitch is for developers building the next layer of agent tooling: guardrails, observability, local policy, replay, and workflow automation.

Contributions are especially useful in:

- additional harness adapters
- real installer support for harness configuration files
- handler examples and reusable policy packs
- event replay and audit tooling
- protocol compatibility tests