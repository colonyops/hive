//go:build !windows

package logutils

import (
	"context"

	"github.com/colonyops/hive/pkg/flock"
)

func lockFile(path string) (func(), error) {
	unlock, err := flock.Lock(context.Background(), path, true)
	if err != nil {
		return nil, err
	}
	return func() { _ = unlock() }, nil
}
