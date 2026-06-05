# Hitch

Write one hook event handler for all your agent harnesses.

Hitch is a local universal hook adapter for Codex, Pi, OMP, Hermes, and OpenCode. It gives open-source developers a single, portable handler protocol instead of forcing every tool, guardrail, logger, or policy script to learn each harness's native hook format.

## Why Hitch?

Agent harnesses are moving fast, and each one ships its own hook payloads, event names, and response shapes. That fragmentation makes useful developer tools harder to share.

Hitch normalizes those harness-specific hooks into one event envelope, runs your configured handlers, records what happened, and translates synchronous decisions back into the native response format the harness expects.

Use Hitch to build:

- shared guardrails for tool calls and command execution
- audit logs for local agent activity
- policy checks that work across multiple harnesses
- context injection and result transformation handlers
- reusable open-source hook packages that are not locked to one agent CLI

## What it provides

- **Universal event envelope** for native Codex, Pi, OMP, Hermes, and OpenCode hook payloads.
- **External-command handler protocol**: handlers read normalized JSON from stdin and return JSON decisions on stdout.
- **Synchronous decisions** translated back to each harness's native response format.
- **SQLite audit trail** for inbound events, normalized events, handler invocations, and native responses.
- **Local HTTP API** for event ingestion, synchronous dispatch, health checks, event inspection, and replay.
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
make test              # Run Go and adapter tests
make build             # Build bin/hitch
make serve             # Run the local Hitch server
make install-dry-run   # Preview hook installation
```

Run tests directly:

```sh
go test ./...
bun test adapters/**/*.test.ts
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

The source installer creates missing Hitch user config with `hitch config init`, prompts for a Hitch server URL through `/dev/tty` even when installed with `curl ... | sh`, and can persist `HITCH_URL` for remote servers. `hitch-client install` installs supported Codex and Hermes hooks, installs managed Pi and OMP extensions, installs a managed OpenCode plugin, and prefers `hitch-client` in managed shell-hook commands when it is installed beside `hitch`. Generated hooks resolve the server URL at runtime unless `--url` pins one.

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