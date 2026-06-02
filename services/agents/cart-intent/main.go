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
	agentDID     string
	agentPubKey  ed25519.PublicKey
	agentPrivKey ed25519.PrivateKey
	meshClient   *ap2.Client
)

func main() {
	var err error
	agentPubKey, agentPrivKey, err = ed25519.GenerateKey(nil)
	if err != nil {
		log.Fatalf("Failed to generate keys: %v", err)
	}

	kongProxyURL := os.Getenv("KONG_PROXY_URL")
	if kongProxyURL == "" {
		kongProxyURL = "http://kong-dp:8000"
	}

	agentDID = registerDID()
	meshClient = ap2.NewClient(kongProxyURL, agentDID)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /", handleJSONRPC)
	mux.HandleFunc("GET /.well-known/agent.json", handleAgentCard)
	mux.HandleFunc("GET /health", handleHealth)

	port := os.Getenv("AGENT_PORT")
	if port == "" {
		port = "9002"
	}

	log.Printf("Cart Intent Agent listening on :%s (DID: %s)", port, agentDID)
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
	case ap2.MethodCartAddIntent, "message/send":
		handleCartAddIntent(w, r, &req)
	default:
		sendError(w, req.ID, -32601, fmt.Sprintf("Method not found: %s", req.Method))
	}
}

func handleCartAddIntent(w http.ResponseWriter, r *http.Request, req *ap2.Request) {
	params, _ := req.Params.(map[string]interface{})

	// Extract the intent mandate
	mandateData, _ := params["mandate"]
	mandateJSON, _ := json.Marshal(mandateData)

	var intentMandate ap2.IntentMandate
	json.Unmarshal(mandateJSON, &intentMandate)

	// Hash the intent mandate for reference
	intentRef, err := ap2.HashMandate(intentMandate)
	if err != nil {
		log.Printf("Failed to hash intent mandate: %v", err)
		intentRef = "unknown"
	}

	// Extract products from search results
	products, _ := params["products"].([]interface{})

	// Build cart items from first matching product
	var items []ap2.CartItem
	var totalAmount float64

	for _, p := range products {
		prod, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		price := 0.0
		if priceObj, ok := prod["price"].(map[string]interface{}); ok {
			price, _ = priceObj["amount"].(float64)
		}
		items = append(items, ap2.CartItem{
			ProductID:   fmt.Sprintf("%v", prod["productId"]),
			Name:        fmt.Sprintf("%v", prod["name"]),
			Quantity:    1,
			UnitPrice:   ap2.Money{Amount: price, Currency: "USD"},
			Description: fmt.Sprintf("%v", prod["description"]),
		})
		totalAmount += price
		break // Take first product for demo
	}

	if len(items) == 0 {
		items = []ap2.CartItem{{
			ProductID: "default-001",
			Name:      "Demo Product",
			Quantity:  1,
			UnitPrice: ap2.Money{Amount: 100.00, Currency: "USD"},
		}}
		totalAmount = 100.00
	}

	// Create CartMandate
	cartMandate := ap2.CartMandate{
		Type:           "CartMandate",
		IntentRef:      intentRef,
		Merchant:       "Demo Commerce Store",
		Items:          items,
		TotalAmount:    ap2.Money{Amount: totalAmount, Currency: "USD"},
		PaymentMethods: []string{"card", "bank_transfer"},
		IssuedAt:       time.Now().UTC(),
	}

	// Sign the cart mandate
	sig, err := ap2.SignMandate(agentPrivKey, cartMandate)
	if err != nil {
		log.Printf("Failed to sign cart mandate: %v", err)
	}
	cartMandate.Signature = sig

	result := map[string]interface{}{
		"cartMandate": cartMandate,
		"agentDID":    agentDID,
		"status":      "cart_created",
	}

	// Forward to Cart Mandate Agent via Kong mesh (disabled when BFF orchestrates)
	if os.Getenv("DISABLE_AUTO_CHAIN") == "" {
		traceID := extractTraceID(r)
		ctx := ap2.WithTraceID(context.Background(), traceID)

		confirmReq := ap2.NewRequest(ap2.MethodCartConfirm, map[string]interface{}{
			"mandate": cartMandate,
		}, "cart-intent-to-mandate")

		go func() {
			confirmResp, err := meshClient.CallAgent(ctx, "cart-mandate", confirmReq)
			if err != nil {
				log.Printf("Failed to forward to cart-mandate: %v", err)
				return
			}
			log.Printf("Cart mandate response: %+v", confirmResp)
		}()
	}

	resp := ap2.NewResponse(req.ID, result)
	resp.Meta = &ap2.Meta{SenderDID: agentDID}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleAgentCard(w http.ResponseWriter, r *http.Request) {
	card := map[string]interface{}{
		"name":        "Cart Intent Agent",
		"description": "Validates intent mandates and creates cart mandates",
		"version":     "1.0.0",
		"skills": []map[string]interface{}{
			{
				"id":          "cart-intent",
				"name":        "Cart Intent Processing",
				"description": "Process purchase intent and create shopping cart",
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(card)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status": "healthy", "agent": "cart-intent"}`))
}

func registerDID() string {
	registryURL := os.Getenv("DID_REGISTRY_URL")
	if registryURL == "" {
		registryURL = "http://did-registry:8070"
	}
	body := []byte(`{"method":"peer"}`)
	resp, err := http.Post(registryURL+"/dids", "application/json", bytes.NewReader(body))
	if err != nil || resp == nil {
		return fmt.Sprintf("did:peer:cart-intent-agent-%d", time.Now().UnixNano())
	}
	defer resp.Body.Close()
	var result struct {
		DID string `json:"did"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || result.DID == "" {
		return fmt.Sprintf("did:peer:cart-intent-agent-%d", time.Now().UnixNano())
	}
	return result.DID
}

func extractTraceID(r *http.Request) string {
	tp := r.Header.Get("traceparent")
	if tp == "" {
		return fmt.Sprintf("trace-%d", time.Now().UnixNano())
	}
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
