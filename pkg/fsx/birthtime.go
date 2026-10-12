// Package fsx holds file system helpers the standard library leaves to each
// platform.
package fsx

import (
	"io/fs"
	"time"
)

// BirthTime is the creation time the file system recorded for fi, when the
// platform exposes one; ok is false where it does not, and the caller falls
// back to fi.ModTime.
func BirthTime(fi fs.FileInfo) (t time.Time, ok bool) {
	return birthTime(fi)
}
