package ap2

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// SignMandate signs any mandate struct, producing a base64url-encoded Ed25519 signature.
// It strips the existing signature field before signing.
func SignMandate(privKey ed25519.PrivateKey, mandate interface{}) (string, error) {
	canonical, err := canonicalJSON(mandate)
	if err != nil {
		return "", fmt.Errorf("failed to produce canonical JSON: %w", err)
	}
	sig := ed25519.Sign(privKey, canonical)
	return base64.RawURLEncoding.EncodeToString(sig), nil
}

// VerifyMandate verifies the signature on a mandate struct using the given public key.
func VerifyMandate(pubKey ed25519.PublicKey, mandate interface{}, signature string) (bool, error) {
	sig, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return false, fmt.Errorf("failed to decode signature: %w", err)
	}

	canonical, err := canonicalJSON(mandate)
	if err != nil {
		return false, fmt.Errorf("failed to produce canonical JSON: %w", err)
	}

	return ed25519.Verify(pubKey, canonical, sig), nil
}

// HashMandate produces a SHA-256 hash of a mandate for cross-referencing.
func HashMandate(mandate interface{}) (string, error) {
	canonical, err := canonicalJSON(mandate)
	if err != nil {
		return "", fmt.Errorf("failed to produce canonical JSON: %w", err)
	}
	h := sha256.Sum256(canonical)
	return base64.RawURLEncoding.EncodeToString(h[:]), nil
}

// canonicalJSON marshals a struct to JSON, strips the "signature" field, and returns
// the canonical byte representation for signing/verification.
func canonicalJSON(v interface{}) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	// Unmarshal into a map to strip the signature field
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	delete(m, "signature")

	// Re-marshal for canonical form
	return json.Marshal(m)
}
