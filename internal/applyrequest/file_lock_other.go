//go:build !darwin && !linux

package applyrequest

import (
	"context"
	"fmt"
)

func lockStore(context.Context, string) (func(), error) {
	return nil, fmt.Errorf("file apply request writes require Linux or macOS file locking")
}

func lockFile(context.Context, string, bool) (func(), error) {
	return nil, fmt.Errorf("file execution locking requires Linux or macOS")
}
