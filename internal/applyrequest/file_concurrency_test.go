//go:build darwin || linux

package applyrequest

import (
	"bufio"
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
)

// Each subprocess uses its own Go runtime, so an in-process mutex cannot make
// this test pass. Creates and claim-like mutations must share the file lock.
func TestFileStoreConcurrentProcesses(t *testing.T) {
	if path := os.Getenv("TENET_TEST_APPLY_STORE"); path != "" {
		store := NewFileStore(path)
		worker := os.Getenv("TENET_TEST_APPLY_WORKER")
		for i := 0; i < 20; i++ {
			if err := store.Save(context.Background(), Record{ID: fmt.Sprintf("%s-%d", worker, i)}); err != nil {
				t.Fatal(err)
			}
			_, err := store.Mutate(context.Background(), "counter", func(record Record) (Record, error) {
				record.Lifecycle.CompletedSteps++
				return record, nil
			})
			if err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	path := filepath.Join(t.TempDir(), "applies.json")
	store := NewFileStore(path)
	if err := store.Save(context.Background(), Record{ID: "counter"}); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for i := 0; i < 6; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			cmd := exec.Command(os.Args[0], "-test.run=^TestFileStoreConcurrentProcesses$", "-test.timeout=30s")
			cmd.Env = append(os.Environ(), "TENET_TEST_APPLY_STORE="+path, "TENET_TEST_APPLY_WORKER="+strconv.Itoa(i))
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("worker %d: %v\n%s", i, err, output)
			}
		}()
	}
	workers.Wait()
	records, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 121 {
		t.Fatalf("concurrent creates lost records: got %d, want 121", len(records))
	}
	record, err := store.Get(context.Background(), "counter")
	if err != nil || record.Lifecycle.CompletedSteps != 120 {
		t.Fatalf("concurrent changes lost updates: %+v, %v", record.Lifecycle, err)
	}
}

func TestFileStoreLockCancellationAndReuse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "applies.json")
	unlock, err := lockStore(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err = NewFileStore(path).Save(ctx, Record{ID: "cancelled"})
	unlock()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected cancelled lock wait, got %v", err)
	}
	if err := NewFileStore(path).Save(context.Background(), Record{ID: "saved"}); err != nil {
		t.Fatalf("lock was not reusable after cancellation: %v", err)
	}
}

func TestFileStoreLockReleasedAfterProcessExit(t *testing.T) {
	if path := os.Getenv("TENET_TEST_LOCK_EXIT"); path != "" {
		_, err := lockStore(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Println("locked")
		// Exit without calling unlock, as would happen on a worker crash.
		os.Exit(0)
	}
	path := filepath.Join(t.TempDir(), "applies.json")
	cmd := exec.Command(os.Args[0], "-test.run=^TestFileStoreLockReleasedAfterProcessExit$")
	cmd.Env = append(os.Environ(), "TENET_TEST_LOCK_EXIT="+path)
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(output)
	if !scanner.Scan() || scanner.Text() != "locked" {
		t.Error("subprocess did not acquire the lock")
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := NewFileStore(path).Save(ctx, Record{ID: "recovered"}); err != nil {
		t.Fatalf("dead process retained lock: %v", err)
	}
}

func TestFileStoreMutationRejectsIdentityChangesAndDoesNotPersistErrors(t *testing.T) {
	store := NewFileStore(filepath.Join(t.TempDir(), "applies.json"))
	ctx := context.Background()
	if err := store.Save(ctx, Record{ID: "original", Status: StatusQueued}); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(Record) (Record, error){
		func(r Record) (Record, error) { r.ID = "replacement"; return r, nil },
		func(r Record) (Record, error) { r.Status = StatusFailed; return r, ErrAlreadyClaimed },
	} {
		if _, err := store.Mutate(ctx, "original", change); err == nil {
			t.Fatal("expected mutation rejection")
		}
	}
	record, err := store.Get(ctx, "original")
	if err != nil || record.Status != StatusQueued {
		t.Fatalf("rejected mutation changed persisted record: %+v, %v", record, err)
	}
}
