# Handler Protocol

Handlers are external commands configured in `~/.config/hitch/config.toml`.

Input: Hitch writes one normalized event envelope JSON object to handler stdin.

Output: handlers may write one JSON object to stdout:

```json
{
  "status": "ok",
  "decision": {
    "behavior": "none"
  }
}
```

Allowed statuses: `ok`, `error`, `timeout`.

Allowed decisions: `none`, `allow`, `deny`, `block`, `continue`, `stop`, `transform`, `replace_result`, `inject_context`, `handled`.

Invalid JSON is persisted as a handler error and does not crash Hitch. Handler stderr is captured for audit records.
