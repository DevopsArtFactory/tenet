package authstate

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConcurrentFileStoresPreserveStates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth", "states.json")
	const count = 32
	ctx := context.Background()
	start := make(chan struct{})
	var workers sync.WaitGroup
	now := time.Now().UTC()
	for i := 0; i < count; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			<-start
			_, err := NewFileStore(path).Save(ctx, State{Token: fmt.Sprintf("state-%d", i), Provider: "github", CreatedAt: now, ExpiresAt: now.Add(time.Hour)})
			if err != nil {
				t.Errorf("save: %v", err)
			}
		}(i)
	}
	close(start)
	workers.Wait()
	states, err := NewFileStore(path).List(ctx)
	if err != nil || len(states) != count {
		t.Fatalf("saved %d states, want %d: %v", len(states), count, err)
	}
}

func TestConcurrentFileStoresConsumeOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "states.json")
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := NewFileStore(path).Save(ctx, State{Token: "state-0", Provider: "github", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	const count = 32
	var workers sync.WaitGroup
	start := make(chan struct{})
	var consumed atomic.Int32
	for i := 0; i < count; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			_, err := NewFileStore(path).Consume(ctx, "state-0")
			if err == nil {
				consumed.Add(1)
			} else if !errors.Is(err, ErrConsumed) {
				t.Errorf("consume: %v", err)
			}
		}()
	}
	close(start)
	workers.Wait()
	if got := consumed.Load(); got != 1 {
		t.Fatalf("consumed state %d times, want 1", got)
	}
}
