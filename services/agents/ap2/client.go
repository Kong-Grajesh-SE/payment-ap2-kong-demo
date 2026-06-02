package ap2

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client routes all agent-to-agent calls through the Kong mesh.
type Client struct {
	KongProxyURL string
	AgentDID     string
	HTTPClient   *http.Client
}

// NewClient creates a new AP2 mesh client.
func NewClient(kongProxyURL, agentDID string) *Client {
	return &Client{
		KongProxyURL: kongProxyURL,
		AgentDID:     agentDID,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CallAgent sends a JSON-RPC request to another agent via the Kong mesh.
func (c *Client) CallAgent(ctx context.Context, agentPath string, req *Request) (*Response, error) {
	// Inject _meta with this agent's DID
	if req.Meta == nil {
		req.Meta = &Meta{}
	}
	req.Meta.SenderDID = c.AgentDID

	// Extract trace context from context if available
	if traceID, ok := ctx.Value(traceIDKey).(string); ok {
		req.Meta.TraceID = traceID
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/agents/%s", c.KongProxyURL, agentPath)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Inject W3C traceparent header for OpenTelemetry propagation
	if traceID, ok := ctx.Value(traceIDKey).(string); ok {
		httpReq.Header.Set("traceparent", fmt.Sprintf("00-%s-0000000000000001-01", traceID))
	}

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("agent call failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var rpcResp Response
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &rpcResp, nil
}

type contextKey string

const traceIDKey contextKey = "trace_id"

// WithTraceID creates a context with the given trace ID.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey, traceID)
}
