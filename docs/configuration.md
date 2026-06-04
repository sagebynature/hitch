# Configuration

Hitch reads user configuration from `~/.config/hitch/config.toml`. Development commands default to `config/default.config.toml` unless `--config` or `-config` is provided by the command.

Key sections:

- `[server]`: local API bind address and request size limit.
- `[log]`: operational logging. Native payloads are not included by default.
- `[audit]`: event journal persistence. SQLite is the verified backend.
- `[handlers.<name>]`: external command handlers.
- `[harness.<name>]`: per-harness enable flags.

Strict validation rejects unknown config keys and invalid values for:

- server port
- request size limit
- log level and format
- audit backend
- handler event names
- handler mode (`async` or `sync`)
- handler error/timeout policy (`fail_open`, `fail_closed`, `native_default`)

Handler commands receive the normalized Hitch event envelope as JSON on stdin and may return a Hitch handler result as JSON on stdout.

Paths beginning with `~/` are expanded where config allows path values.
