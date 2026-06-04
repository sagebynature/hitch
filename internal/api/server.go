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
	cfg     config.Config
	log     *slog.Logger
	store   *store.Store
	runner  dispatch.Runner
	mux     *http.ServeMux
	mappers map[protocol.Harness]harness.Mapper
}

type EventRequest struct {
	Harness              string           `json:"harness"`
	HarnessVersion       string           `json:"harness_version,omitempty"`
	NativeEventType      string           `json:"native_event_type"`
	NativePayload        protocol.RawJSON `json:"native_payload"`
	SourceAdapterVersion string           `json:"source_adapter_version,omitempty"`
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
	s := &Server{cfg: cfg, log: log, store: st, runner: dispatch.NewRunner(cfg.Handlers), mappers: map[protocol.Harness]harness.Mapper{protocol.HarnessCodex: codex.Mapper{}, protocol.HarnessHermes: hermes.Mapper{}, protocol.HarnessPi: pi.Mapper{}, protocol.HarnessOMP: omp.Mapper{}}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", s.handleHealth)
	mux.HandleFunc("POST /v1/events", s.handleEvent)
	mux.HandleFunc("POST /v1/dispatch-sync", s.handleDispatchSync)
	mux.HandleFunc("GET /v1/events/", s.handleGetEvent)
	s.mux = mux
	return s
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
	mapper := s.mappers[env.Harness]
	native, err := mapper.Translate(env.NativeEventType, result.Aggregate)
	if err != nil {
		writeError(w, err)
		return
	}
	_ = s.store.InsertNativeResponse(r.Context(), store.NativeResponse{ID: harness.NewID("nresp"), NormalizedEventID: resp.NormalizedEventID, Harness: env.Harness, NativeEventType: env.NativeEventType, Response: native, EmittedAt: time.Now().UTC()})
	writeJSON(w, http.StatusOK, DispatchResponse{EventResponse: resp, Aggregate: result.Aggregate, NativeResponse: native})
}

func (s *Server) ingest(ctx context.Context, r *http.Request, sync bool) (EventResponse, protocol.EventEnvelope, error) {
	var req EventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return EventResponse{}, protocol.EventEnvelope{}, badRequest("invalid JSON: %v", err)
	}
	h := protocol.Harness(req.Harness)
	mapper := s.mappers[h]
	if mapper == nil {
		return EventResponse{}, protocol.EventEnvelope{}, badRequest("unsupported harness %q", req.Harness)
	}
	if len(req.NativePayload) == 0 || !json.Valid(req.NativePayload) {
		return EventResponse{}, protocol.EventEnvelope{}, badRequest("native_payload must be valid JSON")
	}
	env, err := mapper.Map(req.NativeEventType, req.NativePayload)
	if err != nil {
		return EventResponse{}, protocol.EventEnvelope{}, badRequest("%s", err.Error())
	}
	env.HarnessVersion = req.HarnessVersion
	inboundID := harness.NewID("in")
	normalizedID := harness.NewID("norm")
	if err := s.store.InsertInbound(ctx, store.InboundEvent{ID: inboundID, ReceivedAt: env.ReceivedAt, Harness: env.Harness, NativeEventType: env.NativeEventType, NativePayload: env.NativePayload, RequestHeaders: protocol.Raw(headers(r)), SourceAdapterVersion: req.SourceAdapterVersion}); err != nil {
		return EventResponse{}, protocol.EventEnvelope{}, err
	}
	if err := s.store.InsertNormalized(ctx, store.NormalizedEvent{ID: normalizedID, InboundEventID: inboundID, HitchEventType: env.HitchEventType, Envelope: env, MappingVersion: protocol.Version}); err != nil {
		return EventResponse{}, protocol.EventEnvelope{}, err
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
	env, err := s.store.GetEvent(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, env)
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
