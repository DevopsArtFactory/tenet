//go:build !darwin && !linux

package applyrequest

import (
	"context"
	"fmt"
)

func lockStore(context.Context, string) (func(), error) {
	return nil, fmt.Errorf("file apply request writes require Linux or macOS file locking")
}
