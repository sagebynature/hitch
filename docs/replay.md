# Replay and Inspect

Inspect a normalized event by id:

```sh
hitch inspect-event --config internal/config/default.config.toml <normalized-event-id>
```

`inspect-event` returns the audit view for the normalized event:

- inbound event record
- normalized event record
- handler invocation records
- native response records

Dry-run replay returns the stored event envelope without writing records:

```sh
hitch replay --config internal/config/default.config.toml --dry-run <normalized-event-id>
```

Replay without `--dry-run` re-runs configured sync handlers for the stored normalized event and persists new handler invocation records linked by `replay_source_id` to the source normalized event id.
