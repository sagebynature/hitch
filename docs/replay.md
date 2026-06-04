# Replay and Inspect

Inspect a normalized event by id:

```sh
hitch inspect-event <normalized-event-id> --config config/default.config.toml
```

Dry-run replay returns the stored event envelope without writing records:

```sh
hitch replay <normalized-event-id> --dry-run --config config/default.config.toml
```

Replay without `--dry-run` re-runs configured sync handlers for the stored normalized event and persists new handler invocation records linked to the source event id.
