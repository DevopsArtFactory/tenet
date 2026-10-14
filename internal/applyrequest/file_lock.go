package applyrequest

import (
	"context"
	"errors"
	"fmt"
	"github.com/DevopsArtFactory/tenet/internal/filelock"
)

func lockStore(ctx context.Context, path string) (func(), error) { return lockFile(ctx, path, true) }
func lockFile(ctx context.Context, path string, wait bool) (func(), error) {
	if path == "" {
		return nil, fmt.Errorf("file apply request store path is required")
	}
	unlock, err := filelock.Acquire(ctx, path, wait)
	if errors.Is(err, filelock.ErrBusy) {
		return nil, ErrExecutionBusy
	}
	return unlock, err
}
