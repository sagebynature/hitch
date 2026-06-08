package dispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/sagebynature/hitch/internal/config"
	"github.com/sagebynature/hitch/internal/protocol"
)

type Invocation struct {
	HandlerName string
	Kind        string
	StartedAt   time.Time
	CompletedAt time.Time
	Status      protocol.HandlerStatus
	Stdout      string
	Stderr      string
	Output      protocol.RawJSON
	Decision    protocol.RawJSON
	Error       string
	Result      protocol.HandlerResult
}

type Result struct {
	Aggregate   protocol.AggregateDecision
	Invocations []Invocation
}

type Runner struct {
	Handlers map[string]config.HandlerConfig
	Log      *slog.Logger
}

type Request struct {
	Envelope          protocol.EventEnvelope
	Kind              string
	InboundEventID    string
	NormalizedEventID string
	ReplaySourceID    string
	TotalDeadline     time.Duration
}

func NewRunner(handlers map[string]config.HandlerConfig) Runner { return Runner{Handlers: handlers} }

func NewRunnerWithLogger(handlers map[string]config.HandlerConfig, log *slog.Logger) Runner {
	return Runner{Handlers: handlers, Log: log}
}

func (r Runner) Dispatch(ctx context.Context, req Request) Result {
	selected := r.matchHandlers(req)
	if len(selected) == 0 {
		return Result{Aggregate: protocol.AggregateDecision{Decision: protocol.Decision{Behavior: protocol.BehaviorNone}}}
	}
	if req.TotalDeadline > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.TotalDeadline)
		defer cancel()
	}
	type pair struct {
		idx int
		inv Invocation
	}
	ch := make(chan pair, len(selected))
	for i, name := range selected {
		cfg := r.Handlers[name]
		go func(idx int, n string, c config.HandlerConfig) {
			ch <- pair{idx: idx, inv: runHandler(ctx, r.Log, n, c, req)}
		}(i, name, cfg)
	}
	inv := make([]Invocation, len(selected))
	for range selected {
		p := <-ch
		inv[p.idx] = p.inv
	}
	return Result{Invocations: inv, Aggregate: aggregate(inv, selected, r.Handlers)}
}

