package services

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/PPO_EVALUATION/models"
)

func TestSet_Get_HappyPath(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := NewKVStore(ctx, 10*time.Second)
	store.Set("name", "Alice", 5*time.Second)

	val, err := store.Get("name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "Alice" {
		t.Fatalf("got %q, want %q", val, "Alice")
	}
}

func TestGet_ErrorCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setup    func(Store)
		key      string
		sleepFor time.Duration
		wantErr  error
	}{
		{
			name:    "key does not exist",
			setup:   func(s Store) {},
			key:     "missing",
			wantErr: models.ErrKeyNotFound,
		},
		{
			name: "key expired",
			setup: func(s Store) {
				s.Set("temp", "val", 50*time.Millisecond)
			},
			key:      "temp",
			sleepFor: 100 * time.Millisecond,
			wantErr:  models.ErrKeyExpired,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			store := NewKVStore(ctx, 10*time.Second)
			tc.setup(store)

			if tc.sleepFor > 0 {
				time.Sleep(tc.sleepFor)
			}

			_, err := store.Get(tc.key)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Get(%q) error = %v, want %v", tc.key, err, tc.wantErr)
			}
		})
	}
}

func TestDelete_RemovesKey(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := NewKVStore(ctx, 10*time.Second)
	store.Set("key", "value", 5*time.Second)

	err := store.Delete("key")
	if err != nil {
		t.Fatalf("Delete returned unexpected error: %v", err)
	}

	_, err = store.Get("key")
	if !errors.Is(err, models.ErrKeyNotFound) {
		t.Errorf("after delete, Get error = %v, want %v", err, models.ErrKeyNotFound)
	}
}

func TestDelete_NonexistentKey(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := NewKVStore(ctx, 10*time.Second)
	err := store.Delete("nope")
	if !errors.Is(err, models.ErrKeyNotFound) {
		t.Errorf("Delete nonexistent key error = %v, want %v", err, models.ErrKeyNotFound)
	}
}

func TestDelete_ExpiredKey(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := NewKVStore(ctx, 10*time.Second)
	store.Set("temp", "val", 50*time.Millisecond)

	time.Sleep(100 * time.Millisecond)

	err := store.Delete("temp")
	if !errors.Is(err, models.ErrKeyExpired) {
		t.Errorf("Delete expired key error = %v, want %v", err, models.ErrKeyExpired)
	}
}

func TestSet_OverwriteExisting(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := NewKVStore(ctx, 10*time.Second)
	store.Set("key", "first", 5*time.Second)
	store.Set("key", "second", 5*time.Second)

	val, err := store.Get("key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "second" {
		t.Fatalf("got %q, want %q", val, "second")
	}
}

func TestConcurrent_SetGet(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := NewKVStore(ctx, 1*time.Second)

	var wg sync.WaitGroup
	const n = 100

	for i := 0; i < n; i++ {
		wg.Add(2)
		go func(id int) {
			defer wg.Done()
			store.Set(fmt.Sprintf("key:%d", id), "value", 5*time.Second)
		}(i)
		go func(id int) {
			defer wg.Done()
			store.Get(fmt.Sprintf("key:%d", id))
		}(i)
	}

	wg.Wait()
}

func TestConcurrent_SetDelete(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := NewKVStore(ctx, 1*time.Second)

	var wg sync.WaitGroup
	const n = 100

	for i := 0; i < n; i++ {
		wg.Add(2)
		go func(id int) {
			defer wg.Done()
			store.Set(fmt.Sprintf("key:%d", id), "value", 5*time.Second)
		}(i)
		go func(id int) {
			defer wg.Done()
			store.Delete(fmt.Sprintf("key:%d", id))
		}(i)
	}

	wg.Wait()
}

func TestBackgroundCleanup_RemovesExpired(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := NewKVStore(ctx, 100*time.Millisecond)
	store.Set("ephemeral", "gone", 50*time.Millisecond)

	time.Sleep(250 * time.Millisecond)

	store.mu.RLock()
	_, exists := store.data["ephemeral"]
	store.mu.RUnlock()

	if exists {
		t.Error("expected background cleanup to remove expired key")
	}
}
