# Handler Protocol

For a step-by-step developer guide with runnable examples, see [Building Hitch Event Handlers](handler-development.md).

Handlers are configured in `~/.config/hitch/config.toml`, the config passed with `--config`, or discovered as native Python extensions under `~/.config/hitch/extensions`.

Input: Hitch writes one invocation context JSON object to handler stdin. The context contains the selected primary `payload`, legacy top-level envelope fields, and a nested `event` object that always includes both `source_payload` and Hitch `payload`.

Shell handler argv: Hitch appends the selected primary payload as one compact JSON argument. For `sh -c` and `bash -c` wrappers, Hitch preserves or inserts the shell `$0` placeholder so the payload is available as `$1`.

Native Python handlers receive the same context through the Hitch SDK and return `HandlerResult`.
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

- stdin context includes selected payload plus source and Hitch payloads.
- shell handlers receive the selected payload argument.
- stdout JSON is parsed into a handler result.
- invalid stdout JSON is persisted as `status = error`.
- timeouts are persisted as `status = timeout`.
- stderr is captured separately from parsed stdout.
- deterministic aggregation uses decision precedence and lexicographic handler-name order.
- multiple context injections concatenate in handler-name order.
- multiple transforms are rejected unless transform chaining is explicitly configured.
- internal `reserved` and `skipped` invocation rows are not accepted as child handler-result statuses.

Hitch sets `HITCH_CHILD=1` on child handler processes for recursion guards.
