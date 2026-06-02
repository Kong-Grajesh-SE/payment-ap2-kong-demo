package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Kong/go-pdk"
)

// Config holds the plugin configuration.
type Config struct {
	WORMStorageURL string `json:"worm_storage_url"`
}

// New creates a new plugin instance.
func New() interface{} {
	return &Config{
		WORMStorageURL: "http://worm-storage:8090",
	}
}

// auditRecord represents what gets written to WORM storage.
type auditRecord struct {
	TraceID        string                 `json:"trace_id"`
	SpanID         string                 `json:"span_id"`
	SenderDID      string                 `json:"sender_did"`
	ReceiverDID    string                 `json:"receiver_did,omitempty"`
	JSONRPCMethod  string                 `json:"jsonrpc_method"`
	MandateType    string                 `json:"mandate_type,omitempty"`
	MandatePayload map[string]interface{} `json:"mandate_payload,omitempty"`
	KongVerified   bool                   `json:"kong_verified"`
	TrustScore     int                    `json:"trust_score"`
}

// Access captures request body data for the Log phase.
// GetRawBody() is only available in rewrite/access phases, not log.
// Body-derived data is stashed in kong.ctx.plugin for cross-phase access.
func (conf *Config) Access(kong *pdk.PDK) {
	body, err := kong.Request.GetRawBody()
	if err != nil {
		return
	}
	if len(body) == 0 {
		return
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return
	}

	// Stash JSON-RPC method in shared context for Log phase
	if method, ok := payload["method"].(string); ok {
		kong.Ctx.SetShared("worm_logger.jsonrpc_method", method)
	}

	// Stash mandate data if present
	if params, ok := payload["params"].(map[string]interface{}); ok {
		if mandate, ok := params["mandate"].(map[string]interface{}); ok {
			if mandateType, ok := mandate["type"].(string); ok {
				kong.Ctx.SetShared("worm_logger.mandate_type", mandateType)
			}
			if mandateJSON, err := json.Marshal(mandate); err == nil {
				kong.Ctx.SetShared("worm_logger.mandate_payload", string(mandateJSON))
			}
		}
	}
}

// Log is called during the Kong log phase (after response is sent).
func (conf *Config) Log(kong *pdk.PDK) {
	// Read trace context from request headers (available in Log phase)
	traceParent, _ := kong.Request.GetHeader("traceparent")
	traceID, spanID := parseTraceParent(traceParent)

	// A2A: Read DID from response headers (set by interceptor) or request headers (client-provided)
	senderDID, _ := kong.Response.GetHeader("X-Agent-DID")
	if senderDID == "" {
		senderDID, _ = kong.Request.GetHeader("X-Agent-DID")
	}
	receiverDID, _ := kong.Request.GetHeader("X-Receiver-DID")

	// A2A: Read verification results from response headers (set by verifier)
	verifiedStr, _ := kong.Response.GetHeader("X-Kong-DID-Verified")
	trustScoreStr, _ := kong.Response.GetHeader("X-Kong-Trust-Score")

	kongVerified := verifiedStr == "true"
	trustScore := 0
	if trustScoreStr != "" {
		fmt.Sscanf(trustScoreStr, "%d", &trustScore)
	}

	// Retrieve body-derived data from shared context (captured in Access phase)
	jsonrpcMethod, _ := kong.Ctx.GetSharedString("worm_logger.jsonrpc_method")
	mandateType, _ := kong.Ctx.GetSharedString("worm_logger.mandate_type")
	mandatePayloadStr, _ := kong.Ctx.GetSharedString("worm_logger.mandate_payload")

	var mandatePayload map[string]interface{}
	if mandatePayloadStr != "" {
		json.Unmarshal([]byte(mandatePayloadStr), &mandatePayload)
	}

	record := auditRecord{
		TraceID:        traceID,
		SpanID:         spanID,
		SenderDID:      senderDID,
		ReceiverDID:    receiverDID,
		JSONRPCMethod:  jsonrpcMethod,
		MandateType:    mandateType,
		MandatePayload: mandatePayload,
		KongVerified:   kongVerified,
		TrustScore:     trustScore,
	}

	// POST to WORM storage synchronously (Log phase runs after response is sent)
	data, err := json.Marshal(record)
	if err != nil {
		kong.Log.Err(fmt.Sprintf("WORM logger: failed to marshal record: %v", err))
		return
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(conf.WORMStorageURL+"/records", "application/json", bytes.NewReader(data))
	if err != nil {
		kong.Log.Err(fmt.Sprintf("WORM logger: failed to write record: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		kong.Log.Err(fmt.Sprintf("WORM logger: storage returned status %d", resp.StatusCode))
	}
}

// parseTraceParent extracts trace_id and span_id from W3C traceparent header.
// Format: version-trace_id-span_id-trace_flags (e.g., 00-abc123-def456-01)
func parseTraceParent(tp string) (traceID, spanID string) {
	if tp == "" {
		return "unknown", "unknown"
	}
	// Split by '-'
	parts := splitTraceparent(tp)
	if len(parts) >= 3 {
		return parts[1], parts[2]
	}
	return "unknown", "unknown"
}

func splitTraceparent(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '-' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}
