//go:build darwin || linux

package planpreview

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/filelock"
)

func TestPlanFileStorePreservesConcurrentProcessCreates(t *testing.T) {
	if storePath := os.Getenv("TENET_TEST_PLAN_STORE"); storePath != "" {
		store := NewFileStore(storePath)
		for i := 0; i < 20; i++ {
			if err := store.Save(context.Background(), Record{ID: fmt.Sprintf("plan_%s_%d", os.Getenv("TENET_TEST_PLAN_WORKER"), i), Status: StatusReady}); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	storePath := filepath.Join(t.TempDir(), "plans.json")
	var workers sync.WaitGroup
	for i := 0; i < 6; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			cmd := exec.Command(os.Args[0], "-test.run=^TestPlanFileStorePreservesConcurrentProcessCreates$", "-test.timeout=30s")
			cmd.Env = append(os.Environ(), "TENET_TEST_PLAN_STORE="+storePath, "TENET_TEST_PLAN_WORKER="+strconv.Itoa(i))
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("plan worker %d: %v\n%s", i, err, output)
			}
		}()
	}
	workers.Wait()
	records, err := Load(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 120 {
		t.Fatalf("concurrent API/reconciler plan writes lost records: %d", len(records))
	}
}

func TestPlanFileStoreCancelledWriterDoesNotChangeExistingPlan(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "plans.json")
	store := NewFileStore(storePath)
	if err := store.Save(context.Background(), Record{ID: "reviewed", Status: StatusReady}); err != nil {
		t.Fatal(err)
	}
	unlock, err := filelock.Acquire(context.Background(), storePath, true)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err = store.Save(ctx, Record{ID: "cancelled", Status: StatusFailed})
	unlock()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected cancelled plan lock wait: %v", err)
	}
	if err := store.Save(context.Background(), Record{ID: "reviewed", Status: StatusFailed}); err != nil {
		t.Fatal(err)
	}
	existing, err := store.Get(context.Background(), "reviewed")
	if err != nil || existing.Status != StatusReady {
		t.Fatal("duplicate save replaced immutable reviewed plan")
	}
	if _, err := store.Get(context.Background(), "cancelled"); !errors.Is(err, ErrNotFound) {
		t.Fatal("cancelled writer changed the plan store")
	}
}
