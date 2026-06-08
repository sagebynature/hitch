package store

import (
	"context"
	"database/sql"
	"net/url"
	"strings"
	"time"

	"github.com/sagebynature/hitch/internal/protocol"
	_ "modernc.org/sqlite"
)

const schemaVersion = 6

var migrations = []string{`
CREATE TABLE IF NOT EXISTS schema_meta (
  version INTEGER NOT NULL
);
`, `
CREATE TABLE IF NOT EXISTS inbound_events (
  id TEXT PRIMARY KEY,
  received_at TEXT NOT NULL,
  harness TEXT NOT NULL,
  source_event_type TEXT NOT NULL,
  source_payload TEXT NOT NULL CHECK (json_valid(source_payload)),
  hitch_client_version TEXT,
  request_headers TEXT CHECK (request_headers IS NULL OR json_valid(request_headers)),
  schema_version INTEGER NOT NULL
);
`, `
CREATE TABLE IF NOT EXISTS normalized_events (
  id TEXT PRIMARY KEY,
  hitch_version TEXT NOT NULL,
  event_id TEXT NOT NULL,
  received_at TEXT NOT NULL,
  harness TEXT NOT NULL,
  source_event_type TEXT NOT NULL,
  source_payload TEXT NOT NULL CHECK (json_valid(source_payload)),
  hitch_event_type TEXT NOT NULL,
  session_id TEXT,
  turn_id TEXT,
  cwd TEXT,
  model TEXT,
  transcript_path TEXT,
  payload TEXT NOT NULL CHECK (json_valid(payload)),
  inbound_event_id TEXT NOT NULL REFERENCES inbound_events(id),
  mapping_version TEXT NOT NULL,
  schema_version INTEGER NOT NULL
);
`, `
CREATE TABLE IF NOT EXISTS handler_invocations (
  id TEXT PRIMARY KEY,
  normalized_event_id TEXT NOT NULL REFERENCES normalized_events(id),
  inbound_event_id TEXT NOT NULL REFERENCES inbound_events(id),
  handler_name TEXT NOT NULL,
  kind TEXT NOT NULL,
  hook_key TEXT NOT NULL,
  started_at TEXT NOT NULL,
  completed_at TEXT,
  status TEXT NOT NULL,
  stdout TEXT,
  stderr TEXT,
  output_json TEXT,
  decision_json TEXT,
  error TEXT,
  replay_source_id TEXT,
  schema_version INTEGER NOT NULL
);
`, `
CREATE UNIQUE INDEX IF NOT EXISTS idx_handler_invocations_dedupe
ON handler_invocations(inbound_event_id, handler_name, hook_key);
`, `
CREATE TABLE IF NOT EXISTS native_responses (
  id TEXT PRIMARY KEY,
  normalized_event_id TEXT NOT NULL REFERENCES normalized_events(id),
  response_json TEXT NOT NULL CHECK (json_valid(response_json)),
  emitted_at TEXT NOT NULL,
  schema_version INTEGER NOT NULL
);
`}

type Store struct{ db *sql.DB }

func sqliteDSN(path string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + "_pragma=" + url.QueryEscape("busy_timeout(5000)") + "&_pragma=" + url.QueryEscape("journal_mode(WAL)")
}

func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.Migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) RawConn(ctx context.Context) (*sql.Conn, error) {
	return s.db.Conn(ctx)
}

