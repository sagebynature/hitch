package opencode

import (
	"encoding/json"

	"github.com/sagebynature/hitch/internal/harness/core"
	"github.com/sagebynature/hitch/internal/protocol"
)

type Mapper struct{}

var controlCapableEvents = map[string]struct{}{
	"chat.message":                         {},
	"tool.execute.before":                  {},
	"tool.execute.after":                   {},
	"permission.ask":                       {},
	"command.execute.before":               {},
	"chat.params":                          {},
	"chat.headers":                         {},
	"shell.env":                            {},
	"tool.definition":                      {},
	"experimental.session.compacting":      {},
	"experimental.compaction.autocontinue": {},
	"experimental.text.complete":           {},
}

var knownSourceEvents = core.SourceEventSet(
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
)

func (Mapper) KnownSourceEvents() map[string]struct{} {
	return knownSourceEvents
}

func (Mapper) Capability(sourceEventType string) core.SourceEventCapability {
	return core.CapabilityFromSet(sourceEventType, controlCapableEvents)
}

type pluginPayloadEnvelope struct {
	Event    protocol.RawJSON `json:"event"`
	Metadata pluginMetadata   `json:"metadata"`
}

type pluginMetadata struct {
	SessionID      string `json:"session_id"`
	TurnID         string `json:"turn_id"`
	CWD            string `json:"cwd"`
	Model          string `json:"model"`
	TranscriptPath string `json:"transcript_path"`
}

type AdapterResponse struct {
	AdapterAction string      `json:"adapter_action"`
	Path          []string    `json:"path,omitempty"`
	Value         interface{} `json:"value,omitempty"`
	Message       string      `json:"message,omitempty"`
}

func (Mapper) Normalize(sourceEventType string, sourcePayload protocol.RawJSON, hitchEventType protocol.EventType) (protocol.EventEnvelope, error) {
	eventPayload, meta := unwrapSourcePayload(sourcePayload)
	env := core.NewEnvelope(protocol.HarnessOpenCode, sourceEventType, sourcePayload, hitchEventType, eventPayload)
	env.SessionID = meta.SessionID
	env.TurnID = meta.TurnID
	env.CWD = meta.CWD
	env.Model = meta.Model
	env.TranscriptPath = meta.TranscriptPath
	if env.SessionID == "" && env.TurnID == "" && env.CWD == "" && env.Model == "" && env.TranscriptPath == "" {
		core.ApplySourceMetadata(&env, eventPayload)
	}
	return env, protocol.ValidateEnvelope(env)
}

func unwrapSourcePayload(sourcePayload protocol.RawJSON) (protocol.RawJSON, pluginMetadata) {
	var wrapped pluginPayloadEnvelope
	if err := json.Unmarshal(sourcePayload, &wrapped); err != nil {
		return sourcePayload, pluginMetadata{}
	}
	if len(wrapped.Event) != 0 && json.Valid(wrapped.Event) {
		return wrapped.Event, wrapped.Metadata
	}
	return sourcePayload, pluginMetadata{}
}

func (Mapper) Translate(sourceEventType string, aggregate protocol.AggregateDecision) (protocol.RawJSON, error) {
	d := aggregate.Decision
	if len(d.NativeResponse) != 0 {
		return d.NativeResponse, nil
	}
	resp := AdapterResponse{AdapterAction: "noop"}
	switch sourceEventType {
	case "tool.execute.before":
		if shouldThrow(d.Behavior) {
			resp.AdapterAction = "throw"
			resp.Message = d.Reason
		} else if d.Behavior == protocol.BehaviorTransform && len(d.UpdatedInput) != 0 {
			resp.AdapterAction = "set"
			resp.Path = []string{"args"}
			resp.Value = decodeJSON(d.UpdatedInput)
		}
	case "tool.execute.after", "experimental.text.complete":
		if (d.Behavior == protocol.BehaviorReplaceResult || d.Behavior == protocol.BehaviorTransform) && len(d.UpdatedOutput) != 0 {
			resp.AdapterAction = "set"
			resp.Path = []string{"output"}
			resp.Value = decodeJSON(d.UpdatedOutput)
		}
	case "permission.ask":
		if d.Behavior == protocol.BehaviorAllow {
			resp.AdapterAction = "set"
			resp.Path = []string{"status"}
			resp.Value = "allow"
		} else if shouldThrow(d.Behavior) {
			resp.AdapterAction = "set"
			resp.Path = []string{"status"}
			resp.Value = "deny"
		}
	case "chat.message", "command.execute.before":
		if shouldThrow(d.Behavior) {
			resp.AdapterAction = "throw"
			resp.Message = d.Reason
		} else if d.Behavior == protocol.BehaviorInjectContext && d.Context != "" {
			resp.AdapterAction = "inject_context"
			resp.Value = d.Context
		} else if d.Behavior == protocol.BehaviorTransform && len(d.UpdatedInput) != 0 {
			resp.AdapterAction = "set"
			resp.Path = []string{"parts"}
			resp.Value = decodeJSON(d.UpdatedInput)
		}
	case "shell.env", "chat.params", "chat.headers", "tool.definition":
		if d.Behavior == protocol.BehaviorTransform && len(d.UpdatedInput) != 0 {
			resp.AdapterAction = "set"
			resp.Value = decodeJSON(d.UpdatedInput)
		}
	case "experimental.session.compacting":
		if d.Behavior == protocol.BehaviorInjectContext && d.Context != "" {
			resp.AdapterAction = "append"
			resp.Path = []string{"context"}
			resp.Value = d.Context
		} else if d.Behavior == protocol.BehaviorTransform && len(d.UpdatedInput) != 0 {
			resp.AdapterAction = "set"
			resp.Path = []string{"prompt"}
			resp.Value = decodeJSON(d.UpdatedInput)
		}
	case "experimental.compaction.autocontinue":
		if d.Behavior == protocol.BehaviorStop || d.Behavior == protocol.BehaviorBlock {
			resp.AdapterAction = "set"
			resp.Path = []string{"enabled"}
			resp.Value = false
		}
	}
	return protocol.Raw(resp), nil
}

func shouldThrow(b protocol.DecisionBehavior) bool {
	return b == protocol.BehaviorBlock || b == protocol.BehaviorDeny || b == protocol.BehaviorStop
}

func decodeJSON(raw protocol.RawJSON) interface{} {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	return v
}
