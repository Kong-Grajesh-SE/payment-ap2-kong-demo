package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq"
)

var db *sql.DB

type auditRecord struct {
	ID             string                 `json:"id,omitempty"`
	TraceID        string                 `json:"trace_id"`
	SpanID         string                 `json:"span_id"`
	SenderDID      string                 `json:"sender_did"`
	ReceiverDID    string                 `json:"receiver_did,omitempty"`
	JSONRPCMethod  string                 `json:"jsonrpc_method"`
	MandateType    string                 `json:"mandate_type,omitempty"`
	MandatePayload map[string]interface{} `json:"mandate_payload,omitempty"`
	KongVerified   bool                   `json:"kong_verified"`
	TrustScore     int                    `json:"trust_score"`
	CreatedAt      string                 `json:"created_at,omitempty"`
}

func main() {
	dbHost := getEnv("WORM_DB_HOST", "localhost")
	dbPort := getEnv("WORM_DB_PORT", "5432")
	dbUser := getEnv("WORM_DB_USER", "worm_user")
	dbPassword := getEnv("WORM_DB_PASSWORD", "worm_secure_password")
	dbName := getEnv("WORM_DB_NAME", "audit_db")

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	var err error
	for i := 0; i < 30; i++ {
		db, err = sql.Open("postgres", connStr)
		if err == nil {
			if err = db.Ping(); err == nil {
				break
			}
		}
		log.Printf("Waiting for database... (%d/30)", i+1)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /records", handleCreateRecord)
	mux.HandleFunc("GET /records", handleQueryRecords)
	mux.HandleFunc("GET /records/{id}", handleGetRecord)
	mux.HandleFunc("GET /health", handleHealth)

	port := getEnv("PORT", "8090")
	log.Printf("WORM Storage listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func handleCreateRecord(w http.ResponseWriter, r *http.Request) {
	var rec auditRecord
	if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
		http.Error(w, `{"error": "invalid request body"}`, http.StatusBadRequest)
		return
	}

	mandatePayloadJSON, _ := json.Marshal(rec.MandatePayload)

	var id string
	var createdAt time.Time
	err := db.QueryRow(`
		INSERT INTO audit_records (trace_id, span_id, sender_did, receiver_did, jsonrpc_method, mandate_type, mandate_payload, kong_verified, trust_score)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at`,
		rec.TraceID, rec.SpanID, rec.SenderDID, nullString(rec.ReceiverDID),
		rec.JSONRPCMethod, nullString(rec.MandateType), mandatePayloadJSON,
		rec.KongVerified, rec.TrustScore,
	).Scan(&id, &createdAt)

	if err != nil {
		log.Printf("Failed to insert record: %v", err)
		http.Error(w, fmt.Sprintf(`{"error": "insert failed: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	rec.ID = id
	rec.CreatedAt = createdAt.Format(time.RFC3339)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(rec)
}

func handleQueryRecords(w http.ResponseWriter, r *http.Request) {
	traceID := r.URL.Query().Get("trace_id")
	senderDID := r.URL.Query().Get("sender_did")

	var rows *sql.Rows
	var err error

	switch {
	case traceID != "":
		rows, err = db.Query(`SELECT id, trace_id, span_id, sender_did, receiver_did, jsonrpc_method, mandate_type, mandate_payload, kong_verified, trust_score, created_at FROM audit_records WHERE trace_id = $1 ORDER BY created_at`, traceID)
	case senderDID != "":
		rows, err = db.Query(`SELECT id, trace_id, span_id, sender_did, receiver_did, jsonrpc_method, mandate_type, mandate_payload, kong_verified, trust_score, created_at FROM audit_records WHERE sender_did = $1 ORDER BY created_at`, senderDID)
	default:
		rows, err = db.Query(`SELECT id, trace_id, span_id, sender_did, receiver_did, jsonrpc_method, mandate_type, mandate_payload, kong_verified, trust_score, created_at FROM audit_records ORDER BY created_at DESC LIMIT 100`)
	}

	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var records []auditRecord
	for rows.Next() {
		var rec auditRecord
		var receiverDID, mandateType sql.NullString
		var mandatePayloadJSON []byte
		var createdAt time.Time

		if err := rows.Scan(&rec.ID, &rec.TraceID, &rec.SpanID, &rec.SenderDID, &receiverDID,
			&rec.JSONRPCMethod, &mandateType, &mandatePayloadJSON, &rec.KongVerified, &rec.TrustScore, &createdAt); err != nil {
			continue
		}
		rec.ReceiverDID = receiverDID.String
		rec.MandateType = mandateType.String
		rec.CreatedAt = createdAt.Format(time.RFC3339)
		if len(mandatePayloadJSON) > 0 {
			json.Unmarshal(mandatePayloadJSON, &rec.MandatePayload)
		}
		records = append(records, rec)
	}

	if records == nil {
		records = []auditRecord{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(records)
}

func handleGetRecord(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var rec auditRecord
	var receiverDID, mandateType sql.NullString
	var mandatePayloadJSON []byte
	var createdAt time.Time

	err := db.QueryRow(`SELECT id, trace_id, span_id, sender_did, receiver_did, jsonrpc_method, mandate_type, mandate_payload, kong_verified, trust_score, created_at FROM audit_records WHERE id = $1`, id).
		Scan(&rec.ID, &rec.TraceID, &rec.SpanID, &rec.SenderDID, &receiverDID,
			&rec.JSONRPCMethod, &mandateType, &mandatePayloadJSON, &rec.KongVerified, &rec.TrustScore, &createdAt)

	if err == sql.ErrNoRows {
		http.Error(w, `{"error": "record not found"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	rec.ReceiverDID = receiverDID.String
	rec.MandateType = mandateType.String
	rec.CreatedAt = createdAt.Format(time.RFC3339)
	if len(mandatePayloadJSON) > 0 {
		json.Unmarshal(mandatePayloadJSON, &rec.MandatePayload)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rec)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := db.Ping(); err != nil {
		http.Error(w, `{"status": "unhealthy"}`, http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status": "healthy"}`))
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
