package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/sagebynature/hitch/internal/config"
	"github.com/sagebynature/hitch/internal/dispatch"
	"github.com/sagebynature/hitch/internal/harness"
	"github.com/sagebynature/hitch/internal/protocol"
	"github.com/sagebynature/hitch/internal/store"
)

type Server struct {
	cfg       config.Config
	log       *slog.Logger
	store     *store.Store
	runner    dispatch.Runner
	mux       *http.ServeMux
	harnesses map[protocol.Harness]harnessRuntime
}

type harnessRuntime struct {
	normalizer harness.Normalizer
	eventMap   map[string]config.EventTypes
}

type EventRequest struct {
	Mode               string           `json:"mode"`
	Harness            string           `json:"harness"`
	SourceEventType    string           `json:"source_event_type"`
	SourcePayload      protocol.RawJSON `json:"source_payload"`
	HitchClientVersion string           `json:"hitch_client_version"`
}

type EventResponse struct {
	EventID           string `json:"event_id"`
	NormalizedEventID string `json:"normalized_event_id"`
}

type apiRequestLog struct {
	started time.Time
	attrs   []any
}

func newAPIRequestLog(r *http.Request) apiRequestLog {
	return apiRequestLog{
		started: time.Now(),
		attrs: []any{
			"method", r.Method,
			"path", r.URL.Path,
		},
	}
}

func (l *apiRequestLog) add(key string, value any) {
	if value == nil {
		return
	}
	if s, ok := value.(string); ok && s == "" {
		return
	}
	l.attrs = append(l.attrs, key, value)
}

func (l *apiRequestLog) addEventRequest(mode string, req EventRequest) {
	if mode == "" {
		mode = strings.TrimSpace(req.Mode)
	}
	l.add("mode", mode)
	l.add("harness", req.Harness)
	l.add("source_event_type", req.SourceEventType)
}

func (l *apiRequestLog) addEnvelope(env protocol.EventEnvelope, normalizedID string) {
	l.add("event_id", env.EventID)
	l.add("normalized_event_id", normalizedID)
	l.add("hitch_event_type", string(env.HitchEventType))
	l.add("session_id", env.SessionID)
	l.add("turn_id", env.TurnID)
	l.add("cwd", env.CWD)
	l.add("model", env.Model)
}

func (l *apiRequestLog) addEnvelopeMetadata(env protocol.EventEnvelope) {
	l.add("hitch_event_type", string(env.HitchEventType))
	l.add("session_id", env.SessionID)
	l.add("turn_id", env.TurnID)
	l.add("cwd", env.CWD)
	l.add("model", env.Model)
}

func (l apiRequestLog) emit(ctx context.Context, logger *slog.Logger, level slog.Level, msg string, status int, extra ...any) {
	attrs := append([]any{}, l.attrs...)
	attrs = append(attrs, "status", status, "duration_ms", time.Since(l.started).Milliseconds())
	attrs = append(attrs, extra...)
	logger.Log(ctx, level, msg, attrs...)
}

const (
	requestModeAsync = "async"
	requestModeSync  = "sync"
)

func normalizeRequestMode(mode string) (string, error) {
	switch strings.TrimSpace(mode) {
	case "", requestModeAsync:
		return requestModeAsync, nil
	case requestModeSync:
		return requestModeSync, nil
	default:
		return "", badRequest("mode must be async or sync")
	}
}

func syncOutcome(decision protocol.Decision) string {
	if len(decision.NativeResponse) != 0 {
		return "handler_decision"
	}
	if decision.Behavior == protocol.BehaviorNone {
		return "passthrough"
	}
	return "handler_decision"
}

func New(cfg config.Config, log *slog.Logger, st *store.Store) *Server {
	return NewWithHarnessRegistry(cfg, log, st, harness.DefaultRegistry())
}

