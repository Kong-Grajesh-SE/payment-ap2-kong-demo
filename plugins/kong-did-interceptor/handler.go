package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Kong/go-pdk"
)

// Config holds the plugin configuration.
type Config struct {
	DIDRegistryURL string `json:"did_registry_url"`
	DIDMethod      string `json:"did_method"`
	DIDWebDomain   string `json:"did_web_domain"`
}

// New creates a new plugin instance.
func New() interface{} {
	return &Config{
		DIDRegistryURL: "http://did-registry:8070",
		DIDMethod:      "peer",
		DIDWebDomain:   "localhost",
	}
}

// Access is called during the Kong access phase.
func (conf *Config) Access(kong *pdk.PDK) {
	// Read request body
	body, err := kong.Request.GetRawBody()
	if err != nil {
		kong.Log.Err(fmt.Sprintf("failed to read request body: %v", err))
		return
	}

	if len(body) == 0 {
		return
	}

	// Parse as JSON-RPC
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		// Not JSON, skip
		return
	}

	// A2A: Check if DID already present in HTTP headers (agent-provided)
	existingDID, _ := kong.Request.GetHeader("X-Agent-DID")
	if existingDID != "" {
		// DID already set by the calling agent, let verifier handle
		return
	}

	// Provision a new DID
	didResp, err := provisionDID(conf.DIDRegistryURL, conf.DIDMethod, conf.DIDWebDomain)
	if err != nil {
		kong.Log.Err(fmt.Sprintf("failed to provision DID: %v", err))
		return
	}

	// Sign the payload
	signature, err := signWithRegistry(conf.DIDRegistryURL, didResp.DID, body)
	if err != nil {
		kong.Log.Err(fmt.Sprintf("failed to sign payload: %v", err))
		// Continue without signature
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// A2A: Set operational DID metadata in HTTP headers
	kong.ServiceRequest.SetHeader("X-Agent-DID", didResp.DID)
	kong.ServiceRequest.SetHeader("X-DID-Provisioned-At", now)
	if signature != "" {
		kong.ServiceRequest.SetHeader("X-DID-Signature", signature)
	}

	// A2A: Inject structured DID context as DataPart into message parts
	didContext := map[string]interface{}{
		"type": "data",
		"data": map[string]interface{}{
			"kong_did_context": map[string]interface{}{
				"sender_did":     didResp.DID,
				"provisioned_at": now,
				"did_method":     conf.DIDMethod,
			},
		},
		"metadata": map[string]interface{}{
			"source": "kong-did-interceptor",
			"schema": "kong-did-context/v1",
		},
	}

	if injectA2ADataPart(payload, didContext) {
		newBody, err := json.Marshal(payload)
		if err != nil {
			kong.Log.Err(fmt.Sprintf("failed to marshal modified payload: %v", err))
			return
		}
		kong.ServiceRequest.SetRawBody(string(newBody))
	}

	// Set response header so the agent can cache its DID
	kong.Response.SetHeader("X-Agent-DID", didResp.DID)
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

type didProvisionResponse struct {
	DID                string `json:"did"`
	PublicKeyMultibase string `json:"publicKeyMultibase"`
}

func provisionDID(registryURL, method, domain string) (*didProvisionResponse, error) {
	reqBody := map[string]string{
		"method": method,
	}
	if method == "web" {
		reqBody["domain"] = domain
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(registryURL+"/dids", "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("DID registry request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("DID registry returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result didProvisionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode DID response: %w", err)
	}

	return &result, nil
}

func signWithRegistry(_ string, _ string, _ []byte) (string, error) {
	// For this demo, the signature is produced by the agent itself
	// after receiving its DID. The interceptor sets up the DID identity.
	// Returning empty — the agent signs on subsequent requests.
	return "", nil
}
