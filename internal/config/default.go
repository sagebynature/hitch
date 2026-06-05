package config

// DefaultConfigTOML is the configuration seeded by the installer when
// ~/.config/hitch/config.toml does not exist.
const DefaultConfigTOML = `[server]
host = "127.0.0.1"
port = 8799
max_request_bytes = 1048576

[log]
level = "info"
format = "json"
include_native_payload = false

[log.stdout]
enabled = true

[log.file]
enabled = false
path = "~/.local/state/hitch/hitch.log"
max_size_mb = 100
max_backups = 10
max_age_days = 14
compress = true

[log.otlp]
enabled = false
endpoint = "http://127.0.0.1:4318"
protocol = "http/protobuf"

[audit]
enabled = true
backend = "sqlite"

[audit.sqlite]
path = "~/.local/share/hitch/events.sqlite"

[harness.codex]
enabled = true

[harness.codex.event_map]
SessionStart = "session.started"
SubagentStart = "subagent.started"
UserPromptSubmit = "turn.user_prompt"
PreToolUse = "tool.requested"
PermissionRequest = "tool.permission_requested"
PostToolUse = "tool.completed"
PreCompact = "session.compacted"
PostCompact = "session.compacted"
SubagentStop = "subagent.completed"
Stop = "turn.completed"

[harness.hermes]
enabled = true

[harness.hermes.event_map]
pre_tool_call = "tool.requested"
post_tool_call = "tool.completed"
pre_llm_call = "turn.started"
post_llm_call = "turn.completed"
on_session_start = "session.started"
on_session_end = "session.ended"
subagent_stop = "subagent.completed"
transform_tool_result = "tool.completed"
transform_terminal_output = "tool.completed"
transform_llm_output = "turn.completed"
pre_gateway_dispatch = "turn.user_prompt"

[harness.pi]
enabled = true

[harness.pi.event_map]
input = "turn.user_prompt"
before_agent_start = "turn.started"
agent_start = "turn.started"
turn_start = "turn.started"
context = "turn.started"
before_provider_request = "turn.started"
tool_call = "tool.requested"
tool_result = "tool.completed"
turn_end = "turn.completed"
agent_end = "turn.completed"
session_start = "session.started"
session_shutdown = "session.ended"
session_before_switch = "session.resumed"
session_before_fork = "session.resumed"
session_before_compact = "session.compacted"
session_compact = "session.compacted"
user_bash = "tool.requested"

[harness.omp]
enabled = true

[harness.omp.event_map]
input = "turn.user_prompt"
before_agent_start = "turn.started"
turn_start = "turn.started"
tool_call = "tool.requested"
tool_result = "tool.completed"
turn_end = "turn.completed"
auto_compaction_start = "session.compacted"
todo_reminder = "turn.started"

[handlers.noop_observer]
command = ["hitch", "handler", "noop-observer"]
events = ["*"]
mode = "sync"
timeout_ms = 1000
on_error = "fail_open"
on_timeout = "fail_open"
`