func NewWithHarnessRegistry(cfg config.Config, log *slog.Logger, st *store.Store, registry harness.Registry) *Server {
	s := &Server{cfg: cfg, log: log, store: st, runner: dispatch.NewRunnerWithLogger(cfg.Handlers, log), harnesses: buildHarnessRuntimes(cfg, registry)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", s.handleHealth)
	mux.HandleFunc("POST /v1/events", s.handleEvent)
	mux.HandleFunc("GET /v1/events/", s.handleGetEvent)
	s.mux = mux
	return s
}

func buildHarnessRuntimes(cfg config.Config, registry harness.Registry) map[protocol.Harness]harnessRuntime {
	return map[protocol.Harness]harnessRuntime{
		protocol.HarnessCodex:    buildHarnessRuntime(registry, protocol.HarnessCodex, cfg.Harness.Codex.EventMap),
		protocol.HarnessHermes:   buildHarnessRuntime(registry, protocol.HarnessHermes, cfg.Harness.Hermes.EventMap),
		protocol.HarnessPi:       buildHarnessRuntime(registry, protocol.HarnessPi, cfg.Harness.Pi.EventMap),
		protocol.HarnessOMP:      buildHarnessRuntime(registry, protocol.HarnessOMP, cfg.Harness.OMP.EventMap),
		protocol.HarnessOpenCode: buildHarnessRuntime(registry, protocol.HarnessOpenCode, cfg.Harness.OpenCode.EventMap),
	}
}

func buildHarnessRuntime(registry harness.Registry, h protocol.Harness, eventMap map[string]config.EventTypes) harnessRuntime {
	normalizer, _ := registry.Lookup(h)
	return harnessRuntime{normalizer: normalizer, eventMap: eventMap}
}

func (s *Server) Handler() http.Handler {
	return http.MaxBytesHandler(s.mux, s.cfg.Server.MaxRequestBytes)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	log := newAPIRequestLog(r)
	if err := writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok"}); err != nil {
		log.emit(r.Context(), s.log, slog.LevelInfo, "api response write failed", http.StatusOK, "error_kind", "response_write_failed", "error", err.Error())
		return
	}
	log.emit(r.Context(), s.log, slog.LevelDebug, "api request completed", http.StatusOK)
}

