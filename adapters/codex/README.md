# Codex Adapter

Use the Hitch binary as the hook entrypoint. Install writes one sync command hook per supported Codex lifecycle event:

```sh
hitch adapter -harness codex -event SessionStart -sync
hitch adapter -harness codex -event PreToolUse -sync
hitch adapter -harness codex -event Stop -sync
```

The adapter reads the native hook JSON from stdin and writes the native response JSON to stdout for sync hooks. The seeded Hitch config includes `hitch handler noop-observer`, which reads each normalized event envelope and returns `{"status":"ok","decision":{"behavior":"none"}}`.
