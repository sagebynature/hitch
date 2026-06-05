package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/sagebynature/hitch/internal/protocol"
	_ "modernc.org/sqlite"
)

const schemaVersion = 2

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
  source_payload_json TEXT NOT NULL,
  request_headers_json TEXT,
  hitch_client_version TEXT,
  native_event_type TEXT,
  native_payload_json TEXT,
  source_adapter_version TEXT,
  schema_version INTEGER NOT NULL
);
`, `
CREATE TABLE IF NOT EXISTS normalized_events (
  id TEXT PRIMARY KEY,
  inbound_event_id TEXT NOT NULL REFERENCES inbound_events(id),
  hitch_event_type TEXT NOT NULL,
  normalized_payload_json TEXT NOT NULL,
  mapping_version TEXT NOT NULL,
  schema_version INTEGER NOT NULL
);
`, `
CREATE TABLE IF NOT EXISTS handler_invocations (
  id TEXT PRIMARY KEY,
  normalized_event_id TEXT NOT NULL REFERENCES normalized_events(id),
  handler_name TEXT NOT NULL,
  mode TEXT NOT NULL,
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
CREATE TABLE IF NOT EXISTS native_responses (
  id TEXT PRIMARY KEY,
  normalized_event_id TEXT NOT NULL REFERENCES normalized_events(id),
  harness TEXT NOT NULL,
  source_event_type TEXT NOT NULL,
  native_event_type TEXT,
  response_json TEXT NOT NULL,
  emitted_at TEXT NOT NULL,
  schema_version INTEGER NOT NULL
);
`}

type Store struct{ db *sql.DB }

func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
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

func (s *Store) Migrate(ctx context.Context) error {
	for _, m := range migrations {
		if _, err := s.db.ExecContext(ctx, m); err != nil {
			return err
		}
	}
	if err := s.ensureSourceColumns(ctx); err != nil {
		return err
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_meta`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		_, err := s.db.ExecContext(ctx, `INSERT INTO schema_meta(version) VALUES (?)`, schemaVersion)
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE schema_meta SET version = ?`, schemaVersion)
	return err
}

func (s *Store) ensureSourceColumns(ctx context.Context) error {
	if ok, err := s.columnExists(ctx, "inbound_events", "source_event_type"); err != nil {
		return err
	} else if !ok {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE inbound_events ADD COLUMN source_event_type TEXT`); err != nil {
			return err
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE inbound_events SET source_event_type = native_event_type WHERE source_event_type IS NULL AND native_event_type IS NOT NULL`); err != nil {
			return err
		}
	}
	if ok, err := s.columnExists(ctx, "inbound_events", "source_payload_json"); err != nil {
		return err
	} else if !ok {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE inbound_events ADD COLUMN source_payload_json TEXT`); err != nil {
			return err
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE inbound_events SET source_payload_json = native_payload_json WHERE source_payload_json IS NULL AND native_payload_json IS NOT NULL`); err != nil {
			return err
		}
	}
	if ok, err := s.columnExists(ctx, "inbound_events", "hitch_client_version"); err != nil {
		return err
	} else if !ok {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE inbound_events ADD COLUMN hitch_client_version TEXT`); err != nil {
			return err
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE inbound_events SET hitch_client_version = source_adapter_version WHERE hitch_client_version IS NULL AND source_adapter_version IS NOT NULL`); err != nil {
			return err
		}
	}
	if ok, err := s.columnExists(ctx, "native_responses", "source_event_type"); err != nil {
		return err
	} else if !ok {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE native_responses ADD COLUMN source_event_type TEXT`); err != nil {
			return err
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE native_responses SET source_event_type = native_event_type WHERE source_event_type IS NULL AND native_event_type IS NOT NULL`); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) columnExists(ctx context.Context, table, column string) (bool, error) {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
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
	NormalizedEventID string                 `json:"normalized_event_id"`
	HandlerName       string                 `json:"handler_name"`
	Mode              string                 `json:"mode"`
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

type NativeResponse struct {
	ID                string           `json:"id"`
	NormalizedEventID string           `json:"normalized_event_id"`
	Harness           protocol.Harness `json:"harness"`
	SourceEventType   string           `json:"source_event_type"`
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
	_, err := s.db.ExecContext(ctx, `INSERT INTO inbound_events(id, received_at, harness, source_event_type, source_payload_json, request_headers_json, hitch_client_version, native_event_type, native_payload_json, source_adapter_version, schema_version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, e.ID, e.ReceivedAt.Format(time.RFC3339Nano), e.Harness, e.SourceEventType, string(e.SourcePayload), string(e.RequestHeaders), e.HitchClientVersion, e.SourceEventType, string(e.SourcePayload), e.HitchClientVersion, schemaVersion)
	return err
}

