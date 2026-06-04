# Codex Adapter

Use the Hitch binary as the hook entrypoint:

```sh
hitch adapter -harness codex -event PreToolUse -sync
```

The adapter reads the native hook JSON from stdin and writes the native response JSON to stdout for sync hooks.
