# Configuration

Hitch reads user configuration from `~/.config/hitch/config.toml` by default. Repository checkout examples pass `--config internal/config/default.config.toml` explicitly when they should use the development config.

Create the default user config when it does not already exist:

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

Handlers support two invocation types: `shell` and `native`. Existing command handlers default to `type = "shell"` and may keep using `events = ["tool.requested"]`; new configs should use `hitch_events = ["tool.requested"]`.

Shell handlers receive a Hitch invocation context JSON object on stdin and the selected primary payload as one compact JSON command-line argument. The context keeps legacy top-level event fields for existing handlers and also includes a nested `event` object with both `source_payload` and Hitch `payload`.

`source_events` narrows a handler to exact source hook pairs:

```toml
source_events = [{ harness = "codex", source_event_type = "PreToolUse" }]
```

`payload = "hitch"` passes Hitch's normalized payload as the primary payload. `payload = "source"` passes the preserved source payload as the primary payload. Both payloads are always available in the invocation context.

`handlers.<name>.working_dir` is optional. When set through a loaded main config file, relative values resolve against the directory containing that config file. Hitch runs the handler command from that directory, so relative command arguments and output paths are stable regardless of where `hitch serve` was launched.

Extension-managed handlers are discovered under `~/.config/hitch/extensions/<directory>/config.toml`. Extension configs use the same routing fields. Native extension configs omit `type` or set `type = "native"` and provide `entrypoint = "module:function"`; shell extension configs set `type = "shell"` and provide `command = [...]`. Discovered extensions run from their extension directory by default, so `command = ["/bin/sh", "adapter.sh"]` looks for `adapter.sh` in that extension folder. Relative extension `working_dir` values resolve from the extension directory. Use `/bin/bash` only when the container image or local runtime is known to provide Bash.

Harness event maps live in config. The default config includes the recommended low-noise mappings. Duplicate, high-volume, or product-specific source events are documented in `docs/events.md` as opt-in catalog rows; add entries in the relevant map to capture them:

```toml
[harness.omp.event_map]
before_provider_request = "llm.requested"
tool_execution_update = "tool.progress"
turn_end = ["turn.completed", "turn.assistant_completed", "llm.completed"]
```

```toml
[harness.opencode.event_map]
"chat.params" = "llm.requested"
"command.executed" = "turn.user_prompt"
```

Keys are source hook/callback names. Values are normalized Hitch event names or ordered lists of normalized Hitch event names. The first event is primary for sync dispatch/native responses; additional events are secondary audit rows. Unknown source events are rejected unless configured here.

Legacy default maps from older Hitch configs are upgraded in memory at load time for known completion events such as Pi/OMP `turn_end` and OpenCode `message.part.*`; restart `hitch serve` after installing a newer build so the loaded runtime map reflects those upgrades.


Paths beginning with `~/` are expanded where config allows path values.
