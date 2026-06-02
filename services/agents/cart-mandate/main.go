package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
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
		port = "9003"
	}

	log.Printf("Cart Mandate Agent listening on :%s (DID: %s)", port, agentDID)
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
	case ap2.MethodCartConfirm, "message/send":
		handleCartConfirm(w, r, &req)
	default:
		sendError(w, req.ID, -32601, fmt.Sprintf("Method not found: %s", req.Method))
	}
}

func handleCartConfirm(w http.ResponseWriter, r *http.Request, req *ap2.Request) {
	params, _ := req.Params.(map[string]interface{})

	// Extract the cart mandate
	mandateData, _ := params["mandate"]
	mandateJSON, _ := json.Marshal(mandateData)

	var cartMandate ap2.CartMandate
	json.Unmarshal(mandateJSON, &cartMandate)

	// Hash the cart mandate for reference
	cartRef, err := ap2.HashMandate(cartMandate)
	if err != nil {
		log.Printf("Failed to hash cart mandate: %v", err)
		cartRef = "unknown"
	}

	// Generate DPAN (tokenized card number for demo)
	dpan := generateDPAN()

	// Create PaymentMandate
	paymentMandate := ap2.PaymentMandate{
		Type:     "PaymentMandate",
		CartRef:  cartRef,
		Amount:   cartMandate.TotalAmount,
		Method:   "card",
		DPAN:     dpan,
		AuthCode: generateAuthCode(),
		Status:   "authorized",
		IssuedAt: time.Now().UTC(),
	}

	// Sign the payment mandate
	sig, err := ap2.SignMandate(agentPrivKey, paymentMandate)
	if err != nil {
		log.Printf("Failed to sign payment mandate: %v", err)
	}
	paymentMandate.Signature = sig

	result := map[string]interface{}{
		"paymentMandate": paymentMandate,
		"agentDID":       agentDID,
		"status":         "payment_authorized",
	}

	// Forward to Payment Agent via Kong mesh (disabled when BFF orchestrates)
	if os.Getenv("DISABLE_AUTO_CHAIN") == "" {
		traceID := extractTraceID(r)
		ctx := ap2.WithTraceID(context.Background(), traceID)

		payReq := ap2.NewRequest(ap2.MethodPaymentExecute, map[string]interface{}{
			"mandate": paymentMandate,
		}, "cart-mandate-to-payment")

		go func() {
			payResp, err := meshClient.CallAgent(ctx, "payment", payReq)
			if err != nil {
				log.Printf("Failed to forward to payment: %v", err)
				return
			}
			log.Printf("Payment response: %+v", payResp)
		}()
	}

	resp := ap2.NewResponse(req.ID, result)
	resp.Meta = &ap2.Meta{SenderDID: agentDID}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func generateDPAN() string {
	// Generate a demo tokenized card number (DPAN)
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("4000-%s-%s-%s",
		hex.EncodeToString(b[:2]),
		hex.EncodeToString(b[2:4]),
		hex.EncodeToString(b[4:6]))
}

func generateAuthCode() string {
	b := make([]byte, 3)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func handleAgentCard(w http.ResponseWriter, r *http.Request) {
	card := map[string]interface{}{
		"name":        "Cart Mandate Agent",
		"description": "Validates cart mandates and creates payment mandates with DPAN tokenization",
		"version":     "1.0.0",
		"skills": []map[string]interface{}{
			{
				"id":          "cart-mandate",
				"name":        "Cart Mandate Confirmation",
				"description": "Confirm cart and authorize payment",
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(card)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status": "healthy", "agent": "cart-mandate"}`))
}

func registerDID() string {
	registryURL := os.Getenv("DID_REGISTRY_URL")
	if registryURL == "" {
		registryURL = "http://did-registry:8070"
	}
	body := []byte(`{"method":"peer"}`)
	resp, err := http.Post(registryURL+"/dids", "application/json", bytes.NewReader(body))
	if err != nil || resp == nil {
		return fmt.Sprintf("did:peer:cart-mandate-agent-%d", time.Now().UnixNano())
	}
	defer resp.Body.Close()
	var result struct {
		DID string `json:"did"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || result.DID == "" {
		return fmt.Sprintf("did:peer:cart-mandate-agent-%d", time.Now().UnixNano())
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
