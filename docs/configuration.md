# Configuration

Hitch reads user configuration from `~/.config/hitch/config.toml` by default. Repository checkout examples pass `--config internal/config/default.config.toml` explicitly when they should use the development config.

Create the default user config, or add missing managed default sections to an existing valid config without overwriting user-owned values:

```sh
hitch config init
```

Key sections:

- `[server]`: local API bind address and request size limit.
- `[log]`: operational logging. Native payloads are not included by default. `log.level` and `log.format` are fallbacks for enabled stdout and rolling file sinks; `log.stdout.level`, `log.stdout.format`, `log.file.level`, and `log.file.format` may override them per sink. `[log.otlp].enabled = true` is rejected until OTLP export is implemented.
- `[audit]`: event journal persistence. SQLite is the verified backend. Enabled configs with `audit.backend = "jsonl"` are rejected until the JSONL backend is implemented.
- `[handlers.<name>]`: external command handlers.
- `[harness.<name>]`: per-harness enable flags and source-event mappings.

Strict validation rejects unknown config keys and invalid values for:

- server port
- request size limit
- log level and format (`debug`, `info`, `warn`, `error`; `json`, `console`)
- audit backend
- handler event names
- handler kind (`observer` or `control`)
- handler error/timeout policy (`fail_open`, `fail_closed`, `native_default`)
- harness event-map values


Supported today:

- audit backend: `sqlite`
- log sinks: stdout and rolling file

Rejected until implemented:

- enabled `audit.backend = "jsonl"` configs
- `[log.otlp].enabled = true`

Handler commands receive the normalized Hitch event envelope as JSON on stdin and may return a Hitch handler result as JSON on stdout.

`handlers.<name>.working_dir` is optional. When set through a loaded config file, relative values resolve against the directory containing that config file. Hitch runs the handler command from that directory, so relative command arguments and output paths are stable regardless of where `hitch serve` was launched.

Harness event maps live in config. The default config includes the recommended low-noise mappings. Duplicate, high-volume, or product-specific source events are documented in `docs/events.md` as opt-in catalog rows; add entries in the relevant map to capture them:

```toml
[harness.omp.event_map]
before_provider_request = "llm.requested"
tool_execution_update = "tool.progress"
turn_end = ["turn.completed", "turn.assistant_completed"]
```

```toml
[harness.opencode.event_map]
"chat.params" = "llm.requested"
"command.executed" = "turn.user_prompt"
```

Keys are source hook/callback names. Values are normalized Hitch event names or ordered lists of normalized Hitch event names. The first event is primary for sync dispatch/native responses; additional events are secondary audit rows. Unknown source events are rejected unless configured here.


Paths beginning with `~/` are expanded where config allows path values.
