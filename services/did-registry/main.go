package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
)

var store = NewDIDStore()

func main() {
	r := mux.NewRouter()

	r.HandleFunc("/dids", handleCreateDID).Methods("POST")
	r.HandleFunc("/dids", handleListDIDs).Methods("GET")
	r.HandleFunc("/dids/{did:.*}", handleResolveDID).Methods("GET")
	r.HandleFunc("/dids/{did:.*}", handleDeactivateDID).Methods("DELETE")
	r.HandleFunc("/dids/{did:.*}/verify", handleVerifySignature).Methods("POST")
	r.HandleFunc("/.well-known/did.json", handleWellKnownDID).Methods("GET")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8070"
	}

	log.Printf("DID Registry listening on :%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

type createDIDRequest struct {
	Method string `json:"method"` // "peer" or "web"
	Domain string `json:"domain,omitempty"`
}

type createDIDResponse struct {
	DID                string      `json:"did"`
	DIDDocument        DIDDocument `json:"didDocument"`
	PublicKeyMultibase string      `json:"publicKeyMultibase"`
}

func handleCreateDID(w http.ResponseWriter, r *http.Request) {
	var req createDIDRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Method = os.Getenv("DID_METHOD")
		if req.Method == "" {
			req.Method = "peer"
		}
	}

	var entry *DIDEntry
	var err error

	switch req.Method {
	case "web":
		domain := req.Domain
		if domain == "" {
			domain = os.Getenv("DID_WEB_DOMAIN")
			if domain == "" {
				domain = "localhost"
			}
		}
		entry, err = GenerateDIDWeb(domain)
	default:
		entry, err = GenerateDIDPeer()
	}

	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	store.Put(entry)

	resp := createDIDResponse{
		DID:                entry.DID,
		DIDDocument:        entry.Document,
		PublicKeyMultibase: entry.Document.VerificationMethod[0].PublicKeyMultibase,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func handleResolveDID(w http.ResponseWriter, r *http.Request) {
	did := mux.Vars(r)["did"]
	entry, ok := store.Get(did)
	if !ok {
		http.Error(w, `{"error": "DID not found"}`, http.StatusNotFound)
		return
	}
	if !entry.Active {
		http.Error(w, `{"error": "DID deactivated"}`, http.StatusGone)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entry.Document)
}

func handleDeactivateDID(w http.ResponseWriter, r *http.Request) {
	did := mux.Vars(r)["did"]
	if ok := store.Deactivate(did); !ok {
		http.Error(w, `{"error": "DID not found"}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type verifyRequest struct {
	Payload   string `json:"payload"`   // base64url-encoded
	Signature string `json:"signature"` // base64url-encoded
}

type verifyResponse struct {
	Valid bool `json:"valid"`
}

func handleVerifySignature(w http.ResponseWriter, r *http.Request) {
	did := mux.Vars(r)["did"]
	entry, ok := store.Get(did)
	if !ok {
		http.Error(w, `{"error": "DID not found"}`, http.StatusNotFound)
		return
	}

	var req verifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "invalid request body"}`, http.StatusBadRequest)
		return
	}

	payload, err := base64.RawURLEncoding.DecodeString(req.Payload)
	if err != nil {
		http.Error(w, `{"error": "invalid payload encoding"}`, http.StatusBadRequest)
		return
	}

	valid, err := VerifySignature(entry.PublicKey, payload, req.Signature)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(verifyResponse{Valid: valid})
}

func handleListDIDs(w http.ResponseWriter, r *http.Request) {
	entries := store.List()
	type listItem struct {
		DID       string `json:"did"`
		Active    bool   `json:"active"`
		CreatedAt string `json:"createdAt"`
	}
	var items []listItem
	for _, e := range entries {
		items = append(items, listItem{
			DID:       e.DID,
			Active:    e.Active,
			CreatedAt: e.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}
	if items == nil {
		items = []listItem{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

// handleWellKnownDID serves did:web documents for external resolution.
func handleWellKnownDID(w http.ResponseWriter, r *http.Request) {
	entries := store.List()
	for _, entry := range entries {
		if len(entry.DID) > 8 && entry.DID[:8] == "did:web:" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(entry.Document)
			return
		}
	}
	http.Error(w, `{"error": "no did:web document found"}`, http.StatusNotFound)
}
