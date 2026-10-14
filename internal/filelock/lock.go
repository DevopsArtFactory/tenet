package filelock

import "errors"

var ErrBusy = errors.New("file lock is held by another process")
