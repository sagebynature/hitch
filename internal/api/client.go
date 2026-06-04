package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sagebynature/hitch/internal/protocol"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func (c Client) post(path string, req EventRequest, out interface{}) error {
	h := c.HTTP
	if h == nil {
		h = &http.Client{Timeout: 2 * time.Second}
	}
	b, err := json.Marshal(req)
	if err != nil {
		return err
	}
	resp, err := h.Post(c.BaseURL+path, "application/json", bytes.NewReader(b))
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

func (c Client) Event(req EventRequest) (EventResponse, error) {
	var out EventResponse
	err := c.post("/v1/events", req, &out)
	return out, err
}
func (c Client) Dispatch(req EventRequest) (DispatchResponse, error) {
	var out DispatchResponse
	err := c.post("/v1/dispatch-sync", req, &out)
	return out, err
}

func NewEventRequest(harness, event string, payload protocol.RawJSON) EventRequest {
	return EventRequest{Harness: harness, NativeEventType: event, NativePayload: payload, SourceAdapterVersion: protocol.Version}
}
