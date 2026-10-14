//go:build !darwin && !linux

package filelock

import (
	"context"
	"fmt"
)

func Acquire(context.Context, string, bool) (func(), error) {
	return nil, fmt.Errorf("durable file locking requires Linux or macOS")
}
