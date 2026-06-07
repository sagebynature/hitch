package install

var codexLifecycleEvents = []string{
	"SessionStart",
	"SubagentStart",
	"UserPromptSubmit",
	"PreToolUse",
	"PermissionRequest",
	"PostToolUse",
	"PreCompact",
	"PostCompact",
	"SubagentStop",
	"Stop",
}

var hermesHookEvents = []string{
	"pre_tool_call",
	"post_tool_call",
	"pre_llm_call",
	"post_llm_call",
	"on_session_start",
	"on_session_end",
	"subagent_stop",
	"transform_tool_result",
	"transform_terminal_output",
	"transform_llm_output",
	"pre_gateway_dispatch",
}

var hermesHookSyncEvents = map[string]struct{}{
	"pre_tool_call":             {},
	"pre_llm_call":              {},
	"transform_tool_result":     {},
	"transform_terminal_output": {},
	"transform_llm_output":      {},
	"pre_gateway_dispatch":      {},
}

var piExtensionEvents = []string{
	"input",
	"before_agent_start",
	"agent_start",
	"turn_start",
	"context",
	"before_provider_request",
	"tool_call",
	"tool_result",
	"turn_end",
	"agent_end",
	"session_start",
	"session_shutdown",
	"session_before_switch",
	"session_before_fork",
	"session_before_compact",
	"session_compact",
	"user_bash",
}

var piExtensionSyncEvents = []string{
	"input",
	"context",
	"before_provider_request",
	"tool_call",
	"tool_result",
	"session_before_switch",
	"session_before_fork",
	"session_before_compact",
	"user_bash",
}

var ompExtensionEvents = []string{
	"input",
	"before_agent_start",
	"agent_start",
	"agent_end",
	"turn_start",
	"turn_end",
	"before_provider_request",
	"after_provider_response",
	"context",
	"message_start",
	// OMP v15 TUI clears the final assistant message after a message_end extension callback,
	// even when the callback returns undefined. Do not register it until OMP fixes that behavior.
	"tool_call",
	"tool_result",
	"tool_execution_start",
	"tool_execution_update",
	"tool_execution_end",
	"session_start",
	"session_before_switch",
	"session_switch",
	"session_before_branch",
	"session_branch",
	"session_before_compact",
	"session.compacting",
	"session_compact",
	"session_before_tree",
	"session_tree",
	"session_shutdown",
	"auto_compaction_start",
	"auto_compaction_end",
	"auto_retry_start",
	"auto_retry_end",
	"ttsr_triggered",
	"todo_reminder",
	"goal_updated",
	"credential_disabled",
	"user_bash",
	"user_python",
}

var ompExtensionSyncEvents = []string{
	"input",
	"context",
	"before_provider_request",
	"tool_call",
	"tool_result",
	"session_before_switch",
	"session_before_branch",
	"session_before_compact",
	"session.compacting",
	"user_bash",
	"user_python",
	"session_before_tree",
}

var opencodeHookEvents = []string{
	"chat.message",
	"chat.params",
	"chat.headers",
	"command.execute.before",
	"command.executed",
	"permission.ask",
	"permission.asked",
	"permission.updated",
	"permission.replied",
	"tool.execute.before",
	"tool.execute.after",
	"tool.definition",
	"shell.env",
	"experimental.session.compacting",
	"experimental.compaction.autocontinue",
	"experimental.text.complete",
	"session.created",
	"session.updated",
	"session.deleted",
	"session.diff",
	"session.error",
	"session.idle",
	"session.status",
	"session.compacted",
	"message.updated",
	"message.removed",
	"message.part.updated",
	"message.part.removed",
	"file.edited",
	"file.watcher.updated",
	"todo.updated",
	"server.connected",
	"server.instance.disposed",
	"installation.updated",
	"installation.update-available",
	"lsp.client.diagnostics",
	"lsp.updated",
	"tui.prompt.append",
	"tui.command.execute",
	"tui.toast.show",
	"pty.created",
	"pty.updated",
	"pty.exited",
	"pty.deleted",
	"vcs.branch.updated",
}

const piManagedExtensionMarker = "Managed by Hitch"

type harnessSpec struct {
	Name       string
	Title      string
	Command    string
	ConfigPath string
	Supported  bool
	Reason     string
}

type harnessDetection struct {
	Harness    string `json:"harness"`
	Available  bool   `json:"available"`
	Reason     string `json:"reason"`
	BinaryPath string `json:"binary_path,omitempty"`
	ConfigPath string `json:"config_path,omitempty"`
	Installed  bool   `json:"installed"`
	Supported  bool   `json:"supported"`
}

type installOperation struct {
	Harness    string `json:"harness"`
	Action     string `json:"action"`
	Path       string `json:"path,omitempty"`
	BackupPath string `json:"backup_path,omitempty"`
	Status     string `json:"status,omitempty"`
	Reason     string `json:"reason,omitempty"`
	AdapterURL string `json:"-"`
}

func knownHarnessSpecs() []harnessSpec {
	return []harnessSpec{
		{Name: "codex", Title: "Codex", Command: "codex", ConfigPath: "~/.codex/hooks.json", Supported: true},
		{Name: "hermes", Title: "Hermes", Command: "hermes", ConfigPath: "~/.hermes/config.yaml", Supported: true},
		{Name: "pi", Title: "Pi", Command: "pi", ConfigPath: "~/.pi/agent/extensions/hitch/index.ts", Supported: true},
		{Name: "omp", Title: "OMP", Command: "omp", ConfigPath: "~/.omp/agent/extensions/hitch/index.ts", Supported: true},
		{Name: "opencode", Title: "OpenCode", Command: "opencode", ConfigPath: "~/.config/opencode/plugins/hitch.ts", Supported: true},
	}
}

func knownHarnessNames() []string {
	specs := knownHarnessSpecs()
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, spec.Name)
	}
	return names
}
