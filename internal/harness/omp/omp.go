package omp

import (
	"github.com/sagebynature/hitch/internal/harness/core"
	piharness "github.com/sagebynature/hitch/internal/harness/pi"
	"github.com/sagebynature/hitch/internal/protocol"
)

type Mapper struct{}

var knownSourceEvents = core.SourceEventSet(
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
	"message_end",
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
)

func (Mapper) KnownSourceEvents() map[string]struct{} {
	return knownSourceEvents
}

func (Mapper) Normalize(sourceEventType string, sourcePayload protocol.RawJSON, hitchEventType protocol.EventType) (protocol.EventEnvelope, error) {
	return piharness.NormalizeForHarness(protocol.HarnessOMP, sourceEventType, sourcePayload, hitchEventType)
}

func (Mapper) Translate(sourceEventType string, aggregate protocol.AggregateDecision) (protocol.RawJSON, error) {
	return piharness.TranslateForHarness(sourceEventType, aggregate)
}

func (Mapper) Capability(sourceEventType string) core.SourceEventCapability {
	return piharness.CapabilityForHarness(sourceEventType)
}
