package services

import (
	"context"
	"sync"
	"time"

	"github.com/PPO_EVALUATION/models"
)

type Store interface {
	Set(key string, value string, ttl time.Duration)
	Get(key string) (string, error)
	Delete(key string) error
}

type kvStore struct {
	mu   sync.RWMutex
	data map[string]models.Entry
}

func NewKVStore(ctx context.Context, cleanupInterval time.Duration) *kvStore {
	s := &kvStore{
		data: make(map[string]models.Entry),
	}
	go s.startCleanup(ctx, cleanupInterval)
	return s
}

func (s *kvStore) Set(key string, value string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data[key] = models.Entry{
		Value:     value,
		ExpiresAt: time.Now().Add(ttl),
	}
}

func (s *kvStore) Get(key string) (string, error) {
	s.mu.RLock()
	entry, exists := s.data[key]
	s.mu.RUnlock()

	if !exists {
		return "", models.ErrKeyNotFound
	}

	if entry.IsExpired() {
		s.mu.Lock()
		if e, ok := s.data[key]; ok && e.IsExpired() {
			s.removeKey(key)
		}
		s.mu.Unlock()
		return "", models.ErrKeyExpired
	}

	return entry.Value, nil
}

func (s *kvStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, exists := s.data[key]
	if !exists {
		return models.ErrKeyNotFound
	}

	if entry.IsExpired() {
		s.removeKey(key)
		return models.ErrKeyExpired
	}

	s.removeKey(key)
	return nil
}

// removeKey deletes a key from the map. Caller must hold the write lock.
func (s *kvStore) removeKey(key string) {
	delete(s.data, key)
}

func (s *kvStore) startCleanup(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.removeExpired()
		case <-ctx.Done():
			return
		}
	}
}

func (s *kvStore) removeExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for key, entry := range s.data {
		if now.After(entry.ExpiresAt) {
			s.removeKey(key)
		}
	}
}