func (s *Server) handleEvent(w http.ResponseWriter, r *http.Request) {
	log := newAPIRequestLog(r)
	resp, env, req, mode, err := s.ingest(r.Context(), r)
	log.addEventRequest(mode, req)
	if env.EventID != "" {
		log.addEnvelope(env, resp.NormalizedEventID)
	}
	if err != nil {
		var unmapped unmappedSourceEventError
		if errors.As(err, &unmapped) {
			log.emit(r.Context(), s.log, slog.LevelDebug, "api request ignored", http.StatusAccepted, "error_kind", errorKind(err), "reason", "source event is known but not mapped in config")
			if writeErr := writeJSON(w, http.StatusAccepted, EventResponse{}); writeErr != nil {
				log.emit(r.Context(), s.log, slog.LevelInfo, "api response write failed", http.StatusAccepted, "error_kind", "response_write_failed", "error", writeErr.Error())
			}
			return
		}
		status := statusForError(err)
		log.emit(r.Context(), s.log, slog.LevelInfo, "api request failed", status, "error_kind", errorKind(err), "error", err.Error())
		if writeErr := writeError(w, err); writeErr != nil {
			log.emit(r.Context(), s.log, slog.LevelInfo, "api response write failed", status, "error_kind", "response_write_failed", "error", writeErr.Error())
		}
		return
	}
	if mode == requestModeAsync {
		go s.dispatchObservers(context.Background(), resp.NormalizedEventID, env)
		if err := writeJSON(w, http.StatusAccepted, resp); err != nil {
			log.emit(r.Context(), s.log, slog.LevelInfo, "api response write failed", http.StatusAccepted, "error_kind", "response_write_failed", "error", err.Error())
			return
		}
		log.emit(r.Context(), s.log, slog.LevelDebug, "api request completed", http.StatusAccepted)
		return
	}
	s.handleSyncEvent(w, r, resp, env, log)
}
func (s *Server) handleSyncEvent(w http.ResponseWriter, r *http.Request, resp EventResponse, env protocol.EventEnvelope, log apiRequestLog) {
	result := s.runner.Dispatch(r.Context(), env, "control", 2*time.Second)
	for _, inv := range result.Invocations {
		if err := s.store.InsertHandlerInvocation(r.Context(), store.HandlerInvocation{ID: harness.NewID("hinv"), NormalizedEventID: resp.NormalizedEventID, HandlerName: inv.HandlerName, Kind: inv.Kind, StartedAt: inv.StartedAt, CompletedAt: inv.CompletedAt, Status: inv.Status, Stdout: inv.Stdout, Stderr: inv.Stderr, Output: inv.Output, Decision: inv.Decision, Error: inv.Error}); err != nil {
			log.emit(r.Context(), s.log, slog.LevelInfo, "api request failed", http.StatusInternalServerError, "control_handler_count", len(result.Invocations), "error_kind", errorKind(err), "error", err.Error())
			if writeErr := writeError(w, err); writeErr != nil {
				log.emit(r.Context(), s.log, slog.LevelInfo, "api response write failed", http.StatusInternalServerError, "control_handler_count", len(result.Invocations), "error_kind", "response_write_failed", "error", writeErr.Error())
			}
			return
		}
	}
	runtime := s.harnesses[env.Harness]
	native, err := runtime.normalizer.Translate(env.SourceEventType, result.Aggregate)
	if err != nil {
		log.emit(r.Context(), s.log, slog.LevelInfo, "api request failed", http.StatusInternalServerError, "control_handler_count", len(result.Invocations), "error_kind", errorKind(err), "error", err.Error())
		if writeErr := writeError(w, err); writeErr != nil {
			log.emit(r.Context(), s.log, slog.LevelInfo, "api response write failed", http.StatusInternalServerError, "control_handler_count", len(result.Invocations), "error_kind", "response_write_failed", "error", writeErr.Error())
		}
		return
	}
	if err := s.store.InsertNativeResponse(r.Context(), store.NativeResponse{ID: harness.NewID("nresp"), NormalizedEventID: resp.NormalizedEventID, Response: native, EmittedAt: time.Now().UTC()}); err != nil {
		log.emit(r.Context(), s.log, slog.LevelInfo, "api request failed", http.StatusInternalServerError, "control_handler_count", len(result.Invocations), "native_response_bytes", len(native), "error_kind", errorKind(err), "error", err.Error())
		if writeErr := writeError(w, err); writeErr != nil {
			log.emit(r.Context(), s.log, slog.LevelInfo, "api response write failed", http.StatusInternalServerError, "control_handler_count", len(result.Invocations), "native_response_bytes", len(native), "error_kind", "response_write_failed", "error", writeErr.Error())
		}
		return
	}
	go s.dispatchObservers(context.Background(), resp.NormalizedEventID, env)
	w.Header().Set("X-Hitch-Event-ID", resp.EventID)
	w.Header().Set("X-Hitch-Normalized-Event-ID", resp.NormalizedEventID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(native); err != nil {
		log.emit(r.Context(), s.log, slog.LevelInfo, "api response write failed", http.StatusOK, "control_handler_count", len(result.Invocations), "native_response_bytes", len(native), "error_kind", "response_write_failed", "error", err.Error())
		return
	}
	log.emit(r.Context(), s.log, slog.LevelInfo, "api request completed", http.StatusOK, "control_handler_count", len(result.Invocations), "native_response_bytes", len(native), "sync_outcome", syncOutcome(result.Aggregate.Decision))
}
func (s *Server) ingest(ctx context.Context, r *http.Request) (EventResponse, protocol.EventEnvelope, EventRequest, string, error) {
	var req EventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return EventResponse{}, protocol.EventEnvelope{}, req, "", badRequest("invalid JSON: %v", err)
	}
	mode, err := normalizeRequestMode(req.Mode)
	if err != nil {
		return EventResponse{}, protocol.EventEnvelope{}, req, strings.TrimSpace(req.Mode), err
	}
	h := protocol.Harness(req.Harness)
	runtime := s.harnesses[h]
	if runtime.normalizer == nil {
		return EventResponse{}, protocol.EventEnvelope{}, req, mode, badRequest("unsupported harness %q", req.Harness)
	}
	if req.SourceEventType == "" {
		return EventResponse{}, protocol.EventEnvelope{}, req, mode, badRequest("source_event_type is required")
	}
	if len(req.SourcePayload) == 0 || !json.Valid(req.SourcePayload) {
		return EventResponse{}, protocol.EventEnvelope{}, req, mode, badRequest("source_payload must be valid JSON")
	}
	hitchEventTypes, ok := runtime.eventMap[req.SourceEventType]
	if !ok {
		if _, known := runtime.normalizer.KnownSourceEvents()[req.SourceEventType]; known {
			return EventResponse{}, protocol.EventEnvelope{}, req, mode, unmappedSourceEventError{harness: req.Harness, event: req.SourceEventType}
		}
		return EventResponse{}, protocol.EventEnvelope{}, req, mode, unknownSourceEventError{harness: req.Harness, event: req.SourceEventType}
	}
	if mode == requestModeSync && runtime.normalizer.Capability(req.SourceEventType) != harness.CapabilityControlCapable {
		return EventResponse{}, protocol.EventEnvelope{}, req, mode, badRequest("%s event %q does not support sync dispatch", req.Harness, req.SourceEventType)
	}
	env, err := runtime.normalizer.Normalize(req.SourceEventType, req.SourcePayload, hitchEventTypes[0])
	if err != nil {
		return EventResponse{}, protocol.EventEnvelope{}, req, mode, badRequest("%s", err.Error())
	}
	if env.Harness != h {
		return EventResponse{}, protocol.EventEnvelope{}, req, mode, badRequest("normalizer returned harness %q for request harness %q", env.Harness, h)
	}
	inboundID := harness.NewID("in")
	normalizedID := harness.NewID("norm")
	resp := EventResponse{EventID: env.EventID, NormalizedEventID: normalizedID}
	if err := s.store.InsertInbound(ctx, store.InboundEvent{ID: inboundID, ReceivedAt: env.ReceivedAt, Harness: env.Harness, SourceEventType: env.SourceEventType, SourcePayload: env.SourcePayload, RequestHeaders: protocol.Raw(headers(r)), HitchClientVersion: req.HitchClientVersion}); err != nil {
		return resp, env, req, mode, err
	}
	if err := s.store.InsertNormalized(ctx, store.NormalizedEvent{ID: normalizedID, InboundEventID: inboundID, HitchEventType: env.HitchEventType, Envelope: env, MappingVersion: protocol.Version}); err != nil {
		return resp, env, req, mode, err
	}
	for _, eventType := range hitchEventTypes[1:] {
		derived := env
		derived.EventID = harness.NewID("evt")
		derived.HitchEventType = eventType
		if err := s.store.InsertNormalized(ctx, store.NormalizedEvent{ID: harness.NewID("norm"), InboundEventID: inboundID, HitchEventType: derived.HitchEventType, Envelope: derived, MappingVersion: protocol.Version}); err != nil {
			return resp, env, req, mode, err
		}
	}
	return resp, env, req, mode, nil
}

