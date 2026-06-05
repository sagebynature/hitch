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
	"github.com/sagebynature/hitch/internal/harness/codex"
	"github.com/sagebynature/hitch/internal/harness/hermes"
	"github.com/sagebynature/hitch/internal/harness/omp"
	"github.com/sagebynature/hitch/internal/harness/pi"
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
	Harness            string           `json:"harness"`
	SourceEventType    string           `json:"source_event_type"`
	SourcePayload      protocol.RawJSON `json:"source_payload"`
	HitchClientVersion string           `json:"hitch_client_version"`
}

type EventResponse struct {
	EventID           string `json:"event_id"`
	NormalizedEventID string `json:"normalized_event_id"`
}

type DispatchResponse struct {
	EventResponse
	Aggregate      protocol.AggregateDecision `json:"aggregate"`
	NativeResponse protocol.RawJSON           `json:"native_response"`
}

func New(cfg config.Config, log *slog.Logger, st *store.Store) *Server {
	s := &Server{cfg: cfg, log: log, store: st, runner: dispatch.NewRunner(cfg.Handlers), harnesses: buildHarnessRuntimes(cfg)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", s.handleHealth)
	mux.HandleFunc("POST /v1/events", s.handleEvent)
	mux.HandleFunc("POST /v1/dispatch-sync", s.handleDispatchSync)
	mux.HandleFunc("GET /v1/events/", s.handleGetEvent)
	s.mux = mux
	return s
}
func buildHarnessRuntimes(cfg config.Config) map[protocol.Harness]harnessRuntime {
	return map[protocol.Harness]harnessRuntime{
		protocol.HarnessCodex:  {normalizer: codex.Mapper{}, eventMap: cfg.Harness.Codex.EventMap},
		protocol.HarnessHermes: {normalizer: hermes.Mapper{}, eventMap: cfg.Harness.Hermes.EventMap},
		protocol.HarnessPi:     {normalizer: pi.Mapper{}, eventMap: cfg.Harness.Pi.EventMap},
		protocol.HarnessOMP:    {normalizer: omp.Mapper{}, eventMap: cfg.Harness.OMP.EventMap},
	}
}

func (s *Server) Handler() http.Handler {
	return http.MaxBytesHandler(s.mux, s.cfg.Server.MaxRequestBytes)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
}

func (s *Server) handleEvent(w http.ResponseWriter, r *http.Request) {
	resp, _, err := s.ingest(r.Context(), r, false)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, resp)
}

func (s *Server) handleDispatchSync(w http.ResponseWriter, r *http.Request) {
	resp, env, err := s.ingest(r.Context(), r, true)
	if err != nil {
		writeError(w, err)
		return
	}
	result := s.runner.Dispatch(r.Context(), env, "sync", 2*time.Second)
	for _, inv := range result.Invocations {
		_ = s.store.InsertHandlerInvocation(r.Context(), store.HandlerInvocation{ID: harness.NewID("hinv"), NormalizedEventID: resp.NormalizedEventID, HandlerName: inv.HandlerName, Mode: inv.Mode, StartedAt: inv.StartedAt, CompletedAt: inv.CompletedAt, Status: inv.Status, Stdout: inv.Stdout, Stderr: inv.Stderr, Output: inv.Output, Decision: inv.Decision, Error: inv.Error})
	}
	runtime := s.harnesses[env.Harness]
	native, err := runtime.normalizer.Translate(env.SourceEventType, result.Aggregate)
	if err != nil {
		writeError(w, err)
		return
	}
	_ = s.store.InsertNativeResponse(r.Context(), store.NativeResponse{ID: harness.NewID("nresp"), NormalizedEventID: resp.NormalizedEventID, Response: native, EmittedAt: time.Now().UTC()})
	writeJSON(w, http.StatusOK, DispatchResponse{EventResponse: resp, Aggregate: result.Aggregate, NativeResponse: native})
}

