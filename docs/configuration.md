# Configuration

Hitch reads user configuration from `~/.config/hitch/config.toml`. Development commands default to `config/default.config.toml` unless `-config` is provided.

Key sections:

- `[server]`: local API bind address and request size limit.
- `[log]`: operational logging. Native payloads are not included by default.
- `[audit]`: event journal persistence. SQLite is the initial backend.
- `[handlers.<name>]`: external command handlers.
- `[harness.<name>]`: per-harness enable flags.

Handler commands receive the normalized Hitch event envelope as JSON on stdin and may return a Hitch handler result as JSON on stdout.
