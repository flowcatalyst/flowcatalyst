package checkpoint

import (
	"sync"

	"go.mongodb.org/mongo-driver/bson"
)

// MemoryStore stores checkpoints in memory.
// This is intended for testing and development only.
// All checkpoints are lost on restart.
type MemoryStore struct {
	mu     sync.RWMutex
	tokens map[string]bson.Raw
}

// NewMemoryStore creates a new in-memory checkpoint store
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		tokens: make(map[string]bson.Raw),
	}
}

// GetCheckpoint retrieves a checkpoint (resume token)
func (s *MemoryStore) GetCheckpoint(key string) (bson.Raw, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	token, ok := s.tokens[key]
	if !ok {
		return nil, nil
	}

	// Return a copy to prevent external mutation
	if len(token) == 0 {
		return nil, nil
	}

	copied := make(bson.Raw, len(token))
	copy(copied, token)
	return copied, nil
}

// SaveCheckpoint saves a checkpoint
func (s *MemoryStore) SaveCheckpoint(key string, token bson.Raw) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Store a copy to prevent external mutation
	copied := make(bson.Raw, len(token))
	copy(copied, token)
	s.tokens[key] = copied

	return nil
}

// Clear removes all checkpoints (useful for testing)
func (s *MemoryStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens = make(map[string]bson.Raw)
}

// Delete removes a specific checkpoint
func (s *MemoryStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, key)
	return nil
}
