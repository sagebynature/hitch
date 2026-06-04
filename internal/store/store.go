package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/sage-scm/hitch/internal/protocol"
	_ "modernc.org/sqlite"
)

const schemaVersion = 1

var migrations = []string{`
CREATE TABLE IF NOT EXISTS schema_meta (
  version INTEGER NOT NULL
);
`, `
CREATE TABLE IF NOT EXISTS inbound_events (
  id TEXT PRIMARY KEY,
  received_at TEXT NOT NULL,
  harness TEXT NOT NULL,
  native_event_type TEXT NOT NULL,
  native_payload_json TEXT NOT NULL,
  request_headers_json TEXT,
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
  native_event_type TEXT NOT NULL,
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
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_meta`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		_, err := s.db.ExecContext(ctx, `INSERT INTO schema_meta(version) VALUES (?)`, schemaVersion)
		return err
	}
	return nil
}

type InboundEvent struct {
	ID                   string
	ReceivedAt           time.Time
	Harness              protocol.Harness
	NativeEventType      string
	NativePayload        protocol.RawJSON
	RequestHeaders       protocol.RawJSON
	SourceAdapterVersion string
}

type NormalizedEvent struct {
	ID             string
	InboundEventID string
	HitchEventType protocol.EventType
	Envelope       protocol.EventEnvelope
	MappingVersion string
}

type HandlerInvocation struct {
	ID                string
	NormalizedEventID string
	HandlerName       string
	Mode              string
	StartedAt         time.Time
	CompletedAt       time.Time
	Status            protocol.HandlerStatus
	Stdout            string
	Stderr            string
	Output            protocol.RawJSON
	Decision          protocol.RawJSON
	Error             string
	ReplaySourceID    string
}

type NativeResponse struct {
	ID                string
	NormalizedEventID string
	Harness           protocol.Harness
	NativeEventType   string
	Response          protocol.RawJSON
	EmittedAt         time.Time
}

func (s *Store) InsertInbound(ctx context.Context, e InboundEvent) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO inbound_events(id, received_at, harness, native_event_type, native_payload_json, request_headers_json, source_adapter_version, schema_version) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, e.ID, e.ReceivedAt.Format(time.RFC3339Nano), e.Harness, e.NativeEventType, string(e.NativePayload), string(e.RequestHeaders), e.SourceAdapterVersion, schemaVersion)
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
	_, err := s.db.ExecContext(ctx, `INSERT INTO native_responses(id, normalized_event_id, harness, native_event_type, response_json, emitted_at, schema_version) VALUES (?, ?, ?, ?, ?, ?, ?)`, r.ID, r.NormalizedEventID, r.Harness, r.NativeEventType, string(r.Response), r.EmittedAt.Format(time.RFC3339Nano), schemaVersion)
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
