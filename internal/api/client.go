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

func (c Client) postRaw(path string, req EventRequest) (protocol.RawJSON, error) {
	h := c.HTTP
	if h == nil {
		h = &http.Client{Timeout: 2 * time.Second}
	}
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	resp, err := h.Post(c.BaseURL+path, "application/json", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("hitch API %s: %s", resp.Status, string(body))
	}
	return protocol.RawJSON(body), nil
}

func (c Client) Event(req EventRequest) (EventResponse, error) {
	req.Mode = requestModeAsync
	var out EventResponse
	err := c.post("/v1/events", req, &out)
	return out, err
}
func (c Client) Dispatch(req EventRequest) (protocol.RawJSON, error) {
	req.Mode = requestModeSync
	return c.postRaw("/v1/events", req)
}

func NewEventRequest(harness, event string, payload protocol.RawJSON) EventRequest {
	return EventRequest{Harness: harness, SourceEventType: event, SourcePayload: payload, HitchClientVersion: protocol.Version}
}
