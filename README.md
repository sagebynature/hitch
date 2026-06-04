# Hitch

Hitch is a local universal hook adapter for agent harnesses. It normalizes native hook payloads from Codex, Pi, OMP, and Hermes, dispatches configured external-command handlers, persists audit records, and translates synchronous decisions back into each harness's native hook response format.

Verified in this implementation:

- Local HTTP API: `GET /v1/health`, `POST /v1/events`, `POST /v1/dispatch-sync`, `GET /v1/events/<id>`.
- SQLite audit persistence for inbound events, normalized events, handler invocations, and native responses.
- Codex and Hermes shell adapter entrypoint via `hitch adapter`.
- Pi/OMP-compatible TypeScript adapter response application helpers.
- Installer placeholder management under `~/.config/hitch/integrations`.
- `inspect-event` and `replay` CLI support for persisted normalized events.

Run tests:

```sh
go test ./...
bun test adapters/**/*.test.ts
```

See:

- `docs/configuration.md`
- `docs/installation.md`
- `docs/handler-protocol.md`
- `docs/harness-contracts.md`
- `docs/replay.md`
- `docs/adr/0001-hitch-architecture-and-technology-stack.md`
