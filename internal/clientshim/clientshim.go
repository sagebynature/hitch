package clientshim

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/sagebynature/hitch/internal/config"
	"github.com/sagebynature/hitch/internal/harness/codex"
	"github.com/sagebynature/hitch/internal/harness/hermes"
	"github.com/sagebynature/hitch/internal/harness/omp"
	"github.com/sagebynature/hitch/internal/harness/opencode"
	"github.com/sagebynature/hitch/internal/harness/pi"
	"github.com/sagebynature/hitch/internal/protocol"
)

type eventRequest struct {
	Harness            string           `json:"harness"`
	SourceEventType    string           `json:"source_event_type"`
	SourcePayload      protocol.RawJSON `json:"source_payload"`
	HitchClientVersion string           `json:"hitch_client_version"`
}

type eventResponse struct {
	EventID           string `json:"event_id"`
	NormalizedEventID string `json:"normalized_event_id"`
}

type dispatchResponse struct {
	eventResponse
	Aggregate      protocol.AggregateDecision `json:"aggregate"`
	NativeResponse protocol.RawJSON           `json:"native_response"`
}

type Options struct {
	Harness string
	Event   string
	Sync    bool
	URL     string
	Stdin   io.Reader
	Stdout  io.Writer
}

// Run reads one source hook payload, dispatches it to the local Hitch API, and writes only
// the harness-native response for synchronous dispatches.
func Run(ctx context.Context, opts Options) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if opts.Harness == "" || opts.Event == "" {
		return fmt.Errorf("-harness and -event are required")
	}
	stdin := opts.Stdin
	if stdin == nil {
		stdin = bytes.NewReader(nil)
	}
	stdout := opts.Stdout
	if stdout == nil {
		stdout = io.Discard
	}

	payload, err := io.ReadAll(stdin)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(payload)) == 0 {
		payload = []byte(`{}`)
	}
	if !json.Valid(payload) {
		return fmt.Errorf("stdin must be JSON")
	}

	baseURL := opts.URL
	if baseURL == "" {
		baseURL = DefaultURL()
	}
	client := httpClient{baseURL: baseURL}
	req := eventRequest{Harness: opts.Harness, SourceEventType: opts.Event, SourcePayload: protocol.RawJSON(payload), HitchClientVersion: protocol.Version}
	if !opts.Sync {
		var resp eventResponse
		_ = client.post(ctx, "/v1/events", req, &resp)
		return nil
	}

	var resp dispatchResponse
	err = client.post(ctx, "/v1/dispatch-sync", req, &resp)
	native := resp.NativeResponse
	if err != nil || len(native) == 0 {
		native = NativeNoop(opts.Harness, opts.Event)
	}
	if len(native) == 0 {
		return nil
	}
	if _, err := stdout.Write(native); err != nil {
		return err
	}
	_, err = stdout.Write([]byte("\n"))
	return err
}

type httpClient struct {
	baseURL string
	http    *http.Client
}

func (c httpClient) post(ctx context.Context, path string, req eventRequest, out interface{}) error {
	h := c.http
	if h == nil {
		h = &http.Client{Timeout: 2 * time.Second}
	}
	b, err := json.Marshal(req)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := h.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("hitch API %s: %s", resp.Status, string(body))
	}
	return json.Unmarshal(body, out)
}

// DefaultURL resolves the API endpoint used by hook shims.
func DefaultURL() string {
	if v := os.Getenv("HITCH_URL"); v != "" {
		return v
	}
	cfg, err := config.Load(config.DefaultPath)
	if err == nil && cfg.Server.Host != "" && cfg.Server.Port != 0 {
		return fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port)
	}
	return "http://127.0.0.1:8799"
}

// NativeNoop returns the harness-native fail-open response used when synchronous dispatch cannot reach Hitch.
func NativeNoop(harnessName, sourceEventType string) protocol.RawJSON {
	aggregate := protocol.AggregateDecision{Decision: protocol.Decision{Behavior: protocol.BehaviorNone}}
	switch protocol.Harness(harnessName) {
	case protocol.HarnessCodex:
		native, _ := codex.Mapper{}.Translate(sourceEventType, aggregate)
		return native
	case protocol.HarnessHermes:
		native, _ := hermes.Mapper{}.Translate(sourceEventType, aggregate)
		return native
	case protocol.HarnessPi:
		native, _ := pi.Mapper{}.Translate(sourceEventType, aggregate)
		return native
	case protocol.HarnessOMP:
		native, _ := omp.Mapper{}.Translate(sourceEventType, aggregate)
		return native
	case protocol.HarnessOpenCode:
		native, _ := opencode.Mapper{}.Translate(sourceEventType, aggregate)
		return native
	default:
		return nil
	}
}
