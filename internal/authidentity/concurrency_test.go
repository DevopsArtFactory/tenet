package authidentity

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

func TestConcurrentFileStoresPreserveIdentities(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth", "identities.json")
	const count = 32
	ctx := context.Background()
	start := make(chan struct{})
	var workers sync.WaitGroup
	for i := 0; i < count; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			<-start
			_, err := NewFileStore(path).Upsert(ctx, Identity{Provider: "github", Subject: fmt.Sprint(i)})
			if err != nil {
				t.Errorf("upsert: %v", err)
			}
		}(i)
	}
	close(start)
	workers.Wait()
	identities, err := NewFileStore(path).List(ctx)
	if err != nil || len(identities) != count {
		t.Fatalf("saved %d identities, want %d: %v", len(identities), count, err)
	}
}
