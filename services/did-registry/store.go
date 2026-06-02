package main

import (
	"sync"
)

// DIDStore is a thread-safe in-memory store for DID entries.
type DIDStore struct {
	mu      sync.RWMutex
	entries map[string]*DIDEntry
}

// NewDIDStore creates a new empty DID store.
func NewDIDStore() *DIDStore {
	return &DIDStore{
		entries: make(map[string]*DIDEntry),
	}
}

// Put adds or updates a DID entry.
func (s *DIDStore) Put(entry *DIDEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[entry.DID] = entry
}

// Get retrieves a DID entry by its DID string.
func (s *DIDStore) Get(did string) (*DIDEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.entries[did]
	return entry, ok
}

// Deactivate marks a DID as inactive.
func (s *DIDStore) Deactivate(did string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[did]
	if !ok {
		return false
	}
	entry.Active = false
	return true
}

// List returns all active DID entries.
func (s *DIDStore) List() []*DIDEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*DIDEntry
	for _, entry := range s.entries {
		if entry.Active {
			result = append(result, entry)
		}
	}
	return result
}