func (r Runner) matchHandlers(req Request) []string {
	env := req.Envelope
	names := make([]string, 0, len(r.Handlers))
	for name, h := range r.Handlers {
		if h.Kind != req.Kind {
			continue
		}
		if !matchesHitchEvent(h.HitchEvents, env.HitchEventType) {
			continue
		}
		if !matchesSourceEvent(h.SourceEvents, env) {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func runHandler(parent context.Context, log *slog.Logger, name string, cfg config.HandlerConfig, req Request) Invocation {
	env := req.Envelope
	started := time.Now().UTC()
	inv := Invocation{HandlerName: name, Kind: cfg.Kind, StartedAt: started, Status: protocol.StatusOK}
	ctx, cancel := context.WithTimeout(parent, time.Duration(cfg.TimeoutMS)*time.Millisecond)
	defer cancel()
	invCtx, primaryPayload, err := buildInvocationContext(name, cfg, req)
	if err != nil {
		inv.Status = protocol.StatusError
		inv.Error = err.Error()
		inv.CompletedAt = time.Now().UTC()
		return inv
	}
	args := handlerArgs(cfg.Command, string(primaryPayload))
	cmd := exec.CommandContext(ctx, cfg.Command[0], args...)
	if cfg.WorkingDir != "" {
		cmd.Dir = cfg.WorkingDir
	}
	cmd.Stdin = bytes.NewReader(invCtx)
	cmd.Env = append(cmd.Environ(), "HITCH_CHILD=1")
	logHandlerInvocationStarted(log, inv, env, cfg.TimeoutMS)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	inv.CompletedAt = time.Now().UTC()
	inv.Stdout = stdout.String()
	inv.Stderr = stderr.String()
	defer func() { logHandlerInvocationCompleted(log, inv, env) }()
	if ctx.Err() != nil {
		inv.Status = protocol.StatusTimeout
		inv.Error = ctx.Err().Error()
		inv.Result = protocol.HandlerResult{Status: protocol.StatusTimeout, Decision: &protocol.Decision{Behavior: protocol.BehaviorNone}}
		return inv
	}
	if err != nil {
		inv.Status = protocol.StatusError
		inv.Error = err.Error()
		inv.Result = protocol.HandlerResult{Status: protocol.StatusError, Decision: &protocol.Decision{Behavior: protocol.BehaviorNone}}
		return inv
	}
	out := strings.TrimSpace(inv.Stdout)
	if out == "" {
		inv.Result = protocol.HandlerResult{Status: protocol.StatusOK}
		_ = protocol.NormalizeHandlerResult(&inv.Result)
		inv.Output = protocol.Raw(inv.Result)
		inv.Decision = protocol.Raw(inv.Result.Decision)
		return inv
	}
	var hr protocol.HandlerResult
	if err := json.Unmarshal([]byte(out), &hr); err != nil {
		inv.Status = protocol.StatusError
		inv.Error = fmt.Sprintf("invalid handler JSON: %v", err)
		inv.Result = protocol.HandlerResult{Status: protocol.StatusError, Decision: &protocol.Decision{Behavior: protocol.BehaviorNone}}
		inv.Output = protocol.Raw(map[string]interface{}{"raw_stdout": inv.Stdout})
		inv.Decision = protocol.Raw(inv.Result.Decision)
		return inv
	}
	if err := protocol.NormalizeHandlerResult(&hr); err != nil {
		inv.Status = protocol.StatusError
		inv.Error = err.Error()
		inv.Result = protocol.HandlerResult{Status: protocol.StatusError, Decision: &protocol.Decision{Behavior: protocol.BehaviorNone}}
		inv.Output = protocol.Raw(hr)
		inv.Decision = protocol.Raw(inv.Result.Decision)
		return inv
	}
	inv.Status = hr.Status
	inv.Result = hr
	inv.Output = protocol.Raw(hr)
	inv.Decision = protocol.Raw(hr.Decision)
	return inv
}

func handlerArgs(command []string, selectedPayload string) []string {
	args := append([]string(nil), command[1:]...)
	if isShellCommand(command) {
		args = append(args, "hitch-handler", selectedPayload)
		return args
	}
	args = append(args, selectedPayload)
	return args
}

func isShellCommand(command []string) bool {
	if len(command) >= 3 && isShell(command[0]) && command[1] == "-c" {
		return true
	}
	return len(command) >= 4 && isEnv(command[0]) && isShell(command[1]) && command[2] == "-c"
}

func isShell(command string) bool {
	switch shellName(command) {
	case "sh", "bash":
		return true
	default:
		return false
	}
}

func isEnv(command string) bool {
	return shellName(command) == "env"
}

func shellName(command string) string {
	if i := strings.LastIndexByte(command, '/'); i >= 0 {
		return command[i+1:]
	}
	return command
}

func matchesHitchEvent(events []string, event protocol.EventType) bool {
	for _, e := range events {
		if e == "*" || e == string(event) {
			return true
		}
	}
	return false
}

func matchesSourceEvent(filters []config.SourceEventFilter, env protocol.EventEnvelope) bool {
	if len(filters) == 0 {
		return true
	}
	for _, f := range filters {
		if f.Harness == string(env.Harness) && f.SourceEventType == env.SourceEventType {
			return true
		}
	}
	return false
}

func buildInvocationContext(name string, cfg config.HandlerConfig, req Request) ([]byte, protocol.RawJSON, error) {
	env := req.Envelope
	payloadKind := "hitch"
	payload := env.Payload
	if cfg.Payload == "source" {
		payloadKind = "source"
		payload = env.SourcePayload
	}
	primaryPayload, err := compactJSON(payload)
	if err != nil {
		return nil, nil, err
	}
	invCtx := protocol.InvocationContext{
		HitchVersion:      env.HitchVersion,
		HandlerName:       name,
		HandlerType:       cfg.Type,
		Kind:              req.Kind,
		InboundEventID:    req.InboundEventID,
		NormalizedEventID: req.NormalizedEventID,
		PayloadKind:       payloadKind,
		Payload:           primaryPayload,
		Event: protocol.InvocationEvent{
			HitchVersion:    env.HitchVersion,
			EventID:         env.EventID,
			ReceivedAt:      env.ReceivedAt,
			Harness:         env.Harness,
			SourceEventType: env.SourceEventType,
			SourcePayload:   env.SourcePayload,
			HitchEventType:  env.HitchEventType,
			SessionID:       env.SessionID,
			TurnID:          env.TurnID,
			CWD:             env.CWD,
			Model:           env.Model,
			TranscriptPath:  env.TranscriptPath,
			Payload:         env.Payload,
		},
	}
	stdin, err := json.Marshal(invCtx)
	if err != nil {
		return nil, nil, err
	}
	return stdin, primaryPayload, nil
}

func compactJSON(raw protocol.RawJSON) (protocol.RawJSON, error) {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return nil, err
	}
	return protocol.RawJSON(buf.Bytes()), nil
}

func logHandlerInvocationStarted(log *slog.Logger, inv Invocation, env protocol.EventEnvelope, timeoutMS int) {
	if log == nil {
		return
	}
	log.Info("handler invocation starting", handlerLogAttrs(inv, env, "timeout_ms", timeoutMS)...)
}

func logHandlerInvocationCompleted(log *slog.Logger, inv Invocation, env protocol.EventEnvelope) {
	if log == nil {
		return
	}
	attrs := handlerLogAttrs(inv, env, "status", inv.Status, "duration_ms", inv.CompletedAt.Sub(inv.StartedAt).Milliseconds())
	if inv.Error != "" {
		attrs = append(attrs, "error", inv.Error)
	}
	log.Info("handler invocation completed", attrs...)
}

func handlerLogAttrs(inv Invocation, env protocol.EventEnvelope, extra ...any) []any {
	attrs := []any{
		"handler", inv.HandlerName,
		"kind", inv.Kind,
		"harness", env.Harness,
		"source_event_type", env.SourceEventType,
		"hitch_event_type", env.HitchEventType,
		"event_id", env.EventID,
	}
	if env.SessionID != "" {
		attrs = append(attrs, "session_id", env.SessionID)
	}
	if env.TurnID != "" {
		attrs = append(attrs, "turn_id", env.TurnID)
	}
	if env.CWD != "" {
		attrs = append(attrs, "cwd", env.CWD)
	}
	if env.Model != "" {
		attrs = append(attrs, "model", env.Model)
	}
	return append(attrs, extra...)
}

func aggregate(inv []Invocation, order []string, handlers map[string]config.HandlerConfig) protocol.AggregateDecision {
	_ = order
	best := protocol.Decision{Behavior: protocol.BehaviorNone}
	var native *protocol.Decision
	var contexts []string
	var errs []string
	transformSeen := false
	for _, in := range inv {
		if in.Error != "" {
			errs = append(errs, in.HandlerName+": "+in.Error)
		}
		if in.Status == protocol.StatusSkipped || in.Status == protocol.StatusReserved {
			continue
		}
		if in.Status != protocol.StatusOK {
			if failClosed(handlers[in.HandlerName], in.Status) {
				return protocol.AggregateDecision{Decision: protocol.Decision{Behavior: protocol.BehaviorBlock, Reason: in.StatusReason()}, HandlerResults: results(inv), Errors: errs}
			}
			continue
		}
		d := in.Result.Decision
		if d == nil {
			continue
		}
		if len(d.NativeResponse) != 0 && native == nil {
			candidate := *d
			native = &candidate
		}
		if d.Behavior == protocol.BehaviorInjectContext && d.Context != "" {
			contexts = append(contexts, d.Context)
		}
		if d.Behavior == protocol.BehaviorTransform || d.Behavior == protocol.BehaviorReplaceResult {
			if transformSeen {
				errs = append(errs, "multiple transform decisions rejected")
				continue
			}
			transformSeen = true
		}
		if precedence(d.Behavior) > precedence(best.Behavior) {
			best = *d
		}
	}
	if native != nil {
		return protocol.AggregateDecision{Decision: *native, HandlerResults: results(inv), Errors: errs}
	}
	if best.Behavior == protocol.BehaviorNone && len(contexts) > 0 {
		best = protocol.Decision{Behavior: protocol.BehaviorInjectContext, Context: strings.Join(contexts, "\n\n")}
	}
	if best.Behavior == protocol.BehaviorInjectContext && len(contexts) > 0 {
		best.Context = strings.Join(contexts, "\n\n")
	}
	return protocol.AggregateDecision{Decision: best, HandlerResults: results(inv), Errors: errs}
}

func failClosed(h config.HandlerConfig, status protocol.HandlerStatus) bool {
	if status == protocol.StatusTimeout {
		return h.OnTimeout == "fail_closed"
	}
	return h.OnError == "fail_closed"
}

func (i Invocation) StatusReason() string {
	if i.Error != "" {
		return i.Error
	}
	return string(i.Status)
}

func precedence(b protocol.DecisionBehavior) int {
	switch b {
	case protocol.BehaviorDeny, protocol.BehaviorBlock, protocol.BehaviorStop:
		return 5
	case protocol.BehaviorHandled:
		return 4
	case protocol.BehaviorTransform, protocol.BehaviorReplaceResult:
		return 3
	case protocol.BehaviorInjectContext:
		return 2
	case protocol.BehaviorAllow:
		return 1
	default:
		return 0
	}
}

func results(inv []Invocation) []protocol.HandlerResult {
	out := make([]protocol.HandlerResult, 0, len(inv))
	for _, i := range inv {
		r := i.Result
		if r.Status == "" {
			r.Status = i.Status
		}
		if r.Decision == nil {
			r.Decision = &protocol.Decision{Behavior: protocol.BehaviorNone}
		}
		out = append(out, r)
	}
	return out
}

var ErrNoHandlers = errors.New("no handlers")
