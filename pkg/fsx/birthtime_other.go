//go:build !darwin

package fsx

import (
	"io/fs"
	"time"
)

func birthTime(fs.FileInfo) (time.Time, bool) {
	return time.Time{}, false
}
