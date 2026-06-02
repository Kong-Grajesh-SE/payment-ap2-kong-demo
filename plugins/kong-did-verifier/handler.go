package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Kong/go-pdk"
)

// Config holds the plugin configuration.
type Config struct {
	DIDRegistryURL string `json:"did_registry_url"`
}

// New creates a new plugin instance.
func New() interface{} {
	return &Config{
		DIDRegistryURL: "http://did-registry:8070",
	}
}

// didDocument matches the DID Document structure from the registry.
type didDocument struct {
	ID                 string               `json:"id"`
	VerificationMethod []verificationMethod `json:"verificationMethod"`
}

type verificationMethod struct {
	ID                 string `json:"id"`
	Type               string `json:"type"`
	PublicKeyMultibase string `json:"publicKeyMultibase"`
}

// Access is called during the Kong access phase.
func (conf *Config) Access(kong *pdk.PDK) {
	// A2A: Read DID identity from HTTP headers (set by interceptor or calling agent)
	senderDID, _ := kong.Request.GetHeader("X-Agent-DID")
	signature, _ := kong.Request.GetHeader("X-DID-Signature")

	if senderDID == "" {
		return // No DID present, interceptor will handle
	}

	if signature == "" {
		// DID present but no signature — newly provisioned, skip verification
		return
	}

	// Resolve DID from registry
	pubKey, err := resolveDIDPublicKey(conf.DIDRegistryURL, senderDID)
	if err != nil {
		kong.Log.Err(fmt.Sprintf("DID resolution failed: %v", err))
		sendJSONRPCError(kong, -32003, "DID resolution failed")
		return
	}

	// Read body for signature verification
	body, err := kong.Request.GetRawBody()
	if err != nil {
		kong.Log.Err(fmt.Sprintf("failed to read request body: %v", err))
		return
	}

	// A2A: Verify signature directly against raw body
	// (signature is in HTTP header, not embedded in body — no canonical reconstruction needed)
	sigBytes, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		kong.Log.Err(fmt.Sprintf("invalid signature encoding: %v", err))
		sendJSONRPCError(kong, -32003, "Invalid signature encoding")
		return
	}

	if !ed25519.Verify(pubKey, body, sigBytes) {
		kong.Log.Warn("DID signature verification failed")
		sendJSONRPCError(kong, -32003, "DID signature verification failed")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// A2A: Set verification results in HTTP headers (operational metadata)
	kong.ServiceRequest.SetHeader("X-Kong-DID-Verified", "true")
	kong.ServiceRequest.SetHeader("X-Kong-Trust-Score", "99")
	kong.ServiceRequest.SetHeader("X-Kong-Verifier-DID", "did:web:kong-gateway")
	kong.ServiceRequest.SetHeader("X-Kong-Verified-At", now)

	// Also set on response for downstream plugin access in Log phase
	kong.Response.SetHeader("X-Kong-DID-Verified", "true")
	kong.Response.SetHeader("X-Kong-Trust-Score", "99")

	// A2A: Inject verification results as DataPart into message parts
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return
	}

	verifyPart := map[string]interface{}{
		"type": "data",
		"data": map[string]interface{}{
			"kong_verification": map[string]interface{}{
				"verified":     true,
				"trust_score":  99,
				"verified_at":  now,
				"verifier_did": "did:web:kong-gateway",
			},
		},
		"metadata": map[string]interface{}{
			"source": "kong-did-verifier",
			"schema": "kong-verification/v1",
		},
	}

	if injectA2ADataPart(payload, verifyPart) {
		newBody, err := json.Marshal(payload)
		if err != nil {
			kong.Log.Err(fmt.Sprintf("failed to marshal enriched payload: %v", err))
			return
		}
		kong.ServiceRequest.SetRawBody(string(newBody))
	}
}

// injectA2ADataPart appends a DataPart into an A2A message's parts array.
// Navigates the JSON-RPC params.message.parts path per A2A protocol spec.
// Returns true if the payload had A2A structure and was modified.
func injectA2ADataPart(payload map[string]interface{}, part map[string]interface{}) bool {
	params, ok := payload["params"].(map[string]interface{})
	if !ok {
		return false
	}
	message, ok := params["message"].(map[string]interface{})
	if !ok {
		return false
	}
	parts, ok := message["parts"].([]interface{})
	if !ok {
		parts = []interface{}{}
	}
	parts = append(parts, part)
	message["parts"] = parts
	return true
}

func resolveDIDPublicKey(registryURL, did string) (ed25519.PublicKey, error) {
	encodedDID := url.PathEscape(did)
	resp, err := http.Get(registryURL + "/dids/" + encodedDID)
	if err != nil {
		return nil, fmt.Errorf("registry request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("registry returned %d: %s", resp.StatusCode, string(respBody))
	}

	var doc didDocument
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("failed to decode DID document: %w", err)
	}

	if len(doc.VerificationMethod) == 0 {
		return nil, fmt.Errorf("no verification methods in DID document")
	}

	pkMultibase := doc.VerificationMethod[0].PublicKeyMultibase
	if !strings.HasPrefix(pkMultibase, "z") {
		return nil, fmt.Errorf("unsupported multibase encoding")
	}

	pubKeyBytes, err := base64.RawURLEncoding.DecodeString(pkMultibase[1:])
	if err != nil {
		return nil, fmt.Errorf("failed to decode public key: %w", err)
	}

	if len(pubKeyBytes) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid public key size: %d", len(pubKeyBytes))
	}

	return ed25519.PublicKey(pubKeyBytes), nil
}

func sendJSONRPCError(kong *pdk.PDK, code int, message string) {
	errResp := map[string]interface{}{
		"jsonrpc": "2.0",
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
		"id": nil,
	}
	body, _ := json.Marshal(errResp)
	kong.Response.Exit(403, body, map[string][]string{
		"Content-Type": {"application/json"},
	})
}