func (s *Server) ingest(ctx context.Context, r *http.Request, sync bool) (EventResponse, protocol.EventEnvelope, error) {
	var req EventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return EventResponse{}, protocol.EventEnvelope{}, badRequest("invalid JSON: %v", err)
	}
	h := protocol.Harness(req.Harness)
	runtime := s.harnesses[h]
	if runtime.normalizer == nil {
		return EventResponse{}, protocol.EventEnvelope{}, badRequest("unsupported harness %q", req.Harness)
	}
	if req.SourceEventType == "" {
		return EventResponse{}, protocol.EventEnvelope{}, badRequest("source_event_type is required")
	}
	if len(req.SourcePayload) == 0 || !json.Valid(req.SourcePayload) {
		return EventResponse{}, protocol.EventEnvelope{}, badRequest("source_payload must be valid JSON")
	}
	hitchEventTypes, ok := runtime.eventMap[req.SourceEventType]
	if !ok {
		return EventResponse{}, protocol.EventEnvelope{}, badRequest("unsupported %s event %q", req.Harness, req.SourceEventType)
	}
	env, err := runtime.normalizer.Normalize(req.SourceEventType, req.SourcePayload, hitchEventTypes[0])
	if err != nil {
		return EventResponse{}, protocol.EventEnvelope{}, badRequest("%s", err.Error())
	}
	inboundID := harness.NewID("in")
	normalizedID := harness.NewID("norm")
	if err := s.store.InsertInbound(ctx, store.InboundEvent{ID: inboundID, ReceivedAt: env.ReceivedAt, Harness: env.Harness, SourceEventType: env.SourceEventType, SourcePayload: env.SourcePayload, RequestHeaders: protocol.Raw(headers(r)), HitchClientVersion: req.HitchClientVersion}); err != nil {
		return EventResponse{}, protocol.EventEnvelope{}, err
	}
	if err := s.store.InsertNormalized(ctx, store.NormalizedEvent{ID: normalizedID, InboundEventID: inboundID, HitchEventType: env.HitchEventType, Envelope: env, MappingVersion: protocol.Version}); err != nil {
		return EventResponse{}, protocol.EventEnvelope{}, err
	}
	for _, eventType := range hitchEventTypes[1:] {
		derived := env
		derived.EventID = harness.NewID("evt")
		derived.HitchEventType = eventType
		if err := s.store.InsertNormalized(ctx, store.NormalizedEvent{ID: harness.NewID("norm"), InboundEventID: inboundID, HitchEventType: derived.HitchEventType, Envelope: derived, MappingVersion: protocol.Version}); err != nil {
			return EventResponse{}, protocol.EventEnvelope{}, err
		}
	}
	if !sync {
		go s.dispatchAsync(context.Background(), normalizedID, env)
	}
	return EventResponse{EventID: env.EventID, NormalizedEventID: normalizedID}, env, nil
}

func (s *Server) dispatchAsync(ctx context.Context, normalizedID string, env protocol.EventEnvelope) {
	result := s.runner.Dispatch(ctx, env, "async", 0)
	for _, inv := range result.Invocations {
		_ = s.store.InsertHandlerInvocation(ctx, store.HandlerInvocation{ID: harness.NewID("hinv"), NormalizedEventID: normalizedID, HandlerName: inv.HandlerName, Mode: inv.Mode, StartedAt: inv.StartedAt, CompletedAt: inv.CompletedAt, Status: inv.Status, Stdout: inv.Stdout, Stderr: inv.Stderr, Output: inv.Output, Decision: inv.Decision, Error: inv.Error})
	}
}

func (s *Server) handleGetEvent(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/v1/events/")
	if id == "" {
		writeError(w, badRequest("missing id"))
		return
	}
	inspection, err := s.store.InspectEvent(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, inspection)
}

func headers(r *http.Request) map[string][]string {
	out := make(map[string][]string, len(r.Header))
	for k, v := range r.Header {
		out[k] = v
	}
	return out
}

type httpError struct {
	code int
	msg  string
}

func (e httpError) Error() string { return e.msg }
func badRequest(format string, args ...interface{}) error {
	return httpError{code: http.StatusBadRequest, msg: fmt.Sprintf(format, args...)}
}

func writeError(w http.ResponseWriter, err error) {
	var he httpError
	code := http.StatusInternalServerError
	if errors.As(err, &he) {
		code = he.code
	}
	writeJSON(w, code, map[string]interface{}{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
