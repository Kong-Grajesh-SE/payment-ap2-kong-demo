package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// DIDDocument represents a W3C DID Core compliant DID Document.
type DIDDocument struct {
	Context            []string             `json:"@context"`
	ID                 string               `json:"id"`
	VerificationMethod []VerificationMethod `json:"verificationMethod"`
	Authentication     []string             `json:"authentication"`
	AssertionMethod    []string             `json:"assertionMethod"`
	Created            time.Time            `json:"created"`
	Updated            time.Time            `json:"updated"`
}

// VerificationMethod holds the public key material.
type VerificationMethod struct {
	ID                 string `json:"id"`
	Type               string `json:"type"`
	Controller         string `json:"controller"`
	PublicKeyMultibase string `json:"publicKeyMultibase"`
}

// DIDEntry stores a DID along with its key material.
type DIDEntry struct {
	DID         string         `json:"did"`
	Document    DIDDocument    `json:"didDocument"`
	PrivateKey  ed25519.PrivateKey `json:"-"`
	PublicKey   ed25519.PublicKey  `json:"-"`
	Active      bool           `json:"active"`
	CreatedAt   time.Time      `json:"createdAt"`
}

// GenerateDIDPeer creates a new did:peer DID with Ed25519 key pair.
func GenerateDIDPeer() (*DIDEntry, error) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ed25519 key: %w", err)
	}

	// Multibase encode: 'z' prefix + base58btc (we use base64url for simplicity)
	pubKeyMultibase := "z" + base64.RawURLEncoding.EncodeToString(pub)

	// did:peer:2.Ez<encoded-key>
	did := fmt.Sprintf("did:peer:2.Ez%s", base64.RawURLEncoding.EncodeToString(pub))

	doc := BuildDIDDocument(did, pubKeyMultibase)
	return &DIDEntry{
		DID:        did,
		Document:   doc,
		PrivateKey: priv,
		PublicKey:  pub,
		Active:     true,
		CreatedAt:  time.Now().UTC(),
	}, nil
}

// GenerateDIDWeb creates a new did:web DID with Ed25519 key pair.
func GenerateDIDWeb(domain string) (*DIDEntry, error) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ed25519 key: %w", err)
	}

	pubKeyMultibase := "z" + base64.RawURLEncoding.EncodeToString(pub)

	// Encode colons in the domain
	encodedDomain := strings.ReplaceAll(domain, ":", "%3A")
	did := fmt.Sprintf("did:web:%s", encodedDomain)

	doc := BuildDIDDocument(did, pubKeyMultibase)
	return &DIDEntry{
		DID:        did,
		Document:   doc,
		PrivateKey: priv,
		PublicKey:  pub,
		Active:     true,
		CreatedAt:  time.Now().UTC(),
	}, nil
}

// BuildDIDDocument creates a W3C DID Core compliant document.
func BuildDIDDocument(did, pubKeyMultibase string) DIDDocument {
	now := time.Now().UTC()
	vmID := did + "#key-1"
	return DIDDocument{
		Context: []string{
			"https://www.w3.org/ns/did/v1",
			"https://w3id.org/security/suites/ed25519-2020/v1",
		},
		ID: did,
		VerificationMethod: []VerificationMethod{
			{
				ID:                 vmID,
				Type:               "Ed25519VerificationKey2020",
				Controller:         did,
				PublicKeyMultibase: pubKeyMultibase,
			},
		},
		Authentication:  []string{vmID},
		AssertionMethod: []string{vmID},
		Created:         now,
		Updated:         now,
	}
}

// SignPayload signs a payload with the given Ed25519 private key.
func SignPayload(privKey ed25519.PrivateKey, payload []byte) string {
	sig := ed25519.Sign(privKey, payload)
	return base64.RawURLEncoding.EncodeToString(sig)
}

// VerifySignature verifies an Ed25519 signature against a public key.
func VerifySignature(pubKey ed25519.PublicKey, payload []byte, signatureB64 string) (bool, error) {
	sig, err := base64.RawURLEncoding.DecodeString(signatureB64)
	if err != nil {
		return false, fmt.Errorf("failed to decode signature: %w", err)
	}
	return ed25519.Verify(pubKey, payload, sig), nil
}

// HashPayload returns a SHA-256 hash of a JSON-serializable value.
func HashPayload(v interface{}) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return base64.RawURLEncoding.EncodeToString(h[:]), nil
}
