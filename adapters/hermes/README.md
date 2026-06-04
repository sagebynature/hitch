# Hermes Adapter

Use the Hitch binary as the shell hook entrypoint:

```sh
hitch adapter -harness hermes -event pre_tool_call -sync
```

The adapter reads native hook JSON from stdin and writes Hermes-compatible JSON to stdout for sync hooks.
