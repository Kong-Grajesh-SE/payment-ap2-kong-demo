package ap2

import "time"

// Constraints defines spending limits for an IntentMandate.
type Constraints struct {
	MaxAmount Money    `json:"maxAmount"`
	Currency  string   `json:"currency"`
	Categories []string `json:"categories,omitempty"`
}

// Money represents a monetary amount.
type Money struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

// CartItem represents an item in a shopping cart.
type CartItem struct {
	ProductID   string  `json:"productId"`
	Name        string  `json:"name"`
	Quantity    int     `json:"quantity"`
	UnitPrice   Money   `json:"unitPrice"`
	Description string  `json:"description,omitempty"`
}

// IntentMandate represents a consumer's purchase intent (AP2 Layer 1).
type IntentMandate struct {
	Type        string      `json:"type"`        // "IntentMandate"
	Consumer    string      `json:"consumer"`    // consumer DID
	Intent      string      `json:"intent"`      // natural language intent
	Constraints Constraints `json:"constraints"`
	IssuedAt    time.Time   `json:"issuedAt"`
	Signature   string      `json:"signature"`   // Ed25519 sig
}

// CartMandate represents a confirmed shopping cart (AP2 Layer 2).
type CartMandate struct {
	Type           string     `json:"type"`           // "CartMandate"
	IntentRef      string     `json:"intentRef"`      // hash of IntentMandate
	Merchant       string     `json:"merchant"`
	Items          []CartItem `json:"items"`
	TotalAmount    Money      `json:"totalAmount"`
	PaymentMethods []string   `json:"paymentMethods"`
	IssuedAt       time.Time  `json:"issuedAt"`
	Signature      string     `json:"signature"`
}

// PaymentMandate represents a payment authorization (AP2 Layer 3).
type PaymentMandate struct {
	Type     string    `json:"type"`             // "PaymentMandate"
	CartRef  string    `json:"cartRef"`          // hash of CartMandate
	Amount   Money     `json:"amount"`
	Method   string    `json:"method"`           // "card", "bank_transfer"
	DPAN     string    `json:"dpan,omitempty"`   // tokenized card
	AuthCode string    `json:"authCode"`
	Status   string    `json:"status"`           // "authorized", "settled"
	IssuedAt time.Time `json:"issuedAt"`
	Signature string   `json:"signature"`
}
