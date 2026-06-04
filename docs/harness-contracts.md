# Harness Contracts

Hitch supports two dispatch modes:

- Async observer events use `POST /v1/events`.
- Sync control events use `POST /v1/dispatch-sync` and return a native response.

Initial harness contracts:

- Codex: maps native command hook payloads and translates decisions to Codex stdout JSON.
- Hermes: maps shell hook payloads and translates decisions to Hermes stdout JSON.
- Pi/OMP: maps extension callback events and returns adapter instructions for callback return values or event mutation.

Unsupported native event types are rejected unless a future passthrough mode is configured.
