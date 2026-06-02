package main

import (
	"bytes"
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
)

func main() {
	var err error
	agentPubKey, agentPrivKey, err = ed25519.GenerateKey(nil)
	if err != nil {
		log.Fatalf("Failed to generate keys: %v", err)
	}

	agentDID = registerDID()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /", handleJSONRPC)
	mux.HandleFunc("GET /.well-known/agent.json", handleAgentCard)
	mux.HandleFunc("GET /health", handleHealth)

	port := os.Getenv("AGENT_PORT")
	if port == "" {
		port = "9004"
	}

	log.Printf("Payment Agent listening on :%s (DID: %s)", port, agentDID)
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
	case ap2.MethodPaymentExecute, "message/send":
		handlePaymentExecute(w, r, &req)
	default:
		sendError(w, req.ID, -32601, fmt.Sprintf("Method not found: %s", req.Method))
	}
}

func handlePaymentExecute(w http.ResponseWriter, r *http.Request, req *ap2.Request) {
	params, _ := req.Params.(map[string]interface{})

	// Extract the payment mandate
	mandateData, _ := params["mandate"]
	mandateJSON, _ := json.Marshal(mandateData)

	var paymentMandate ap2.PaymentMandate
	json.Unmarshal(mandateJSON, &paymentMandate)

	// Simulate OTP challenge
	otpChallenge := generateOTP()
	log.Printf("OTP Challenge generated: %s (auto-verified for demo)", otpChallenge)

	// Simulate payment processing
	time.Sleep(100 * time.Millisecond) // Simulate processing delay

	// Update mandate status to settled
	paymentMandate.Status = "settled"

	// Sign the settlement
	sig, err := ap2.SignMandate(agentPrivKey, paymentMandate)
	if err != nil {
		log.Printf("Failed to sign settlement: %v", err)
	}
	paymentMandate.Signature = sig

	// Generate receipt
	receipt := map[string]interface{}{
		"receiptId":      generateReceiptID(),
		"paymentMandate": paymentMandate,
		"settlement": map[string]interface{}{
			"status":      "settled",
			"settledAt":   time.Now().UTC().Format(time.RFC3339),
			"processorId": "demo-processor-001",
			"authCode":    paymentMandate.AuthCode,
		},
		"otpVerified": true,
		"agentDID":    agentDID,
	}

	resp := ap2.NewResponse(req.ID, receipt)
	resp.Meta = &ap2.Meta{SenderDID: agentDID}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)

	log.Printf("Payment settled: %s for %s %.2f",
		paymentMandate.CartRef,
		paymentMandate.Amount.Currency,
		paymentMandate.Amount.Amount)
}

func generateOTP() string {
	b := make([]byte, 3)
	rand.Read(b)
	// Generate a 6-digit OTP
	otp := int(b[0])<<16 | int(b[1])<<8 | int(b[2])
	return fmt.Sprintf("%06d", otp%1000000)
}

func generateReceiptID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("RCP-%s", hex.EncodeToString(b))
}

func handleAgentCard(w http.ResponseWriter, r *http.Request) {
	card := map[string]interface{}{
		"name":        "Payment Agent",
		"description": "Processes payment mandates with OTP challenge and settlement",
		"version":     "1.0.0",
		"skills": []map[string]interface{}{
			{
				"id":          "payment-processing",
				"name":        "Payment Processing",
				"description": "Execute payment with OTP verification and settlement",
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(card)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status": "healthy", "agent": "payment"}`))
}

func registerDID() string {
	registryURL := os.Getenv("DID_REGISTRY_URL")
	if registryURL == "" {
		registryURL = "http://did-registry:8070"
	}
	body := []byte(`{"method":"peer"}`)
	resp, err := http.Post(registryURL+"/dids", "application/json", bytes.NewReader(body))
	if err != nil || resp == nil {
		return fmt.Sprintf("did:peer:payment-agent-%d", time.Now().UnixNano())
	}
	defer resp.Body.Close()
	var result struct {
		DID string `json:"did"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || result.DID == "" {
		return fmt.Sprintf("did:peer:payment-agent-%d", time.Now().UnixNano())
	}
	return result.DID
}

func sendError(w http.ResponseWriter, id interface{}, code int, message string) {
	resp := ap2.NewErrorResponse(id, code, message)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(resp)
}
