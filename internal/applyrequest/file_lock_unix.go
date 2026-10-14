//go:build darwin || linux

package applyrequest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// Lock a stable sidecar inode: locking the JSON file would stop protecting it
// as soon as an atomic rename replaces that inode. Never unlink the sidecar.
func lockStore(ctx context.Context, path string) (func(), error) { return lockFile(ctx, path, true) }

func lockFile(ctx context.Context, path string, wait bool) (func(), error) {
	if path == "" {
		return nil, fmt.Errorf("file apply request store path is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create apply request store dir: %w", err)
	}
	file, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open apply request store lock: %w", err)
	}
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { _ = file.Close() }, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
			_ = file.Close()
			return nil, fmt.Errorf("lock apply request store: %w", err)
		}
		if !wait {
			_ = file.Close()
			return nil, ErrExecutionBusy
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			_ = file.Close()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
