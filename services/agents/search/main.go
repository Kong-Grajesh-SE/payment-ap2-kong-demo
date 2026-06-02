package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/kong/payment-ap2-demo/services/agents/ap2"
)

var (
	agentDID    string
	agentPubKey ed25519.PublicKey
	agentPrivKey ed25519.PrivateKey
	meshClient  *ap2.Client
)

// Mock product catalog
var catalog = []map[string]interface{}{
	{
		"productId":   "nike-am90-001",
		"name":        "Nike Air Max 90",
		"description": "Classic Nike Air Max 90 in white/black",
		"price":       map[string]interface{}{"amount": 130.00, "currency": "USD"},
		"sizes":       []int{8, 9, 10, 11, 12},
		"category":    "footwear",
	},
	{
		"productId":   "nike-am90-002",
		"name":        "Nike Air Max 90 Premium",
		"description": "Premium edition Nike Air Max 90",
		"price":       map[string]interface{}{"amount": 160.00, "currency": "USD"},
		"sizes":       []int{8, 9, 10, 11},
		"category":    "footwear",
	},
	{
		"productId":   "nike-af1-001",
		"name":        "Nike Air Force 1 Low",
		"description": "Classic Nike Air Force 1",
		"price":       map[string]interface{}{"amount": 110.00, "currency": "USD"},
		"sizes":       []int{7, 8, 9, 10, 11, 12},
		"category":    "footwear",
	},
}

func main() {
	// Generate agent DID keys
	var err error
	agentPubKey, agentPrivKey, err = ed25519.GenerateKey(nil)
	if err != nil {
		log.Fatalf("Failed to generate keys: %v", err)
	}

	kongProxyURL := os.Getenv("KONG_PROXY_URL")
	if kongProxyURL == "" {
		kongProxyURL = "http://kong-dp:8000"
	}

	// Register DID with registry
	agentDID = registerDID()
	meshClient = ap2.NewClient(kongProxyURL, agentDID)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /", handleJSONRPC)
	mux.HandleFunc("GET /.well-known/agent.json", handleAgentCard)
	mux.HandleFunc("GET /health", handleHealth)

	port := os.Getenv("AGENT_PORT")
	if port == "" {
		port = "9001"
	}

	log.Printf("Search Agent listening on :%s (DID: %s)", port, agentDID)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func handleJSONRPC(w http.ResponseWriter, r *http.Request) {
	var req ap2.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, req.ID, -32700, "Parse error")
		return
	}

	log.Printf("Received method: %s", req.Method)

	switch req.Method {
	case ap2.MethodSearchExecute, "message/send":
		handleSearch(w, r, &req)
	default:
		sendError(w, req.ID, -32601, fmt.Sprintf("Method not found: %s", req.Method))
	}
}

func handleSearch(w http.ResponseWriter, r *http.Request, req *ap2.Request) {
	// Extract search intent from params
	params, _ := req.Params.(map[string]interface{})
	intent, _ := params["intent"].(string)
	if intent == "" {
		intent = "search products"
	}

	// Create IntentMandate
	mandate := ap2.IntentMandate{
		Type:     "IntentMandate",
		Consumer: getConsumerDID(req),
		Intent:   intent,
		Constraints: ap2.Constraints{
			MaxAmount:  ap2.Money{Amount: 200.00, Currency: "USD"},
			Currency:   "USD",
			Categories: []string{"footwear"},
		},
		IssuedAt: time.Now().UTC(),
	}

	// Sign the mandate
	sig, err := ap2.SignMandate(agentPrivKey, mandate)
	if err != nil {
		log.Printf("Failed to sign mandate: %v", err)
	}
	mandate.Signature = sig

	// Search the catalog (simple keyword match for demo)
	results := catalog // Return all for demo

	// Build result
	result := map[string]interface{}{
		"products":      results,
		"intentMandate": mandate,
		"agentDID":      agentDID,
	}

	// Forward to Cart Intent Agent via Kong mesh (disabled when BFF orchestrates)
	if os.Getenv("DISABLE_AUTO_CHAIN") == "" {
		traceID := extractTraceID(r)
		ctx := ap2.WithTraceID(context.Background(), traceID)

		cartReq := ap2.NewRequest(ap2.MethodCartAddIntent, map[string]interface{}{
			"mandate":  mandate,
			"products": results,
		}, "search-to-cart")

		go func() {
			cartResp, err := meshClient.CallAgent(ctx, "cart-intent", cartReq)
			if err != nil {
				log.Printf("Failed to forward to cart-intent: %v", err)
				return
			}
			log.Printf("Cart intent response: %+v", cartResp)
		}()
	}

	resp := ap2.NewResponse(req.ID, result)
	resp.Meta = &ap2.Meta{SenderDID: agentDID}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleAgentCard(w http.ResponseWriter, r *http.Request) {
	card := map[string]interface{}{
		"name":        "Search Agent",
		"description": "Product search and catalog agent with AP2 mandate support",
		"url":         fmt.Sprintf("http://search-agent:%s", os.Getenv("AGENT_PORT")),
		"version":     "1.0.0",
		"capabilities": map[string]interface{}{
			"streaming":           false,
			"pushNotifications":   false,
			"stateTransitionHistory": false,
		},
		"defaultInputModes":  []string{"text"},
		"defaultOutputModes": []string{"text"},
		"skills": []map[string]interface{}{
			{
				"id":          "product-search",
				"name":        "Product Search",
				"description": "Search the product catalog based on customer intent",
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(card)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status": "healthy", "agent": "search"}`))
}

func registerDID() string {
	registryURL := os.Getenv("DID_REGISTRY_URL")
	if registryURL == "" {
		registryURL = "http://did-registry:8070"
	}

	body := []byte(`{"method":"peer"}`)
	resp, err := http.Post(registryURL+"/dids", "application/json",
		bytes.NewReader(body))
	if err != nil || resp == nil {
		return fmt.Sprintf("did:peer:search-agent-%d", time.Now().UnixNano())
	}
	defer resp.Body.Close()

	var result struct {
		DID string `json:"did"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || result.DID == "" {
		return fmt.Sprintf("did:peer:search-agent-%d", time.Now().UnixNano())
	}
	return result.DID
}

func getConsumerDID(req *ap2.Request) string {
	if req.Meta != nil && req.Meta.SenderDID != "" {
		return req.Meta.SenderDID
	}
	return "did:peer:anonymous"
}

func extractTraceID(r *http.Request) string {
	tp := r.Header.Get("traceparent")
	if tp == "" {
		return fmt.Sprintf("trace-%d", time.Now().UnixNano())
	}
	// Parse W3C traceparent: version-trace_id-span_id-flags
	parts := splitByDash(tp)
	if len(parts) >= 2 {
		return parts[1]
	}
	return tp
}

func splitByDash(s string) []string {
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

func sendError(w http.ResponseWriter, id interface{}, code int, message string) {
	resp := ap2.NewErrorResponse(id, code, message)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(resp)
}
