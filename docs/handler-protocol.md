# Handler Protocol

For a step-by-step developer guide with runnable examples, see [Building Hitch Event Handlers](handler-development.md).

Handlers are external commands configured in `~/.config/hitch/config.toml` or the config passed with `--config`.

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

If `decision` is omitted, Hitch treats it as `{"behavior":"none"}`.

Allowed statuses: `ok`, `error`, `timeout`.

Allowed decisions: `none`, `allow`, `deny`, `block`, `continue`, `stop`, `transform`, `replace_result`, `inject_context`, `handled`.

Handler behavior verified by tests:

- stdout JSON is parsed into a handler result.
- invalid stdout JSON is persisted as `status = error`.
- timeouts are persisted as `status = timeout`.
- stderr is captured separately from parsed stdout.
- deterministic aggregation uses decision precedence and lexicographic handler-name order.
- multiple context injections concatenate in handler-name order.
- multiple transforms are rejected unless transform chaining is explicitly configured.

Hitch sets `HITCH_CHILD=1` on child handler processes for recursion guards.