func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, migrations[0]); err != nil {
		return err
	}
	var version int
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_meta`).Scan(&version); err != nil {
		return err
	}
	if version != schemaVersion {
		if err := s.resetSchema(ctx); err != nil {
			return err
		}
	}
	for _, m := range migrations {
		if _, err := s.db.ExecContext(ctx, m); err != nil {
			return err
		}
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM schema_meta`); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO schema_meta(version) VALUES (?)`, schemaVersion)
	return err
}

func (s *Store) resetSchema(ctx context.Context) error {
	for _, table := range []string{"handler_invocations", "native_responses", "normalized_events", "inbound_events", "schema_meta"} {
		if _, err := s.db.ExecContext(ctx, `DROP TABLE IF EXISTS `+table); err != nil {
			return err
		}
	}
	return nil
}

type InboundEvent struct {
	ID                 string           `json:"id"`
	ReceivedAt         time.Time        `json:"received_at"`
	Harness            protocol.Harness `json:"harness"`
	SourceEventType    string           `json:"source_event_type"`
	SourcePayload      protocol.RawJSON `json:"source_payload"`
	RequestHeaders     protocol.RawJSON `json:"request_headers,omitempty"`
	HitchClientVersion string           `json:"hitch_client_version"`
}

type NormalizedEvent struct {
	ID             string                 `json:"id"`
	InboundEventID string                 `json:"inbound_event_id"`
	HitchEventType protocol.EventType     `json:"hitch_event_type"`
	Envelope       protocol.EventEnvelope `json:"envelope"`
	MappingVersion string                 `json:"mapping_version"`
}

type HandlerInvocation struct {
	ID                string                 `json:"id"`
	InboundEventID    string                 `json:"inbound_event_id"`
	NormalizedEventID string                 `json:"normalized_event_id"`
	HandlerName       string                 `json:"handler_name"`
	Kind              string                 `json:"kind"`
	HookKey           string                 `json:"hook_key"`
	StartedAt         time.Time              `json:"started_at"`
	CompletedAt       time.Time              `json:"completed_at,omitempty"`
	Status            protocol.HandlerStatus `json:"status"`
	Stdout            string                 `json:"stdout,omitempty"`
	Stderr            string                 `json:"stderr,omitempty"`
	Output            protocol.RawJSON       `json:"output,omitempty"`
	Decision          protocol.RawJSON       `json:"decision,omitempty"`
	Error             string                 `json:"error,omitempty"`
	ReplaySourceID    string                 `json:"replay_source_id,omitempty"`
}

type HandlerInvocationReservation struct {
	ID                string
	InboundEventID    string
	NormalizedEventID string
	HandlerName       string
	Kind              string
	HookKey           string
	StartedAt         time.Time
}

type NativeResponse struct {
	ID                string           `json:"id"`
	NormalizedEventID string           `json:"normalized_event_id"`
	Response          protocol.RawJSON `json:"response"`
	EmittedAt         time.Time        `json:"emitted_at"`
}

type EventInspection struct {
	Inbound            InboundEvent        `json:"inbound"`
	Normalized         NormalizedEvent     `json:"normalized"`
	HandlerInvocations []HandlerInvocation `json:"handler_invocations"`
	NativeResponses    []NativeResponse    `json:"native_responses"`
}

func (s *Store) InsertInbound(ctx context.Context, e InboundEvent) error {
	var requestHeaders interface{}
	if len(e.RequestHeaders) != 0 {
		requestHeaders = string(e.RequestHeaders)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO inbound_events(id, received_at, harness, source_event_type, source_payload, hitch_client_version, request_headers, schema_version) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, e.ID, e.ReceivedAt.Format(time.RFC3339Nano), e.Harness, e.SourceEventType, string(e.SourcePayload), e.HitchClientVersion, requestHeaders, schemaVersion)
	return err
}

func (s *Store) InsertNormalized(ctx context.Context, e NormalizedEvent) error {
	env := e.Envelope
	_, err := s.db.ExecContext(ctx, `INSERT INTO normalized_events(id, hitch_version, event_id, received_at, harness, source_event_type, source_payload, hitch_event_type, session_id, turn_id, cwd, model, transcript_path, payload, inbound_event_id, mapping_version, schema_version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, e.ID, env.HitchVersion, env.EventID, env.ReceivedAt.Format(time.RFC3339Nano), env.Harness, env.SourceEventType, string(env.SourcePayload), env.HitchEventType, env.SessionID, env.TurnID, env.CWD, env.Model, env.TranscriptPath, string(env.Payload), e.InboundEventID, e.MappingVersion, schemaVersion)
	return err
}