func (s *Server) dispatchObservers(ctx context.Context, normalizedID string, env protocol.EventEnvelope) {
	started := time.Now()
	result := s.runner.Dispatch(ctx, env, "observer", 0)
	if len(result.Invocations) == 0 {
		return
	}
	persisted := 0
	for _, inv := range result.Invocations {
		if err := s.store.InsertHandlerInvocation(ctx, store.HandlerInvocation{ID: harness.NewID("hinv"), NormalizedEventID: normalizedID, HandlerName: inv.HandlerName, Kind: inv.Kind, StartedAt: inv.StartedAt, CompletedAt: inv.CompletedAt, Status: inv.Status, Stdout: inv.Stdout, Stderr: inv.Stderr, Output: inv.Output, Decision: inv.Decision, Error: inv.Error}); err != nil {
			s.log.Log(ctx, slog.LevelInfo, "observer dispatch failed",
				"normalized_event_id", normalizedID,
				"harness", string(env.Harness),
				"source_event_type", env.SourceEventType,
				"hitch_event_type", string(env.HitchEventType),
				"observer_handler_count", len(result.Invocations),
				"observer_handler_persisted_count", persisted,
				"duration_ms", time.Since(started).Milliseconds(),
				"error", err.Error(),
			)
			return
		}
		persisted++
	}
	attrs := []any{
		"normalized_event_id", normalizedID,
		"harness", string(env.Harness),
		"source_event_type", env.SourceEventType,
		"hitch_event_type", string(env.HitchEventType),
		"observer_handler_count", len(result.Invocations),
		"duration_ms", time.Since(started).Milliseconds(),
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
	s.log.Log(ctx, slog.LevelDebug, "observer dispatch completed", attrs...)
}

func (s *Server) handleGetEvent(w http.ResponseWriter, r *http.Request) {
	log := newAPIRequestLog(r)
	id := strings.TrimPrefix(r.URL.Path, "/v1/events/")
	log.add("normalized_event_id", id)
	if id == "" {
		err := badRequest("missing id")
		log.emit(r.Context(), s.log, slog.LevelInfo, "api request failed", http.StatusBadRequest, "error_kind", errorKind(err), "error", err.Error())
		if writeErr := writeError(w, err); writeErr != nil {
			log.emit(r.Context(), s.log, slog.LevelInfo, "api response write failed", http.StatusBadRequest, "error_kind", "response_write_failed", "error", writeErr.Error())
		}
		return
	}
	inspection, err := s.store.InspectEvent(r.Context(), id)
	if err != nil {
		log.emit(r.Context(), s.log, slog.LevelInfo, "api request failed", http.StatusInternalServerError, "error_kind", errorKind(err), "error", err.Error())
		if writeErr := writeError(w, err); writeErr != nil {
			log.emit(r.Context(), s.log, slog.LevelInfo, "api response write failed", http.StatusInternalServerError, "error_kind", "response_write_failed", "error", writeErr.Error())
		}
		return
	}
	if err := writeJSON(w, http.StatusOK, inspection); err != nil {
		log.emit(r.Context(), s.log, slog.LevelInfo, "api response write failed", http.StatusOK, "error_kind", "response_write_failed", "error", err.Error())
		return
	}
	log.emit(r.Context(), s.log, slog.LevelDebug, "api request completed", http.StatusOK)
}

func headers(r *http.Request) map[string][]string {
	out := make(map[string][]string, len(r.Header))
	for k, v := range r.Header {
		out[k] = v
	}
	return out
}

type unmappedSourceEventError struct {
	harness string
	event   string
}

func (e unmappedSourceEventError) Error() string {
	return fmt.Sprintf("unmapped %s event %q", e.harness, e.event)
}

type unknownSourceEventError struct {
	harness string
	event   string
}

func (e unknownSourceEventError) Error() string {
	return fmt.Sprintf("unknown %s event %q", e.harness, e.event)
}

func errorKind(err error) string {
	var unmapped unmappedSourceEventError
	if errors.As(err, &unmapped) {
		return "unmapped_source_event"
	}
	var unknown unknownSourceEventError
	if errors.As(err, &unknown) {
		return "unknown_source_event"
	}
	if isSQLiteBusy(err) {
		return "store_busy"
	}
	var he httpError
	if errors.As(err, &he) {
		return "bad_request"
	}
	return "internal_error"
}

func isSQLiteBusy(err error) bool {
	return strings.Contains(err.Error(), "SQLITE_BUSY") || strings.Contains(err.Error(), "database is locked")
}

type httpError struct {
	code int
	msg  string
}

func (e httpError) Error() string { return e.msg }
func badRequest(format string, args ...interface{}) error {
	return httpError{code: http.StatusBadRequest, msg: fmt.Sprintf(format, args...)}
}

func statusForError(err error) int {
	var he httpError
	if errors.As(err, &he) {
		return he.code
	}
	var unknown unknownSourceEventError
	if errors.As(err, &unknown) {
		return http.StatusBadRequest
	}
	var unmapped unmappedSourceEventError
	if errors.As(err, &unmapped) {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func writeError(w http.ResponseWriter, err error) error {
	return writeJSON(w, statusForError(err), map[string]interface{}{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	return json.NewEncoder(w).Encode(v)
}
