package antigravity

import (
	"testing"

	"github.com/sagebynature/hitch/internal/protocol"
)

func TestTranslate(t *testing.T) {
	m := Mapper{}

	tests := []struct {
		name            string
		sourceEventType string
		decision        protocol.Decision
		wantJSON        string
	}{
		{
			name:            "PreToolUse Deny",
			sourceEventType: "PreToolUse",
			decision:        protocol.Decision{Behavior: protocol.BehaviorDeny, Reason: "blocked by policy"},
			wantJSON:        `{"decision":"deny","reason":"blocked by policy"}`,
		},
		{
			name:            "PreToolUse Allow",
			sourceEventType: "PreToolUse",
			decision:        protocol.Decision{Behavior: protocol.BehaviorAllow},
			wantJSON:        `{"decision":"allow"}`,
		},
		{
			name:            "PostToolUse",
			sourceEventType: "PostToolUse",
			decision:        protocol.Decision{Behavior: protocol.BehaviorNone},
			wantJSON:        `{}`,
		},
		{
			name:            "PreInvocation InjectContext",
			sourceEventType: "PreInvocation",
			decision:        protocol.Decision{Behavior: protocol.BehaviorInjectContext, Context: "Remember to lint"},
			wantJSON:        `{"injectSteps":[{"ephemeralMessage":"Remember to lint"}]}`,
		},
		{
			name:            "PostInvocation Stop",
			sourceEventType: "PostInvocation",
			decision:        protocol.Decision{Behavior: protocol.BehaviorStop},
			wantJSON:        `{"terminationBehavior":"terminate"}`,
		},
		{
			name:            "Stop Continue",
			sourceEventType: "Stop",
			decision:        protocol.Decision{Behavior: protocol.BehaviorContinue, Reason: "Not done yet"},
			wantJSON:        `{"decision":"continue","reason":"Not done yet"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := m.Translate(tt.sourceEventType, protocol.AggregateDecision{Decision: tt.decision})
			if err != nil {
				t.Fatalf("Translate error: %v", err)
			}
			if string(b) != tt.wantJSON {
				t.Errorf("got %s, want %s", string(b), tt.wantJSON)
			}
		})
	}
}