func (s *Store) InsertHandlerInvocation(ctx context.Context, h HandlerInvocation) error {
	completed := ""
	if !h.CompletedAt.IsZero() {
		completed = h.CompletedAt.Format(time.RFC3339Nano)
	}
	if h.InboundEventID == "" || h.HookKey == "" {
		var err error
		h, err = s.withLegacyInvocationDedupeFields(ctx, h)
		if err != nil {
			return err
		}
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO handler_invocations(id, inbound_event_id, normalized_event_id, handler_name, kind, hook_key, started_at, completed_at, status, stdout, stderr, output_json, decision_json, error, replay_source_id, schema_version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, h.ID, h.InboundEventID, h.NormalizedEventID, h.HandlerName, h.Kind, h.HookKey, h.StartedAt.Format(time.RFC3339Nano), completed, h.Status, h.Stdout, h.Stderr, string(h.Output), string(h.Decision), h.Error, h.ReplaySourceID, schemaVersion)
	return err
}

func (s *Store) ReserveHandlerInvocation(ctx context.Context, r HandlerInvocationReservation) (bool, error) {
	_, err := s.db.ExecContext(ctx, `INSERT INTO handler_invocations(id, inbound_event_id, normalized_event_id, handler_name, kind, hook_key, started_at, status, schema_version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, r.ID, r.InboundEventID, r.NormalizedEventID, r.HandlerName, r.Kind, r.HookKey, r.StartedAt.Format(time.RFC3339Nano), protocol.StatusReserved, schemaVersion)
	if err != nil {
		if isHandlerInvocationDedupeConflict(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func isHandlerInvocationDedupeConflict(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed: handler_invocations.inbound_event_id, handler_invocations.handler_name, handler_invocations.hook_key")
}

func (s *Store) CompleteHandlerInvocation(ctx context.Context, h HandlerInvocation) error {
	completed := ""
	if !h.CompletedAt.IsZero() {
		completed = h.CompletedAt.Format(time.RFC3339Nano)
	}
	result, err := s.db.ExecContext(ctx, `UPDATE handler_invocations SET completed_at = ?, status = ?, stdout = ?, stderr = ?, output_json = ?, decision_json = ?, error = ?, replay_source_id = ? WHERE id = ?`, completed, h.Status, h.Stdout, h.Stderr, string(h.Output), string(h.Decision), h.Error, h.ReplaySourceID, h.ID)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) withLegacyInvocationDedupeFields(ctx context.Context, h HandlerInvocation) (HandlerInvocation, error) {
	if h.InboundEventID == "" {
		if err := s.db.QueryRowContext(ctx, `SELECT inbound_event_id FROM normalized_events WHERE id = ?`, h.NormalizedEventID).Scan(&h.InboundEventID); err != nil {
			return HandlerInvocation{}, err
		}
	}
	if h.HookKey == "" {
		if h.ReplaySourceID != "" {
			h.HookKey = strings.Join([]string{"legacy", "replay", h.ReplaySourceID, h.HandlerName, h.Kind}, ":")
		} else {
			h.HookKey = strings.Join([]string{"legacy", h.NormalizedEventID, h.HandlerName, h.Kind}, ":")
		}
	}
	return h, nil
}

func (s *Store) InsertNativeResponse(ctx context.Context, r NativeResponse) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO native_responses(id, normalized_event_id, response_json, emitted_at, schema_version) VALUES (?, ?, ?, ?, ?)`, r.ID, r.NormalizedEventID, string(r.Response), r.EmittedAt.Format(time.RFC3339Nano), schemaVersion)
	return err
}

func (s *Store) GetEvent(ctx context.Context, id string) (protocol.EventEnvelope, error) {
	var e protocol.EventEnvelope
	var receivedAt, sourcePayloadRaw, payloadRaw string
	err := s.db.QueryRowContext(ctx, `SELECT hitch_version, event_id, received_at, harness, source_event_type, source_payload, hitch_event_type, session_id, turn_id, cwd, model, transcript_path, payload FROM normalized_events WHERE id = ?`, id).Scan(&e.HitchVersion, &e.EventID, &receivedAt, &e.Harness, &e.SourceEventType, &sourcePayloadRaw, &e.HitchEventType, &e.SessionID, &e.TurnID, &e.CWD, &e.Model, &e.TranscriptPath, &payloadRaw)
	if err != nil {
		return protocol.EventEnvelope{}, err
	}
	e.ReceivedAt, err = time.Parse(time.RFC3339Nano, receivedAt)
	if err != nil {
		return protocol.EventEnvelope{}, err
	}
	e.SourcePayload = protocol.RawJSON(sourcePayloadRaw)
	e.Payload = protocol.RawJSON(payloadRaw)
	return e, nil
}

func (s *Store) InspectEvent(ctx context.Context, id string) (EventInspection, error) {
	var out EventInspection
	var requestHeadersRaw sql.NullString
	var inboundPayloadRaw string
	var receivedAt string

	err := s.db.QueryRowContext(ctx, `
SELECT i.id, i.received_at, i.harness, i.source_event_type, i.source_payload, i.request_headers, i.hitch_client_version,
       n.id, n.inbound_event_id, n.hitch_event_type, n.mapping_version
FROM normalized_events n
JOIN inbound_events i ON i.id = n.inbound_event_id
WHERE n.id = ?`, id).Scan(
		&out.Inbound.ID, &receivedAt, &out.Inbound.Harness, &out.Inbound.SourceEventType, &inboundPayloadRaw, &requestHeadersRaw, &out.Inbound.HitchClientVersion,
		&out.Normalized.ID, &out.Normalized.InboundEventID, &out.Normalized.HitchEventType, &out.Normalized.MappingVersion,
	)
	if err != nil {
		return EventInspection{}, err
	}
	out.Inbound.ReceivedAt, err = time.Parse(time.RFC3339Nano, receivedAt)
	if err != nil {
		return EventInspection{}, err
	}
	out.Inbound.SourcePayload = protocol.RawJSON(inboundPayloadRaw)
	if requestHeadersRaw.Valid {
		out.Inbound.RequestHeaders = protocol.RawJSON(requestHeadersRaw.String)
	}
	out.Normalized.Envelope, err = s.GetEvent(ctx, id)
	if err != nil {
		return EventInspection{}, err
	}

	rows, err := s.db.QueryContext(ctx, `SELECT id, inbound_event_id, normalized_event_id, handler_name, kind, hook_key, started_at, completed_at, status, stdout, stderr, output_json, decision_json, error, replay_source_id FROM handler_invocations WHERE normalized_event_id = ? ORDER BY started_at, id`, id)
	if err != nil {
		return EventInspection{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var h HandlerInvocation
		var startedAt string
		var completedAt, stdout, stderr, outputRaw, decisionRaw, handlerError, replaySourceID sql.NullString
		if err := rows.Scan(&h.ID, &h.InboundEventID, &h.NormalizedEventID, &h.HandlerName, &h.Kind, &h.HookKey, &startedAt, &completedAt, &h.Status, &stdout, &stderr, &outputRaw, &decisionRaw, &handlerError, &replaySourceID); err != nil {
			return EventInspection{}, err
		}
		h.StartedAt, err = time.Parse(time.RFC3339Nano, startedAt)
		if err != nil {
			return EventInspection{}, err
		}
		if completedAt.Valid && completedAt.String != "" {
			h.CompletedAt, err = time.Parse(time.RFC3339Nano, completedAt.String)
			if err != nil {
				return EventInspection{}, err
			}
		}
		if stdout.Valid {
			h.Stdout = stdout.String
		}
		if stderr.Valid {
			h.Stderr = stderr.String
		}
		if outputRaw.Valid && outputRaw.String != "" {
			h.Output = protocol.RawJSON(outputRaw.String)
		}
		if decisionRaw.Valid && decisionRaw.String != "" {
			h.Decision = protocol.RawJSON(decisionRaw.String)
		}
		if handlerError.Valid {
			h.Error = handlerError.String
		}
		if replaySourceID.Valid {
			h.ReplaySourceID = replaySourceID.String
		}
		out.HandlerInvocations = append(out.HandlerInvocations, h)
	}
	if err := rows.Err(); err != nil {
		return EventInspection{}, err
	}

	responseRows, err := s.db.QueryContext(ctx, `SELECT id, normalized_event_id, response_json, emitted_at FROM native_responses WHERE normalized_event_id = ? ORDER BY emitted_at, id`, id)
	if err != nil {
		return EventInspection{}, err
	}
	defer responseRows.Close()
	for responseRows.Next() {
		var r NativeResponse
		var emittedAt, responseRaw string
		if err := responseRows.Scan(&r.ID, &r.NormalizedEventID, &responseRaw, &emittedAt); err != nil {
			return EventInspection{}, err
		}
		r.EmittedAt, err = time.Parse(time.RFC3339Nano, emittedAt)
		if err != nil {
			return EventInspection{}, err
		}
		r.Response = protocol.RawJSON(responseRaw)
		out.NativeResponses = append(out.NativeResponses, r)
	}
	if err := responseRows.Err(); err != nil {
		return EventInspection{}, err
	}
	return out, nil
}

func (s *Store) LatestEventIDByType(ctx context.Context, eventType protocol.EventType) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM normalized_events WHERE hitch_event_type = ? ORDER BY rowid DESC LIMIT 1`, eventType).Scan(&id)
	return id, err
}