func (s *Store) InsertNormalized(ctx context.Context, e NormalizedEvent) error {
	b, err := json.Marshal(e.Envelope)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO normalized_events(id, inbound_event_id, hitch_event_type, normalized_payload_json, mapping_version, schema_version) VALUES (?, ?, ?, ?, ?, ?)`, e.ID, e.InboundEventID, e.HitchEventType, string(b), e.MappingVersion, schemaVersion)
	return err
}

func (s *Store) InsertHandlerInvocation(ctx context.Context, h HandlerInvocation) error {
	completed := ""
	if !h.CompletedAt.IsZero() {
		completed = h.CompletedAt.Format(time.RFC3339Nano)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO handler_invocations(id, normalized_event_id, handler_name, mode, started_at, completed_at, status, stdout, stderr, output_json, decision_json, error, replay_source_id, schema_version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, h.ID, h.NormalizedEventID, h.HandlerName, h.Mode, h.StartedAt.Format(time.RFC3339Nano), completed, h.Status, h.Stdout, h.Stderr, string(h.Output), string(h.Decision), h.Error, h.ReplaySourceID, schemaVersion)
	return err
}

func (s *Store) InsertNativeResponse(ctx context.Context, r NativeResponse) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO native_responses(id, normalized_event_id, harness, source_event_type, native_event_type, response_json, emitted_at, schema_version) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, r.ID, r.NormalizedEventID, r.Harness, r.SourceEventType, r.SourceEventType, string(r.Response), r.EmittedAt.Format(time.RFC3339Nano), schemaVersion)
	return err
}

func (s *Store) GetEvent(ctx context.Context, id string) (protocol.EventEnvelope, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT normalized_payload_json FROM normalized_events WHERE id = ?`, id).Scan(&raw)
	if err != nil {
		return protocol.EventEnvelope{}, err
	}
	var e protocol.EventEnvelope
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		return protocol.EventEnvelope{}, err
	}
	return e, nil
}

func (s *Store) InspectEvent(ctx context.Context, id string) (EventInspection, error) {
	var out EventInspection
	var inboundPayloadRaw, requestHeadersRaw, normalizedRaw string
	var receivedAt string

	err := s.db.QueryRowContext(ctx, `
SELECT i.id, i.received_at, i.harness, i.source_event_type, i.source_payload_json, i.request_headers_json, i.hitch_client_version,
       n.id, n.inbound_event_id, n.hitch_event_type, n.normalized_payload_json, n.mapping_version
FROM normalized_events n
JOIN inbound_events i ON i.id = n.inbound_event_id
WHERE n.id = ?`, id).Scan(
		&out.Inbound.ID, &receivedAt, &out.Inbound.Harness, &out.Inbound.SourceEventType, &inboundPayloadRaw, &requestHeadersRaw, &out.Inbound.HitchClientVersion,
		&out.Normalized.ID, &out.Normalized.InboundEventID, &out.Normalized.HitchEventType, &normalizedRaw, &out.Normalized.MappingVersion,
	)
	if err != nil {
		return EventInspection{}, err
	}
	out.Inbound.ReceivedAt, err = time.Parse(time.RFC3339Nano, receivedAt)
	if err != nil {
		return EventInspection{}, err
	}
	out.Inbound.SourcePayload = protocol.RawJSON(inboundPayloadRaw)
	if requestHeadersRaw != "" {
		out.Inbound.RequestHeaders = protocol.RawJSON(requestHeadersRaw)
	}
	if err := json.Unmarshal([]byte(normalizedRaw), &out.Normalized.Envelope); err != nil {
		return EventInspection{}, err
	}

	rows, err := s.db.QueryContext(ctx, `SELECT id, normalized_event_id, handler_name, mode, started_at, completed_at, status, stdout, stderr, output_json, decision_json, error, replay_source_id FROM handler_invocations WHERE normalized_event_id = ? ORDER BY started_at, id`, id)
	if err != nil {
		return EventInspection{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var h HandlerInvocation
		var startedAt, completedAt, outputRaw, decisionRaw string
		if err := rows.Scan(&h.ID, &h.NormalizedEventID, &h.HandlerName, &h.Mode, &startedAt, &completedAt, &h.Status, &h.Stdout, &h.Stderr, &outputRaw, &decisionRaw, &h.Error, &h.ReplaySourceID); err != nil {
			return EventInspection{}, err
		}
		h.StartedAt, err = time.Parse(time.RFC3339Nano, startedAt)
		if err != nil {
			return EventInspection{}, err
		}
		if completedAt != "" {
			h.CompletedAt, err = time.Parse(time.RFC3339Nano, completedAt)
			if err != nil {
				return EventInspection{}, err
			}
		}
		if outputRaw != "" {
			h.Output = protocol.RawJSON(outputRaw)
		}
		if decisionRaw != "" {
			h.Decision = protocol.RawJSON(decisionRaw)
		}
		out.HandlerInvocations = append(out.HandlerInvocations, h)
	}
	if err := rows.Err(); err != nil {
		return EventInspection{}, err
	}

	responseRows, err := s.db.QueryContext(ctx, `SELECT id, normalized_event_id, harness, source_event_type, response_json, emitted_at FROM native_responses WHERE normalized_event_id = ? ORDER BY emitted_at, id`, id)
	if err != nil {
		return EventInspection{}, err
	}
	defer responseRows.Close()
	for responseRows.Next() {
		var r NativeResponse
		var emittedAt, responseRaw string
		if err := responseRows.Scan(&r.ID, &r.NormalizedEventID, &r.Harness, &r.SourceEventType, &responseRaw, &emittedAt); err != nil {
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
