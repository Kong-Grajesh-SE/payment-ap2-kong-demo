package ap2

// Request represents a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
	ID      interface{} `json:"id"`
	Meta    *Meta       `json:"_meta,omitempty"`
}

// Response represents a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
	ID      interface{} `json:"id"`
	Meta    *Meta       `json:"_meta,omitempty"`
}

// RPCError represents a JSON-RPC 2.0 error.
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Meta represents the _meta extension field with DID and trace context.
type Meta struct {
	SenderDID      string `json:"sender_did,omitempty"`
	ReceiverDID    string `json:"receiver_did,omitempty"`
	Signature      string `json:"signature,omitempty"`
	ProvisionedAt  string `json:"provisioned_at,omitempty"`
	VerifiedByKong bool   `json:"verified_by_kong,omitempty"`
	TrustScore     int    `json:"trust_score,omitempty"`
	VerifiedAt     string `json:"verified_at,omitempty"`
	VerifierDID    string `json:"verifier_did,omitempty"`
	TraceID        string `json:"trace_id,omitempty"`
	SpanID         string `json:"span_id,omitempty"`
}

// NewRequest creates a new JSON-RPC 2.0 request.
func NewRequest(method string, params interface{}, id interface{}) *Request {
	return &Request{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      id,
	}
}

// NewResponse creates a new JSON-RPC 2.0 success response.
func NewResponse(id interface{}, result interface{}) *Response {
	return &Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      id,
	}
}

// NewErrorResponse creates a new JSON-RPC 2.0 error response.
func NewErrorResponse(id interface{}, code int, message string) *Response {
	return &Response{
		JSONRPC: "2.0",
		Error: &RPCError{
			Code:    code,
			Message: message,
		},
		ID: id,
	}
}

// A2A standard methods
const (
	MethodMessageSend = "message/send"
	MethodTasksGet    = "tasks/get"
	MethodTasksCancel = "tasks/cancel"

	// AP2 commerce methods
	MethodSearchExecute     = "search/execute"
	MethodCartAddIntent     = "cart/addIntent"
	MethodCartConfirm       = "cart/confirmMandate"
	MethodPaymentExecute    = "payment/execute"
)
